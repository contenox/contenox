package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/runtime/modeld"
	"github.com/stretchr/testify/require"
)

func TestUnit_CatalogProvider_PaginatesListModels(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":     []map[string]any{{"id": "gpt-5"}},
				"has_more": true,
				"last_id":  "gpt-5",
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":     []map[string]any{{"id": "gpt-4o"}},
				"has_more": false,
			})
		}
	}))
	defer srv.Close()

	catalog, err := modeld.NewCatalogProvider(modeld.BackendSpec{Type: "openai", BaseURL: srv.URL, APIKey: "k"})
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, calls, "must have fetched two pages")
	require.Len(t, models, 2)
	names := []string{models[0].Name, models[1].Name}
	require.Contains(t, names, "gpt-5")
	require.Contains(t, names, "gpt-4o")
}

func TestUnit_CatalogProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-5"},
				{"id": "text-embedding-3-small"},
			},
		})
	}))
	defer server.Close()

	catalog, err := modeld.NewCatalogProvider(modeld.BackendSpec{
		Type:    "openai",
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)

	require.Equal(t, "gpt-5", models[0].Name)
	require.True(t, models[0].CanChat)
	require.True(t, models[0].CanPrompt)
	require.True(t, models[0].CanStream)
	require.False(t, models[0].CanEmbed)
	require.False(t, models[0].CanThink, "OpenAI /models does not expose reasoning capability metadata")
	require.Equal(t, 128000, models[0].MaxOutputTokens)

	require.Equal(t, "text-embedding-3-small", models[1].Name)
	require.False(t, models[1].CanChat)
	require.True(t, models[1].CanEmbed)
	require.Equal(t, 0, models[1].MaxOutputTokens)

	provider := catalog.ProviderFor(models[0])
	require.Equal(t, "openai", provider.GetType())
	require.Equal(t, "gpt-5", provider.ModelName())
	require.False(t, provider.CanThink())
}
