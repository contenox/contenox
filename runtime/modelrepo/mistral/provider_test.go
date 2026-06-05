package mistral

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/stretchr/testify/require"
)

func TestUnit_MistralChat_RequestShapeAndResponse(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"hi there"}}]}`))
	}))
	defer srv.Close()

	p := NewMistralProvider("secret-key", "mistral-large-latest", []string{srv.URL},
		modelrepo.CapabilityConfig{CanChat: true}, srv.Client(), nil)
	chat, err := p.GetChatConnection(context.Background(), "")
	require.NoError(t, err)

	res, err := chat.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)

	require.True(t, strings.HasSuffix(gotPath, "/chat/completions"), "path was %q", gotPath)
	require.Equal(t, "Bearer secret-key", gotAuth)
	require.Equal(t, "mistral-large-latest", gotBody["model"], "model goes in the body")
	require.Equal(t, "hi there", res.Message.Content)
}

func TestUnit_MistralCatalog_DoesNotInferThinkingFromModelName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"magistral-medium-latest"}]}`))
	}))
	defer srv.Close()

	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "mistral", BaseURL: srv.URL})
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "magistral-medium-latest", models[0].Name)
	require.False(t, models[0].CanThink, "model name must not advertise Mistral thinking support")

	provider := catalog.ProviderFor(models[0])
	require.False(t, provider.CanThink())
}

func TestUnit_MistralProvider_CanThinkFromCapabilityConfigOnly(t *testing.T) {
	provider := NewMistralProvider("", "magistral-medium-latest", []string{"http://localhost"}, modelrepo.CapabilityConfig{}, nil, nil)
	require.False(t, provider.CanThink(), "model name alone must not set CanThink")

	provider = NewMistralProvider("", "custom", []string{"http://localhost"}, modelrepo.CapabilityConfig{CanThink: true}, nil, nil)
	require.True(t, provider.CanThink(), "explicit capability config must set CanThink")
}

func TestUnit_MistralCatalog_Registered(t *testing.T) {
	cp, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "mistral", APIKey: "k"})
	require.NoError(t, err, "mistral must be registered in the catalog registry")
	require.Equal(t, "mistral", cp.Type())

	prov := cp.ProviderFor(modelrepo.ObservedModel{
		Name:             "mistral-large-latest",
		CapabilityConfig: modelrepo.CapabilityConfig{CanChat: true},
	})
	require.Equal(t, "mistral", prov.GetType())
	require.True(t, prov.CanChat())
}
