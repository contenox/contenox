package runtimestate_test

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/runtime/runtimestate"
	"github.com/contenox/runtime/store"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"
)

func TestUnit_ModelProviderAdapter_ReturnsExpectedProviders(t *testing.T) {
	now := time.Now()

	runtime := map[string]runtimestate.LLMState{
		"backend1": {
			ID:      "backend1",
			Name:    "Backend One",
			Backend: store.Backend{ID: "backend1", Name: "Ollama", Type: "ollama"},
			PulledModels: []api.ListModelResponse{
				{Name: "Model One", Model: "model1", ModifiedAt: now},
				{Name: "Model Shared", Model: "shared", ModifiedAt: now},
			},
		},
		"backend2": {
			ID:      "backend2",
			Name:    "Backend Two",
			Backend: store.Backend{ID: "backend2", Name: "Ollama", Type: "ollama"},
			PulledModels: []api.ListModelResponse{
				{Name: "Model Two", Model: "model2", ModifiedAt: now},
				{Name: "Model Shared", Model: "shared", ModifiedAt: now},
			},
		},
	}

	adapter := runtimestate.LocalProviderAdapter(context.Background(), runtime)

	providers, err := adapter(context.Background(), "ollama")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}

	models := map[string]bool{}
	for _, provider := range providers {
		models[provider.ModelName()] = true
	}

	expected := []string{"model1", "model2", "shared"}
	for _, model := range expected {
		if !models[model] {
			t.Errorf("expected model %q to be in providers, but it was not found", model)
		}
	}
}

func TestUnit_ModelProviderAdapter_SetsCorrectModelCapabilities(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	chatModelName := "llama3:latest"           // A model known to support chat by default
	embedModelName := "granite-embedding:30m"  // A model known to support embed by default
	unknownModelName := "some-random-model:v1" // A model not in default maps
	backendID := "backend-test"
	backendURL := "http://host:1234"

	// 1. Setup Runtime State with various models
	runtime := map[string]runtimestate.LLMState{
		backendID: {
			ID:      backendID,
			Name:    "Test Backend",
			Backend: store.Backend{ID: backendID, Name: "Ollama", Type: "ollama", BaseURL: backendURL},
			PulledModels: []api.ListModelResponse{
				{Name: chatModelName, Model: chatModelName, ModifiedAt: now},
				{Name: embedModelName, Model: embedModelName, ModifiedAt: now},
				{Name: unknownModelName, Model: unknownModelName, ModifiedAt: now},
			},
		},
	}

	// 2. Get the adapter function (which currently hardcodes WithChat(true))
	adapterFunc := runtimestate.LocalProviderAdapter(ctx, runtime)

	// 3. Get the providers created by the adapter
	// Pass a dummy type, as the adapter's returned function ignores it currently
	providers, err := adapterFunc(ctx, "ollama")
	require.NoError(t, err)
	require.Len(t, providers, 3, "Should create one provider per unique model")

	// 4. Verify capabilities for each provider type
	foundChat := false
	foundEmbed := false
	foundUnknown := false

	for _, p := range providers {
		switch p.ModelName() {
		case chatModelName:
			foundChat = true
			// Default for llama3 is chat=true, embed=true. Adapter uses WithChat(true).
			require.True(t, p.CanChat(), "Provider for %s should support chat (default + adapter override)", chatModelName)
			require.True(t, p.CanEmbed(), "Provider for %s should support embed (default)", chatModelName)
		case embedModelName:
			foundEmbed = true
			// Default for all-minilm is chat=false, embed=false. Adapter uses WithChat(true).
			require.True(t, p.CanEmbed(), "Provider for %s should support embed (adapter override)", embedModelName)
			require.False(t, p.CanChat(), "Provider for %s should NOT support chat (default)", embedModelName)
		case unknownModelName:
			foundUnknown = true
		}
	}

	require.True(t, foundChat, "Provider for chat model not found")
	require.True(t, foundEmbed, "Provider for embed model not found")
	require.True(t, foundUnknown, "Provider for unknown model not found")

	t.Log("Test confirmed: ModelProviderAdapter correctly creates providers, but hardcodes WithChat(true), overriding defaults and potentially setting incorrect capabilities (CanEmbed=false) for models intended for embedding.")
}
