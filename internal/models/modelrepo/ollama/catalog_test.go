package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/stretchr/testify/require"
)

func TestUnit_CatalogProvider_ListModels(t *testing.T) {
	tagsHit := false
	showHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tagsHit = true
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{
						"name":        "qwen3:8b",
						"model":       "qwen3:8b",
						"modified_at": time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
						"size":        12345,
						"digest":      "sha256:test",
						"details":     map[string]any{},
					},
				},
			})
		case "/api/show":
			showHit = true
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion","embedding"],"model_info":{"llama.context_length":4096}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{
		Type:    "ollama",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.True(t, tagsHit)
	require.True(t, showHit)
	require.Len(t, models, 1)

	model := models[0]
	require.Equal(t, "qwen3:8b", model.Name)
	require.Equal(t, 4096, model.ContextLength)
	require.True(t, model.CanChat)
	require.True(t, model.CanPrompt)
	require.True(t, model.CanStream)
	require.True(t, model.CanEmbed)
	require.False(t, model.CanThink, "Ollama show metadata must not infer thinking from qwen3 model names")
	require.Equal(t, int64(12345), model.Size)
	require.Equal(t, "sha256:test", model.Digest)

	provider := catalog.ProviderFor(model)
	require.Equal(t, "ollama", provider.GetType())
	require.Equal(t, "qwen3:8b", provider.ModelName())
	require.Equal(t, 4096, provider.GetContextLength(),
		"a window the catalogue read must reach the provider built from it")
	require.True(t, provider.CanEmbed())
	require.False(t, provider.CanThink())
	require.False(t, model.CanVision, "non-vision model must not claim vision")
}

// CanVision is read from /api/show capabilities, not inferred from the name.
func TestUnit_CatalogProvider_DetectsVisionFromShowCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "llava:7b", "model": "llava:7b", "details": map[string]any{}}},
			})
		case "/api/show":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion","vision"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "ollama", BaseURL: server.URL})
	require.NoError(t, err)
	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.True(t, models[0].CanVision, "vision capability from /api/show must set CanVision")
	require.True(t, catalog.ProviderFor(models[0]).CanVision())
}

// CanThink is read from /api/show capabilities, not inferred from the name.
func TestUnit_CatalogProvider_DetectsThinkingFromShowCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "qwen3:8b", "model": "qwen3:8b", "details": map[string]any{}}},
			})
		case "/api/show":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion","thinking"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "ollama", BaseURL: server.URL})
	require.NoError(t, err)
	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.True(t, models[0].CanThink, "thinking capability from /api/show must set CanThink")
	require.True(t, catalog.ProviderFor(models[0]).CanThink())
}

// MaxOutputTokens is read from the Modelfile-style parameters /api/show
// reports, which is where an Ollama server states num_predict.
func TestUnit_CatalogProvider_DetectsOutputCeilingFromShowParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "qwen3:8b", "model": "qwen3:8b", "details": map[string]any{}}},
			})
		case "/api/show":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":["completion"],"model_info":{"llama.context_length":4096},"parameters":"num_ctx 4096\nnum_predict 8192"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "ollama", BaseURL: server.URL})
	require.NoError(t, err)
	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 8192, models[0].MaxOutputTokens, "num_predict is the output ceiling")
	require.Equal(t, 8192, catalog.ProviderFor(models[0]).GetMaxOutputTokens())
}

// A parameter string that states no positive num_predict leaves the ceiling
// unknown, because Ollama spells "unlimited" as -1 and a clamp to zero would be
// a ceiling nobody stated.
func TestUnit_CatalogProvider_IgnoresNonPositiveNumPredict(t *testing.T) {
	require.Equal(t, 0, parameterInt("num_predict -1", "num_predict"))
	require.Equal(t, 0, parameterInt("num_predict 0", "num_predict"))
	require.Equal(t, 0, parameterInt("num_ctx 4096", "num_predict"))
	require.Equal(t, 0, parameterInt("", "num_predict"))
	require.Equal(t, 4096, parameterInt("stop \"<|end|>\"\nnum_predict 4096\nnum_ctx 8192", "num_predict"))
}

func TestUnit_ListModels_GatesAudioOnTheVersionHandshake(t *testing.T) {
	for name, tc := range map[string]struct {
		handshake string
		wantAudio bool
	}{
		"vanilla ollama": {``, false},
		"contenox gateway": {
			`{"product":"contenox-gateway","version":"v1.0.0","extensions":["audios"]}`,
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case ContenoxPath:
					if tc.handshake == "" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(tc.handshake))
				case "/api/tags":
					_, _ = w.Write([]byte(`{"models":[{"name":"gemma3n","model":"gemma3n"}]}`))
				case "/api/show":
					_, _ = w.Write([]byte(`{"capabilities":["completion","audio"]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "ollama", BaseURL: srv.URL},
				modelrepo.WithCatalogHTTPClient(srv.Client()))
			require.NoError(t, err)

			models, err := catalog.ListModels(context.Background())
			require.NoError(t, err)
			require.Len(t, models, 1)
			require.Equal(t, tc.wantAudio, models[0].CanAudio,
				"audio is only offered when the endpoint claimed the flavour that understands it")
			require.Equal(t, tc.wantAudio, models[0].CapabilityConfig.AudioExtension)

			provider := catalog.ProviderFor(models[0])
			require.Equal(t, tc.wantAudio, provider.CanAudio(),
				"what the catalog decided is what the resolver routes on")
		})
	}
}

// A conversation may only be named to a peer that said it understands the field,
// and the decision has to survive the trip from the handshake to the provider
// that writes the request body.
func TestUnit_ListModels_GatesTheSessionOnTheVersionHandshake(t *testing.T) {
	for name, tc := range map[string]struct {
		handshake   string
		wantSession bool
	}{
		"vanilla ollama": {``, false},
		"gateway without the extension": {
			`{"product":"contenox-gateway","version":"v1.0.0","extensions":["audios"]}`,
			false,
		},
		"gateway with it": {
			`{"product":"contenox-gateway","version":"v1.0.0","extensions":["audios","session"]}`,
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case ContenoxPath:
					if tc.handshake == "" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(tc.handshake))
				case "/api/tags":
					_, _ = w.Write([]byte(`{"models":[{"name":"gemma3n","model":"gemma3n"}]}`))
				case "/api/show":
					_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "ollama", BaseURL: srv.URL},
				modelrepo.WithCatalogHTTPClient(srv.Client()))
			require.NoError(t, err)

			models, err := catalog.ListModels(context.Background())
			require.NoError(t, err)
			require.Len(t, models, 1)
			require.Equal(t, tc.wantSession, models[0].CapabilityConfig.SessionExtension)

			provider, ok := catalog.ProviderFor(models[0]).(*OllamaProvider)
			require.True(t, ok)
			require.Equal(t, tc.wantSession, provider.SupportsSession)
		})
	}
}
