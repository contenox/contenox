package contenoxcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/presence"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

// envMap is the getenv seam for discovery tests: it drives the CONTENOX_SERVER_*
// resolution order without mutating the real process environment.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func newTestForwarder(t *testing.T, env map[string]string, store *presence.Store) *missionForwarder {
	t.Helper()
	f := newMissionForwarder(store)
	f.getenv = envMap(env)
	return f
}

// newPresenceStore builds a real file-backed presence store (the shared-SQLite
// path serve writes and a forwarder reads), so discovery order 2 is exercised
// against the actual store, not a stand-in.
func newPresenceStore(t *testing.T) *presence.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "local.db")
	db, err := libdbexec.NewSQLiteDBManager(context.Background(), path, libkvstore.SQLiteSchema)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return presence.NewStore(libkvstore.NewSQLiteManager(db))
}

// TestUnit_MissionForwarder_DiscoveryOrder pins the fixed resolution order:
// explicit env URL beats a serve presence row beats the loopback default.
func TestUnit_MissionForwarder_DiscoveryOrder(t *testing.T) {
	ctx := context.Background()
	store := newPresenceStore(t)
	require.NoError(t, store.Register(ctx, presence.Record{
		InstanceID: "serve-1",
		Kind:       presence.KindServe,
		Address:    "127.0.0.1:39999",
	}))

	// 1. Explicit CONTENOX_SERVER_URL wins over everything.
	f := newTestForwarder(t, map[string]string{envServeURL: "http://serve.example:8080"}, store)
	url, _ := f.discover(ctx)
	require.Equal(t, "http://serve.example:8080", url, "explicit env URL must win")

	// 2. No env URL → the serve presence row's address.
	f = newTestForwarder(t, map[string]string{}, store)
	url, _ = f.discover(ctx)
	require.Equal(t, "http://127.0.0.1:39999", url, "presence row address is discovery order 2")

	// 3. No env URL and no presence store → serve's loopback bind default.
	f = newTestForwarder(t, map[string]string{}, nil)
	url, _ = f.discover(ctx)
	require.Equal(t, "http://"+defaultServeAddr+":"+defaultServePort, url, "the loopback default is the last resort")
}

// TestUnit_MissionForwarder_PresenceDiscovery_SkipsStaleAndWrongKind proves
// discovery only trusts a LIVE serve row: a stale serve (likely dead) and an
// editor (acp) row are ignored, and a wildcard bind is normalized to loopback.
func TestUnit_MissionForwarder_PresenceDiscovery_SkipsStaleAndWrongKind(t *testing.T) {
	ctx := context.Background()
	store := newPresenceStore(t)

	// An acp editor row (wrong kind, and carries no address anyway).
	require.NoError(t, store.Register(ctx, presence.Record{InstanceID: "acp-x", Kind: presence.KindACP, Cwd: "/tmp"}))
	// A wildcard-bound serve row: 0.0.0.0 is not a dial target, must become loopback.
	require.NoError(t, store.Register(ctx, presence.Record{InstanceID: "serve-live", Kind: presence.KindServe, Address: "0.0.0.0:41000"}))

	f := newTestForwarder(t, map[string]string{}, store)
	url, _ := f.discover(ctx)
	require.Equal(t, "http://127.0.0.1:41000", url, "wildcard bind normalized to loopback; editor row ignored")
}

// fakeServe is an httptest stand-in for a running serve: it answers /health,
// POST /api/fleet/dispatch, and GET /api/agents/by-name/{name}, recording what it
// saw so the forwarding client's wire behavior can be asserted.
type fakeServe struct {
	srv          *httptest.Server
	healthHits   int32
	lastDispatch fleetservice.DispatchRequest
	lastAuth     string
	knownAgents  map[string]bool
	healthDown   bool
}

func newFakeServe(t *testing.T) *fakeServe {
	t.Helper()
	fs := &fakeServe{knownAgents: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fs.healthHits, 1)
		if fs.healthDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/fleet/dispatch", func(w http.ResponseWriter, r *http.Request) {
		fs.lastAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&fs.lastDispatch)
		_ = json.NewEncoder(w).Encode(fleetservice.DispatchResult{
			InstanceID: "inst-remote", SessionID: "sess-remote", MissionID: "m-remote",
		})
	})
	mux.HandleFunc("GET /api/agents/by-name/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !fs.knownAgents[name] {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "agent not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(runtimetypes.Agent{Name: name, Enabled: true})
	})
	fs.srv = httptest.NewServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

