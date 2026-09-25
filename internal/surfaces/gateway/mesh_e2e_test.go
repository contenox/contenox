package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

func prompt() []modelrepo.Message {
	return []modelrepo.Message{{Role: "user", Content: "hello"}}
}

type edgeFixture struct {
	bus   libbus.Messenger
	state *runtimestate.State
	db    libdb.DBManager
	store runtimetypes.Store
	usage runtimetypes.UsageStore
}

// newEdgeFixture is one runtime behind several gateways over one bus: the shape
// an operator runs when a client in each region points at the gateway nearest to
// it, and the meter has to be the deployment's rather than one process's. The bus
// is the SQLite one substrate.OpenBus picks for a local deployment, which is what
// the gateways share in that shape.
func newEdgeFixture(t *testing.T, script string) *edgeFixture {
	t.Helper()
	ctx := context.Background()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "dialog.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o600))

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "gateway.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	bus := libbus.NewSQLite(db.WithoutTransaction())
	t.Cleanup(func() { _ = bus.Close() })
	state, err := runtimestate.New(ctx, db, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID: "scripted", Name: "scripted",
		Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
	}))
	require.NoError(t, state.RunBackendCycle(ctx))

	return &edgeFixture{
		bus: bus, state: state, db: db,
		store: runtimetypes.New(db.WithoutTransaction()),
		usage: runtimetypes.NewUsageStore(db),
	}
}

func (f *edgeFixture) gateway(t *testing.T, authority *gatewayAuthority) gateway.Service {
	t.Helper()
	svc, err := gateway.New(gateway.Config{
		DB: f.db, Verifier: authority.verifier, Hasher: authority.hasher, Runtime: f.state, Bus: f.bus,
		Models: gateway.NewTestModelRepo(t, f.state),
	})
	require.NoError(t, err)
	return svc
}

// mint writes the ledger row into this fixture's store: the point of the mesh is
// several processes over one database, so the key has to be one they all find.
func (f *edgeFixture) mint(t *testing.T, authority *gatewayAuthority, clientID string) string {
	t.Helper()
	ctx := context.Background()
	claims := liblicense.NewClaims("lic-"+clientID, "contenox", clientID)
	claims.Set("allowed_models", "*")
	bearer, err := authority.issuer.Issue(claims)
	require.NoError(t, err)
	digest, err := authority.hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	require.NoError(t, err)
	_, err = f.store.RecordProxyKey(ctx, runtimetypes.ProxyKey{
		KeyHash: digest, ClientID: clientID, Tier: "test",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	return bearer
}

// usageOf reads the client's week for one model, which is what every ceiling is
// measured against.
func (f *edgeFixture) usageOf(t *testing.T, clientID, model string) runtimetypes.UsageSnapshot {
	t.Helper()
	snapshot, err := f.usage.ReadUsage(context.Background(), runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: clientID,
		Model: model, WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	return snapshot
}

// A turn served by one instance is the deployment's usage, so a second instance
// running the usage consumer has to pick it up off the bus. The bridge that
// already runs so a peer can join has to carry the metering too.
func TestSystem_ATurnOnOneGatewayIsMeteredOnTheDeployment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const script = `{
  "model": "edge-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "one", "usage": {"prompt_tokens": 40, "completion_tokens": 60}}, {"text": "two", "usage": {"prompt_tokens": 40, "completion_tokens": 60}}]
}`
	fixture := newEdgeFixture(t, script)
	authority, _ := newGatewayAuthority(t)
	serving := fixture.gateway(t, authority)
	require.NoError(t, serving.StartUsageConsumer(ctx))

	token := fixture.mint(t, authority, "laptop-alex")
	res, err := serving.ChatTurn(ctx, token, "edge-model", prompt())
	require.NoError(t, err)
	require.NotEmpty(t, res.Message.Content)

	require.Eventually(t, func() bool {
		return fixture.usageOf(t, "laptop-alex", "edge-model").TotalTokens == 100
	}, 2*time.Second, 20*time.Millisecond, "the turn is on the deployment's meter, not only the instance that served it")
}

// One instance's lock has to reach the others: a client cut off at the gateway
// it happened to hit must not be served by the one next to it.
func TestSystem_ACutoffOnOneGatewayStopsTheOthers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const script = `{
  "model": "edge-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "one"}, {"text": "two"}]
}`
	fixture := newEdgeFixture(t, script)
	authority, _ := newGatewayAuthority(t)
	primary := fixture.gateway(t, authority)
	require.NoError(t, primary.SubscribeControlPlane(ctx))
	edge := fixture.gateway(t, authority)
	require.NoError(t, edge.SubscribeControlPlane(ctx))

	token := fixture.mint(t, authority, "laptop-bob")
	_, err := edge.ChatTurn(ctx, token, "edge-model", prompt())
	require.NoError(t, err, "the key is good before the cutoff")

	require.NoError(t, primary.BroadcastRevocation(ctx, gateway.ProxyControlRevocationEvent{
		ClientID:  "laptop-bob",
		Reason:    "budget limit reached",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}))

	require.Eventually(t, func() bool {
		_, err := edge.ChatTurn(ctx, token, "edge-model", prompt())
		return err != nil && errors.Is(err, gateway.ErrAllowanceExhausted)
	}, 2*time.Second, 20*time.Millisecond, "a broadcast cutoff must reach the instance that did not raise it")
}

// An instance that starts after a cutoff has to learn about it from a peer
// rather than from the ledger, which holds no lock.
func TestSystem_AGatewayThatStartsLateLearnsTheCutoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const script = `{
  "model": "edge-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "one"}]
}`
	fixture := newEdgeFixture(t, script)
	authority, _ := newGatewayAuthority(t)
	primary := fixture.gateway(t, authority)
	require.NoError(t, primary.StartUsageConsumer(ctx))
	require.NoError(t, primary.SubscribeControlPlane(ctx))

	require.NoError(t, primary.BroadcastRevocation(ctx, gateway.ProxyControlRevocationEvent{
		ClientID:  "laptop-carol",
		Reason:    "budget limit reached",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}))

	late := fixture.gateway(t, authority)
	require.NoError(t, late.SubscribeControlPlane(ctx),
		"the responder belongs to the control plane, not to the usage consumer")

	token := fixture.mint(t, authority, "laptop-carol")
	require.Eventually(t, func() bool {
		_, err := late.ChatTurn(ctx, token, "edge-model", prompt())
		return err != nil && errors.Is(err, gateway.ErrAllowanceExhausted)
	}, 2*time.Second, 20*time.Millisecond, "a late gateway asks a peer for the cutoffs it missed")
}

