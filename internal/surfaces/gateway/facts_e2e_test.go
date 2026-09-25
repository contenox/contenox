package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
	_ "github.com/contenox/contenox/internal/models/modelrepo/ollama"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type licensedVerifier struct{ claims *liblicense.Claims }

func (v licensedVerifier) Verify(string) (*liblicense.Claims, error) { return v.claims, nil }

func TestSystem_LicensedClientLearnsDeclaredFacts(t *testing.T) {
	ctx := context.Background()
	const model = "deepseek-flash"

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "dialog.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "deepseek-flash",
  "context_length": 32768,
  "max_output_tokens": 4096,
  "capabilities": {"chat": true, "vision": false, "think": false, "embed": false},
  "turns": [{"text": "ok"}]
}`), 0o600))
	rtDB, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "runtime.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rtDB.Close() })
	bus := libbus.NewSQLite(rtDB.WithoutTransaction())
	t.Cleanup(func() { _ = bus.Close() })
	rt, err := runtimestate.New(ctx, rtDB, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	keys := runtimetypes.New(rtDB.WithoutTransaction())
	upstream := &runtimetypes.Backend{
		ID: "scripted-upstream", Name: "scripted-upstream",
		Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
	}
	require.NoError(t, keys.CreateBackend(ctx, upstream))
	require.NoError(t, keys.SetLLMProviderModelFact(ctx, upstream.ID, model, runtimetypes.ModelFacts{
		ContextLength:   1000000,
		MaxOutputTokens: 393216,
		Capabilities:    []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityTools, runtimetypes.CapabilityVision, runtimetypes.CapabilityThinking},
	}))
	require.NoError(t, rt.RunBackendCycle(ctx))

	claims := liblicense.NewClaims("lic-test", "issuer", "subject-test")
	claims.Set("allowed_models", model)

	svc, err := gateway.New(gateway.Config{DB: rtDB, Verifier: licensedVerifier{claims: &claims}, Runtime: rt, Models: gateway.NewTestModelRepo(t, rt)})
	require.NoError(t, err)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	proxy := httptest.NewServer(mux)
	t.Cleanup(proxy.Close)

	const token = "licence-token"
	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{
		Type: "ollama", BaseURL: proxy.URL, APIKey: token,
	})
	require.NoError(t, err)
	observed, err := catalog.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, observed, 1)

	got := observed[0]
	assert.Equal(t, model, got.Name)
	assert.Equal(t, 1000000, got.ContextLength)
	assert.Equal(t, 393216, got.MaxOutputTokens)
	assert.True(t, got.CanChat)
	assert.True(t, got.CanPrompt)
	assert.True(t, got.CanStream)
	assert.True(t, got.CanVision)
	assert.True(t, got.CanThink)
	assert.False(t, got.CanEmbed)

	providerForModel := catalog.ProviderFor(got)
	assert.Equal(t, 1000000, providerForModel.GetContextLength())
	assert.Equal(t, 393216, providerForModel.GetMaxOutputTokens())
	assert.True(t, providerForModel.CanVision())
}

func TestSystem_LicensedClientFallsBackToObservation(t *testing.T) {
	ctx := context.Background()
	const model = "undescribed-model"

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "dialog.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`{
  "model": "undescribed-model",
  "context_length": 32768,
  "max_output_tokens": 4096,
  "capabilities": {"chat": true, "vision": true, "think": false, "embed": false},
  "turns": [{"text": "ok"}]
}`), 0o600))
	rtDB, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "runtime.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rtDB.Close() })
	bus := libbus.NewSQLite(rtDB.WithoutTransaction())
	t.Cleanup(func() { _ = bus.Close() })
	rt, err := runtimestate.New(ctx, rtDB, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, runtimetypes.New(rtDB.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID: "scripted-upstream", Name: "scripted-upstream",
		Type: modelrepo.ScriptedTestBackendType, BaseURL: scriptPath,
	}))
	require.NoError(t, rt.RunBackendCycle(ctx))

	claims := liblicense.NewClaims("lic-test", "issuer", "subject-test")
	claims.Set("allowed_models", model)
	svc, err := gateway.New(gateway.Config{DB: rtDB, Verifier: licensedVerifier{claims: &claims}, Runtime: rt, Models: gateway.NewTestModelRepo(t, rt)})
	require.NoError(t, err)
	mux := http.NewServeMux()
	svc.AddOllamaProxyRoutes(mux)
	proxy := httptest.NewServer(mux)
	t.Cleanup(proxy.Close)

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{
		Type: "ollama", BaseURL: proxy.URL, APIKey: "licence-token",
	})
	require.NoError(t, err)
	observed, err := catalog.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, observed, 1)

	got := observed[0]
	assert.Equal(t, 32768, got.ContextLength)
	assert.Equal(t, 4096, got.MaxOutputTokens)
	assert.True(t, got.CanChat)
	assert.True(t, got.CanVision)
}
