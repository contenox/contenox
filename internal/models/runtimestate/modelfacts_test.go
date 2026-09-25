package runtimestate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func newModelFactsTestState(t *testing.T) (context.Context, *State, runtimetypes.Store) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime-model-facts.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx, &State{dbInstance: db}, runtimetypes.New(db.WithoutTransaction())
}

func TestUnit_ApplyModelFacts_DeclarationWinsOverObservation(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "gpt-5", runtimetypes.ModelFacts{
		ContextLength:   1000000,
		MaxOutputTokens: 393216,
		Capabilities:    []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityTools},
		Pricing:         &runtimetypes.ModelPricing{InputPerMillion: 1.25, OutputPerMillion: 10},
	}))

	got := state.applyModelFacts(ctx, "b1", ModelPullStatus{
		Model: "gpt-5", ContextLength: 32768, MaxOutputTokens: 4096, CanVision: true,
	})

	require.Equal(t, 1000000, got.ContextLength)
	require.Equal(t, 393216, got.MaxOutputTokens)
	require.Equal(t, []string{"completion", "tools"}, got.DeclaredCapabilities)
	require.True(t, got.CanChat)
	require.True(t, got.CanPrompt)
	require.True(t, got.CanStream)
	require.NotNil(t, got.Pricing)
	require.InDelta(t, 10.0, got.Pricing.OutputPerMillion, 1e-9)
}

func TestUnit_ApplyModelFacts_CapabilitiesReplaceTheObservedSet(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "m", runtimetypes.ModelFacts{
		Capabilities: []string{runtimetypes.CapabilityEmbedding},
	}))

	got := state.applyModelFacts(ctx, "b1", ModelPullStatus{
		Model: "m", CanChat: true, CanPrompt: true, CanStream: true, CanVision: true, CanThink: true, CanAudio: true,
	})

	require.False(t, got.CanChat)
	require.False(t, got.CanPrompt)
	require.False(t, got.CanStream)
	require.False(t, got.CanVision)
	require.False(t, got.CanThink)
	require.True(t, got.CanEmbed)
	require.False(t, got.CanAudio, "audio is a capability like the rest, so a set that omits it replaces it away")

	audio := state.applyModelFacts(ctx, "b1", ModelPullStatus{
		Model: "m2", CanVision: true,
	})
	require.True(t, audio.CanVision, "a model nobody declared is left as observed")
}

func TestUnit_ApplyModelFacts_DeclaredAudioCapability(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "gemma3n", runtimetypes.ModelFacts{
		Capabilities: []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityAudio},
	}))

	got := state.applyModelFacts(ctx, "b1", ModelPullStatus{Model: "gemma3n"})
	require.True(t, got.CanAudio, "an operator stating audio input is what routes an audio turn here")
	require.True(t, got.CanChat)
	require.False(t, got.CanVision)
}

func TestUnit_ApplyModelFacts_ScopedToItsBackend(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "m", runtimetypes.ModelFacts{ContextLength: 1000000}))

	other := state.applyModelFacts(ctx, "b2", ModelPullStatus{Model: "m", ContextLength: 32768})
	require.Equal(t, 32768, other.ContextLength, "another backend's declaration never applies")

	none := state.applyModelFacts(ctx, "b1", ModelPullStatus{Model: "other"})
	require.Equal(t, 0, none.ContextLength, "a model nobody declared is left as observed")
}

func TestUnit_ApplyModelFacts_PartialDeclarationLeavesTheRestObserved(t *testing.T) {
	ctx, state, store := newModelFactsTestState(t)
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "b1", "m", runtimetypes.ModelFacts{MaxOutputTokens: 8192}))

	got := state.applyModelFacts(ctx, "b1", ModelPullStatus{
		Model: "m", ContextLength: 4096, MaxOutputTokens: 4096, CanVision: true,
	})

	require.Equal(t, 4096, got.ContextLength)
	require.Equal(t, 8192, got.MaxOutputTokens)
	require.True(t, got.CanVision)
	require.Empty(t, got.DeclaredCapabilities)
}

func TestUnit_RunBackendCycle_AppliesDeclaredFacts(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-5"}},
		})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "runtime-facts-cycle.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()

	store := runtimetypes.New(db.WithoutTransaction())
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "openai-backend", Name: "openai", Type: "openai", BaseURL: server.URL,
	}))
	data, err := json.Marshal(ProviderConfig{APIKey: "test-key", Type: "openai"})
	require.NoError(t, err)
	require.NoError(t, store.SetKV(ctx, OpenaiKey, data))
	require.NoError(t, store.SetLLMProviderModelFact(ctx, "openai-backend", "gpt-5", runtimetypes.ModelFacts{
		ContextLength:   1000000,
		MaxOutputTokens: 393216,
		Capabilities:    []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityVision},
		Pricing:         &runtimetypes.ModelPricing{InputPerMillion: 1.25},
	}))

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	state, err := New(ctx, db, bus, WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, state.RunBackendCycle(ctx))

	rt := state.Get(ctx)
	require.Contains(t, rt, "openai-backend")
	require.Empty(t, rt["openai-backend"].Error)
	require.Len(t, rt["openai-backend"].PulledModels, 1)

	pulled := rt["openai-backend"].PulledModels[0]
	require.Equal(t, 1000000, pulled.ContextLength)
	require.Equal(t, 393216, pulled.MaxOutputTokens)
	require.Equal(t, []string{"completion", "vision"}, pulled.DeclaredCapabilities)
	require.True(t, pulled.CanVision)
	require.NotNil(t, pulled.Pricing)
	require.InDelta(t, 1.25, pulled.Pricing.InputPerMillion, 1e-9)
}

func TestSystem_AnEntrysOwnCredentialWinsOverTheTypeDefault(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "creds.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	state, err := New(ctx, db, libbus.NewInMem())
	require.NoError(t, err)
	store := runtimetypes.New(db.WithoutTransaction())

	typeCfg, err := json.Marshal(ProviderConfig{APIKey: "shared-default", Type: "openai"})
	require.NoError(t, err)
	require.NoError(t, store.SetKV(ctx, ProviderKeyPrefix+"openai", typeCfg))

	ownCfg, err := json.Marshal(ProviderConfig{APIKey: "second-account", Type: "openai"})
	require.NoError(t, err)
	require.NoError(t, store.SetKV(ctx, BackendCredentialKey("openai", "acct-b"), ownCfg))

	backend := &runtimetypes.Backend{ID: "acct-b", Name: "acct-b", Type: "openai", BaseURL: "https://api.openai.com/v1"}
	require.NoError(t, store.CreateBackend(ctx, backend))
	require.NoError(t, state.RunBackendCycle(ctx))

	for _, st := range state.Get(ctx) {
		if st.Backend.ID == "acct-b" {
			require.Equal(t, "second-account", st.GetAPIKey())
			return
		}
	}
	t.Fatal("the backend was not observed")
}