// A gateway asks a subject it also answers, so its own snapshot competes for its
// own request: the bus hands a request to one responder, and the asker is one of
// the candidates. The answer it needs is a peer's, so an empty one is not a
// finding and the exchange has to converge by asking again.
func TestUnit_TheSnapshotExchangeConvergesOnAPeersCutoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fixture := newEdgeFixture(t, `{
  "model": "edge-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "one"}]
}`)
	authority, _ := newGatewayAuthority(t)
	holder := fixture.gateway(t, authority)
	require.NoError(t, holder.SubscribeControlPlane(ctx))
	require.NoError(t, holder.BroadcastRevocation(ctx, gateway.ProxyControlRevocationEvent{
		ClientID:  "laptop-dave",
		Reason:    "budget limit reached",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}))

	seeker := fixture.gateway(t, authority)
	require.NoError(t, seeker.SubscribeControlPlane(ctx))

	require.Eventually(t, func() bool {
		blocked, _ := gateway.SnapshotHolds(seeker, "laptop-dave")
		return blocked
	}, 5*time.Second, 50*time.Millisecond, "asking again is what gets past the gateway answering itself")
}

// A backend that refuses every turn must be taken out of service, not asked
// again on every request. llmrepo retries inside one request and remembers
// nothing between them, so a dead upstream was hammered for as long as clients
// kept asking — and the gateway answered 503 "try again later" the whole time,
// which is an invitation to ask again.
//
// The upstream here is discovered normally (it lists the model) and then refuses
// the turn, which is what an expired key or an exhausted quota looks like. One
// refusal is spent learning it: the turn already holds its resolved candidate
// list, so the breaker takes effect from the next request.
func TestSystem_ADeadBackendIsTakenOutOfService(t *testing.T) {
	ctx := context.Background()

	// The scripted backend replays one turn per call and there is no scripted
	// failure mode, so the dialog is written as "as many turns as this test makes
	// calls": what is under test is where the calls go, not how many succeed.
	turns := make([]map[string]any, 0, 12)
	for i := 0; i < 12; i++ {
		turns = append(turns, map[string]any{"text": fmt.Sprintf("turn-%d", i)})
	}
	script, err := json.Marshal(map[string]any{
		"model":        "shared-model",
		"capabilities": map[string]any{"chat": true},
		"turns":        turns,
	})
	require.NoError(t, err)
	fixture := newEdgeFixture(t, string(script))
	authority, _ := newGatewayAuthority(t)

	var asked atomic.Int64
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"shared-model","model":"shared-model","modified_at":"2026-01-01T00:00:00Z","size":1,"digest":"d"}]}`))
		case strings.HasSuffix(r.URL.Path, "/show"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion"],"model_info":{"llama.context_length":8192}}`))
		case strings.HasSuffix(r.URL.Path, "/chat"):
			asked.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dead.Close)

	require.NoError(t, fixture.store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "dead", Name: "dead", Type: "ollama", BaseURL: dead.URL,
	}))
	require.NoError(t, fixture.state.RunBackendCycle(ctx))

	svc := fixture.gateway(t, authority)
	gateway.SetBreakerCooldown(svc, 2*time.Second)
	token := fixture.mint(t, authority, "laptop-dead")

	for i := 0; i < 5; i++ {
		_, err := svc.ChatTurn(ctx, token, "shared-model", prompt())
		require.NoError(t, err, "the backend that works serves every turn (call %d)", i)
	}

	require.Equal(t, int64(1), asked.Load(),
		"the dead backend is asked once, to learn it is dead, and never again")
	require.True(t, gateway.Quarantined(svc, "dead"), "the backend that refused is out of service")

	// Five minutes of client retries against a dead endpoint, in miniature: the
	// cooldown has not elapsed, so none of them reach it.
	for i := 0; i < 5; i++ {
		_, err := svc.ChatTurn(ctx, token, "shared-model", prompt())
		require.NoError(t, err, "the deployment serves straight through the quarantine (call %d)", i)
	}
	require.Equal(t, int64(1), asked.Load(), "retrying clients do not reach an upstream in its cooldown")
}

