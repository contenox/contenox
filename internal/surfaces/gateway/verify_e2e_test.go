package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelrepo/ollama"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/internal/version"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/stretchr/testify/require"
)

type gatewayAuthority struct {
	issuer   *liblicense.Issuer
	verifier gateway.ClaimsVerifier
	hasher   *libtokenkey.Hasher
	store    runtimetypes.Store
	usage    runtimetypes.UsageStore
	bus      libbus.Messenger
}

type gatewaySeed func(ctx context.Context, store runtimetypes.Store)

func newGatewayAuthority(t *testing.T, seeds ...gatewaySeed) (*gatewayAuthority, gateway.Service) {
	t.Helper()
	ctx := context.Background()

	privPEM, pubSSH, err := liblicense.GenerateSSHKeyPair("gateway-test", nil)
	require.NoError(t, err)
	priv, err := liblicense.ParseSSHPrivateKey(privPEM, nil)
	require.NoError(t, err)
	payloadKey, err := liblicense.DeriveKey(priv.Seed(), nil, liblicense.DefaultKDFInfo)
	require.NoError(t, err)

	issuer, err := liblicense.NewIssuerFromSSHPrivateKey(privPEM, nil)
	require.NoError(t, err)
	verifier, err := liblicense.NewVerifierFromSSHPublicKey(pubSSH, liblicense.WithPayloadDecryptionKey(payloadKey))
	require.NoError(t, err)
	hasher, err := libtokenkey.Generate()
	require.NoError(t, err)

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "gateway.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	store := runtimetypes.New(db.WithoutTransaction())
	for _, seed := range seeds {
		seed(ctx, store)
	}

	bus := libbus.NewInMem()
	state, err := runtimestate.New(ctx, db, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, state.RunBackendCycle(ctx))

	svc, err := gateway.New(gateway.Config{
		DB: db, Verifier: verifier, Hasher: hasher, Runtime: state, Bus: bus,
		Models: gateway.NewTestModelRepo(t, state),
	})
	require.NoError(t, err)

	return &gatewayAuthority{
		issuer: issuer, verifier: verifier, hasher: hasher,
		store: store, usage: runtimetypes.NewUsageStore(db), bus: bus,
	}, svc
}

func (a *gatewayAuthority) mint(t *testing.T, clientID string, record bool) string {
	t.Helper()
	return a.mintFor(t, clientID, "*", record)
}

func (a *gatewayAuthority) mintFor(t *testing.T, clientID, models string, record bool, tune ...func(*liblicense.Claims)) string {
	t.Helper()
	ctx := context.Background()

	claims := liblicense.NewClaims("lic-"+clientID, "contenox", clientID)
	claims.Set("allowed_models", models)
	for _, apply := range tune {
		apply(&claims)
	}
	claims.SetInt64(liblicense.OutputAllowanceKey("qwen3:8b"), 1_000_000)
	bearer, err := a.issuer.Issue(claims)
	require.NoError(t, err)

	if !record {
		return bearer
	}
	digest, err := a.hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	require.NoError(t, err)
	expires := time.Now().UTC().Add(time.Hour)
	_, err = a.store.RecordProxyKey(ctx, runtimetypes.ProxyKey{
		KeyHash: digest, ClientID: clientID, Tier: "test", ExpiresAt: expires,
	})
	require.NoError(t, err)
	return bearer
}

func (a *gatewayAuthority) digestOf(t *testing.T, bearer string) string {
	t.Helper()
	digest, err := a.hasher.Hash(libtokenkey.PurposeProxyKey, bearer)
	require.NoError(t, err)
	return digest
}

func gatewayGet(t *testing.T, url, bearer string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url+"/api/tags", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestSystem_GatewayVerifiesMintedKeyAndRefusesRevoked(t *testing.T) {
	authority, svc := newGatewayAuthority(t)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	minted := authority.mint(t, "laptop-alex", true)
	require.Equal(t, http.StatusOK, gatewayGet(t, server.URL, minted), "a key this gateway minted must be accepted")

	unrecorded := authority.mint(t, "never-recorded", false)
	require.Equal(t, http.StatusUnauthorized, gatewayGet(t, server.URL, unrecorded),
		"a token with no ledger row was not minted here and fails closed")

	revoked, err := authority.store.RevokeProxyKey(context.Background(), authority.digestOf(t, minted))
	require.NoError(t, err)
	require.True(t, revoked)
	require.Equal(t, http.StatusUnauthorized, gatewayGet(t, server.URL, minted), "a revoked key reaches nothing, not even the model list")

	require.Equal(t, http.StatusUnauthorized, gatewayGet(t, server.URL, "not-a-token"))
}

func TestSystem_GatewayRefusesAKeyRevokedOverTheControlPlane(t *testing.T) {
	authority, svc := newGatewayAuthority(t)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, svc.SubscribeControlPlane(ctx))

	minted := authority.mint(t, "laptop-bob", true)
	require.Equal(t, http.StatusOK, gatewayGet(t, server.URL, minted))

	event, err := json.Marshal(gateway.ProxyControlRevocationEvent{
		ClientID:  "laptop-bob",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		Reason:    "operator cutoff",
	})
	require.NoError(t, err)
	require.NoError(t, authority.bus.Publish(ctx, gateway.SubjectProxyControlRevocation, event))

	require.Eventually(t, func() bool {
		return gatewayGet(t, server.URL, minted) == http.StatusUnauthorized
	}, 5*time.Second, 50*time.Millisecond, "a broadcast revocation must take effect without a restart")
}

