package runtimestate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func newGeminiCatalogServer(t *testing.T, modelName string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1beta/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": modelName}},
			})
		case "/v1beta/" + modelName:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":                       modelName,
				"inputTokenLimit":            200000,
				"outputTokenLimit":           8192,
				"supportedGenerationMethods": []string{"generateContent"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newGeminiState(t *testing.T, server *httptest.Server) (context.Context, *State, runtimetypes.Store) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime-gemini-declared.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	store := runtimetypes.New(db.WithoutTransaction())
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "gemini-backend", Name: "gemini", Type: "gemini", BaseURL: server.URL,
	}))
	key, err := json.Marshal(ProviderConfig{APIKey: "test-key", Type: "gemini"})
	require.NoError(t, err)
	require.NoError(t, store.SetKV(ctx, GeminiKey, key))

	bus := libbus.NewSQLite(db.WithoutTransaction())
	t.Cleanup(func() { _ = bus.Close() })
	state, err := New(ctx, db, bus)
	require.NoError(t, err)
	return ctx, state, store
}

// A declared row used to be consulted only by the modeld, ollama, vLLM and
// OpenAI paths, so a Gemini operator's stated window was silently ignored.
func TestUnit_RunBackendCycle_GeminiDeclaredContextWins(t *testing.T) {
	ctx := context.Background()
	const modelName = "models/gemini-2.5-pro"
	_, state, store := newGeminiState(t, newGeminiCatalogServer(t, modelName))

	require.NoError(t, store.AppendModel(ctx, &runtimetypes.Model{
		ID: "declared-gemini", Model: modelName, ContextLength: 1_000_000, CanChat: true,
	}))
	require.NoError(t, state.RunBackendCycle(ctx))

	rt := state.Get(ctx)
	require.Contains(t, rt, "gemini-backend")
	require.Empty(t, rt["gemini-backend"].Error)
	require.Len(t, rt["gemini-backend"].PulledModels, 1)

	pm := rt["gemini-backend"].PulledModels[0]
	require.Equal(t, modelName, pm.Model)
	require.Equal(t, 1_000_000, pm.ContextLength, "the declared window must win over the catalog's")
	require.Equal(t, 8192, pm.MaxOutputTokens, "a fact the row cannot state stays as observed")
}

func TestUnit_RunBackendCycle_GeminiLearnsTheObservedContextIntoTheRow(t *testing.T) {
	ctx := context.Background()
	const modelName = "models/gemini-2.5-flash"
	_, state, store := newGeminiState(t, newGeminiCatalogServer(t, modelName))

	require.NoError(t, store.AppendModel(ctx, &runtimetypes.Model{
		ID: "declared-gemini", Model: modelName, CanChat: true,
	}))
	require.NoError(t, state.RunBackendCycle(ctx))

	rt := state.Get(ctx)
	require.Len(t, rt["gemini-backend"].PulledModels, 1)
	require.Equal(t, 200000, rt["gemini-backend"].PulledModels[0].ContextLength)

	learned, err := store.GetModelByName(ctx, modelName)
	require.NoError(t, err)
	require.Equal(t, 200000, learned.ContextLength, "a context the catalog reported is written back, so a later cycle need not re-learn it")
}

func TestUnit_RunBackendCycle_GeminiUndeclaredModelIsStillListed(t *testing.T) {
	ctx := context.Background()
	_, state, _ := newGeminiState(t, newGeminiCatalogServer(t, "models/gemini-2.5-pro"))

	require.NoError(t, state.RunBackendCycle(ctx))

	rt := state.Get(ctx)
	require.Len(t, rt["gemini-backend"].PulledModels, 1, "declaring nothing must not hide what the catalog reports")
	require.Equal(t, 200000, rt["gemini-backend"].PulledModels[0].ContextLength)
}

func TestUnit_ApplyDeclaredModel_MergesRowThenDeclarations(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	observed := modelrepo.ObservedModel{
		Name:          "m",
		ContextLength: 32768,
		CapabilityConfig: modelrepo.CapabilityConfig{
			ContextLength: 32768, MaxOutputTokens: 4096, CanChat: true, CanVision: true,
		},
	}
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "m", runtimetypes.ModelFacts{
		ContextLength: 1000000,
	}))
	backend := &runtimetypes.Backend{ID: "b1", Name: "b1", Type: "openai"}

	undeclared := state.applyDeclaredModel(ctx, backend, nil, observed)
	require.Equal(t, 1000000, undeclared.ContextLength, "facts apply to a model nobody declared")

	declared := state.applyDeclaredModel(ctx, backend, &runtimetypes.Model{
		ID: "row", Model: "m", ContextLength: 8192, CanEmbed: true,
	}, observed)
	require.Equal(t, 1000000, declared.ContextLength, "facts are the most specific declaration")
	require.True(t, declared.CanEmbed, "the row's capabilities add to the observed ones")
	require.True(t, declared.CanVision, "a row that cannot express vision must not strip it")
	require.Equal(t, 4096, declared.MaxOutputTokens)
}

func TestUnit_MergeDeclaredRow_LearnsOnceAndAddsCapabilities(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.AppendModel(ctx, &runtimetypes.Model{ID: "row", Model: "m", CanChat: true}))

	learned := state.mergeDeclaredRow(ctx, &runtimetypes.Model{ID: "row", Model: "m"}, ModelPullStatus{
		CanChat: true, CanStream: true, ContextLength: 32768,
	})
	require.Equal(t, 32768, learned.ContextLength)

	row, err := store.GetModelByName(ctx, "m")
	require.NoError(t, err)
	require.Equal(t, 32768, row.ContextLength, "the observed window is written back into the row")
	require.True(t, row.CanChat)

	explicit := state.mergeDeclaredRow(ctx, &runtimetypes.Model{ID: "row", Model: "m", ContextLength: 8192, CanEmbed: true}, ModelPullStatus{
		CanChat: true, ContextLength: 32768,
	})
	require.Equal(t, 8192, explicit.ContextLength, "a stated window wins and is never overwritten")
	require.True(t, explicit.CanEmbed)
	require.True(t, explicit.CanChat)
}