// The quarantine is what keeps a client's retries off a dead upstream, so the
// check that decides whether to refuse has to leave the half-open probe alone:
// consuming it here would refuse the very request that was going to test the
// backend.
func TestUnit_TheQuarantineCheckDoesNotConsumeTheProbe(t *testing.T) {
	fixture := newEdgeFixture(t, `{
  "model": "edge-model",
  "capabilities": {"chat": true},
  "turns": [{"text": "one"}]
}`)
	authority, _ := newGatewayAuthority(t)
	svc := fixture.gateway(t, authority)
	gateway.SetBreakerCooldown(svc, 80*time.Millisecond)

	gateway.TripBreaker(svc, "edge-model")
	require.True(t, gateway.QuarantinedWholeModel(svc, "edge-model"), "a tripped backend quarantines its model")
	require.False(t, gateway.WouldServe(svc, "edge-model"), "and is not worth attempting yet")

	time.Sleep(120 * time.Millisecond)
	require.True(t, gateway.WouldServe(svc, "edge-model"), "past the cooldown the attempt is worth making")
	require.False(t, gateway.QuarantinedWholeModel(svc, "edge-model"),
		"asking must not have spent the probe, or the request it gates finds nothing to try")
}

// Embedding picks one candidate rather than walking a list, so a quarantine that
// only the chat path consulted left the embed route hammering the same dead
// upstream. The live smoke found this; the unit fixture did not, because it
// exercised chat.
func TestSystem_EmbeddingSkipsAQuarantinedBackend(t *testing.T) {
	ctx := context.Background()

	turns := make([]map[string]any, 0, 8)
	for i := 0; i < 8; i++ {
		turns = append(turns, map[string]any{"text": fmt.Sprintf("turn-%d", i)})
	}
	script, err := json.Marshal(map[string]any{
		"model":            "shared-model",
		"capabilities":     map[string]any{"chat": true, "embed": true},
		"embed_dimensions": 8,
		"turns":            turns,
	})
	require.NoError(t, err)
	fixture := newEdgeFixture(t, string(script))
	authority, _ := newGatewayAuthority(t)

	var asked atomic.Int64
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"shared-model","model":"shared-model","modified_at":"2026-01-01T00:00:00Z","size":1,"digest":"d"}]}`))
		case strings.HasSuffix(r.URL.Path, "/show"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion","embedding"],"model_info":{"llama.context_length":8192}}`))
		case strings.HasSuffix(r.URL.Path, "/embed"):
			asked.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dead.Close)

	require.NoError(t, fixture.store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "dead", Name: "dead", Type: "ollama", BaseURL: dead.URL,
	}))
	require.NoError(t, fixture.state.RunBackendCycle(ctx))

	svc := fixture.gateway(t, authority)
	gateway.SetBreakerCooldown(svc, 2*time.Second)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	token := fixture.mint(t, authority, "laptop-embed")
	for i := 0; i < 6; i++ {
		req, reqErr := http.NewRequest(http.MethodPost, server.URL+"/api/embed",
			strings.NewReader(`{"model":"shared-model","input":"hello"}`))
		require.NoError(t, reqErr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	require.LessOrEqual(t, asked.Load(), int64(1),
		"the dead backend is asked once for embedding, not once per request")
	require.True(t, gateway.Quarantined(svc, "dead"),
		"an embed route that refuses takes its backend out of service like any other turn")
}