// TestUnit_MissionForwarder_ReachableProbesHealthAndCaches proves Reachable maps
// a live /health to true and a stopped serve to false, and that the probe is
// cached for probeTTL (a burst of session advertisements does not spam /health).
func TestUnit_MissionForwarder_ReachableProbesHealthAndCaches(t *testing.T) {
	fs := newFakeServe(t)
	f := newTestForwarder(t, map[string]string{envServeURL: fs.srv.URL}, nil)
	now := time.Unix(0, 0)
	f.now = func() time.Time { return now }
	f.probeTTL = time.Second

	require.True(t, f.Reachable(), "a live serve /health(200) must read reachable")
	require.Equal(t, int32(1), atomic.LoadInt32(&fs.healthHits))

	// A second call within the TTL is served from cache — no new probe.
	require.True(t, f.Reachable())
	require.Equal(t, int32(1), atomic.LoadInt32(&fs.healthHits), "within TTL the probe is cached")

	// Past the TTL it re-probes.
	now = now.Add(2 * time.Second)
	require.True(t, f.Reachable())
	require.Equal(t, int32(2), atomic.LoadInt32(&fs.healthHits), "past TTL the probe re-runs")

	// A serve that stops answering reads unreachable on the next uncached probe.
	fs.healthDown = true
	now = now.Add(2 * time.Second)
	require.False(t, f.Reachable(), "a non-200 /health must read unreachable")
}

// TestUnit_MissionForwarder_Reachable_ConnectionRefused proves a serve that is
// GONE (nothing listening) reads unreachable, not an error — the honest signal
// that drops /mission off a fresh session's menu.
func TestUnit_MissionForwarder_Reachable_ConnectionRefused(t *testing.T) {
	fs := newFakeServe(t)
	url := fs.srv.URL
	fs.srv.Close() // nothing listens now
	f := newTestForwarder(t, map[string]string{envServeURL: url}, nil)
	require.False(t, f.Reachable(), "a serve with nothing listening reads unreachable")
	require.Equal(t, url, f.TargetURL(), "TargetURL still names the configured serve for the teaching error")
}

// TestUnit_MissionForwarder_Dispatch proves a fired mission forwards to POST
// /fleet/dispatch carrying the whole request — including ParentSessionID, the
// supervision edge — and decodes serve's DispatchResult.
func TestUnit_MissionForwarder_Dispatch(t *testing.T) {
	fs := newFakeServe(t)
	f := newTestForwarder(t, map[string]string{envServeURL: fs.srv.URL, envServeToken: "sekret"}, nil)

	res, err := f.Dispatch(context.Background(), fleetservice.DispatchRequest{
		AgentName:       "reporter",
		Intent:          "triage the failing run",
		HITLPolicyName:  "hitl-policy-default.json",
		ParentSessionID: "acp-parent-session",
	})
	require.NoError(t, err)
	require.Equal(t, "m-remote", res.MissionID)
	require.Equal(t, "reporter", fs.lastDispatch.AgentName)
	require.Equal(t, "triage the failing run", fs.lastDispatch.Intent)
	require.Equal(t, "acp-parent-session", fs.lastDispatch.ParentSessionID,
		"the firing session id must ride the dispatch as the supervision edge")
	require.Equal(t, "Bearer sekret", fs.lastAuth,
		"the token comes from CONTENOX_SERVER_TOKEN and authenticates the forwarded dispatch")
}

// TestUnit_MissionForwarder_GetByName resolves a declared agent over REST, and
// maps an unknown agent (404) to an error the /mission resolver reads as "not a
// named agent".
func TestUnit_MissionForwarder_GetByName(t *testing.T) {
	fs := newFakeServe(t)
	fs.knownAgents["planner"] = true
	f := newTestForwarder(t, map[string]string{envServeURL: fs.srv.URL}, nil)

	agent, err := f.GetByName(context.Background(), "planner")
	require.NoError(t, err)
	require.Equal(t, "planner", agent.Name)

	_, err = f.GetByName(context.Background(), "not-an-agent")
	require.Error(t, err, "an unknown agent must surface as an error (read as 'not a named agent')")
}

func TestUnit_NormalizeServeHostPort(t *testing.T) {
	require.Equal(t, "127.0.0.1:32123", normalizeServeHostPort("0.0.0.0:32123"))
	require.Equal(t, "127.0.0.1:8080", normalizeServeHostPort("[::]:8080"))
	require.Equal(t, "10.0.0.5:9000", normalizeServeHostPort("10.0.0.5:9000"))
}

func TestUnit_NormalizeBaseURL(t *testing.T) {
	require.Equal(t, "http://host:8080", normalizeBaseURL("host:8080"), "a bare host:port defaults to http")
	require.Equal(t, "https://host:8443", normalizeBaseURL("https://host:8443/"), "a scheme is kept; trailing slash trimmed")
}