func TestSystem_GatewayAnswersVanillaOnVersionAndNamesItselfOnContenox(t *testing.T) {
	_, svc := newGatewayAuthority(t)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	versionResp, err := http.Get(server.URL + "/api/version")
	require.NoError(t, err)
	defer versionResp.Body.Close()
	require.Equal(t, http.StatusOK, versionResp.StatusCode)

	var vanilla map[string]any
	require.NoError(t, json.NewDecoder(versionResp.Body).Decode(&vanilla))
	require.Equal(t, []string{"version"}, keysOf(vanilla),
		"the vanilla route carries nothing a vanilla client would not expect")

	handshake, err := http.Get(server.URL + ollama.ContenoxPath)
	require.NoError(t, err)
	defer handshake.Body.Close()
	require.Equal(t, http.StatusOK, handshake.StatusCode)

	var decoded ollama.ContenoxResponse
	require.NoError(t, json.NewDecoder(handshake.Body).Decode(&decoded))
	require.True(t, decoded.IsContenoxGateway(), "a client reads the product to know what is answering")
	require.Equal(t, version.Get(), decoded.Version, "the build identity comes from the version plumbing, not a literal")
	require.True(t, decoded.Supports(ollama.ExtensionAudios),
		"the extension a client is about to use is declared by name, or it must not use it")
	require.Equal(t, vanilla["version"], decoded.OllamaVersion,
		"the handshake and the vanilla route must not disagree about the API generation")
}

func TestSystem_GatewayUsageReturnsTheCallingClientsRawAndEffectiveMeter(t *testing.T) {
	authority, svc := newGatewayAuthority(t)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	bearer := authority.mintFor(t, "laptop-alex", "reasoner", true, func(claims *liblicense.Claims) {
		claims.SetInt64(liblicense.ThinkingDiscountMultiplierKey("reasoner"), 2500)
		claims.SetInt64(liblicense.FiveHourAllowanceKey("reasoner"), 10_000)
	})
	now := time.Now().UTC()
	require.NoError(t, authority.usage.RecordUsage(context.Background(), runtimetypes.ProxyUsage{
		KeyHash: "alex", ClientID: "laptop-alex", Model: "reasoner",
		PromptTokens: 10, CompletionTokens: 100, ThinkingTokens: 80, TotalTokens: 110,
		EffectiveInput: 10, EffectiveOutput: 40, CostMicrodollars: 321,
		RecordedAt: now,
	}))
	require.NoError(t, authority.usage.RecordUsage(context.Background(), runtimetypes.ProxyUsage{
		KeyHash: "bob", ClientID: "laptop-bob", Model: "reasoner",
		PromptTokens: 1000, CompletionTokens: 1000, TotalTokens: 2000,
		EffectiveInput: 1000, EffectiveOutput: 1000,
		RecordedAt: now,
	}))

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/contenox/usage?model=reasoner", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got gateway.UsageResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "laptop-alex", got.ClientID)
	require.Equal(t, "reasoner", got.Model)
	require.EqualValues(t, 100, got.Windows.Week.Usage.CompletionTokens)
	require.EqualValues(t, 80, got.Windows.Week.Usage.ThinkingTokens)
	require.EqualValues(t, 20, got.Windows.Week.Usage.VisibleOutputTokens)
	require.EqualValues(t, 40, got.Windows.Week.Usage.EffectiveOutputTokens)
	require.EqualValues(t, 50, got.Windows.FiveHour.Usage.EffectiveTotalTokens)
	require.EqualValues(t, 321, got.Windows.Month.Usage.CostMicrodollars)
	require.EqualValues(t, 110, got.Windows.Total.Usage.TotalTokens)
	require.EqualValues(t, 10_000, got.Allowances.FiveHourTokens)
	require.Equal(t, 0.25, got.Allowances.ThinkingDiscountMultiplier)
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
