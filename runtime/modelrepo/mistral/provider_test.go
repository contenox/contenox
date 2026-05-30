package mistral

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/agent/runtime/modelrepo"
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
