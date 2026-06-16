package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/modeld"
	"github.com/stretchr/testify/require"
)

func TestUnit_OpenRouterCatalog_PreservesTopProviderMaxCompletionTokens(t *testing.T) {
	m := orModel{ID: "deepseek/deepseek-chat-v3-5"}
	m.Architecture.Modality = "text->text"
	m.ContextLength = 128000
	m.TopProvider.ContextLength = 64000
	m.TopProvider.MaxCompletionTokens = 8192

	observed, ok := toObservedModel(m)
	require.True(t, ok)
	require.Equal(t, "deepseek/deepseek-chat-v3-5", observed.Name)
	require.Equal(t, 64000, observed.ContextLength)
	require.Equal(t, 8192, observed.MaxOutputTokens)
	require.True(t, observed.CanChat)
}

func TestUnit_OpenRouterChat_ClampsMaxTokens(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := newOpenRouterProvider("secret-key", "deepseek/deepseek-chat-v3-5", srv.URL,
		modeld.CapabilityConfig{CanChat: true, MaxOutputTokens: 2048}, srv.Client(), nil)
	chat, err := p.GetChatConnection(context.Background(), "")
	require.NoError(t, err)

	res, err := chat.Chat(context.Background(), []modeld.Message{{Role: "user", Content: "hi"}}, modeld.WithMaxTokens(4096))
	require.NoError(t, err)

	require.True(t, strings.HasSuffix(gotPath, "/chat/completions"), "path was %q", gotPath)
	require.Equal(t, "Bearer secret-key", gotAuth)
	require.Equal(t, float64(2048), gotBody["max_tokens"])
	require.Equal(t, "ok", res.Message.Content)
}
