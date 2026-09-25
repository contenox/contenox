package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

// sentSession drives one real chat turn at a live endpoint and reports the
// session that arrived on the request body, which is the only place a client's
// mistake would be invisible.
func sentSession(t *testing.T, sessionKey string, declared bool) string {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop"}`))
	}))
	t.Cleanup(srv.Close)

	client, err := newOllamaHTTPClient(srv.URL, "", srv.Client())
	require.NoError(t, err)

	instance := &OllamaChatClient{
		ollamaClient:     client,
		modelName:        "m",
		backendURL:       srv.URL,
		sessionExtension: declared,
		tracker:          libtracker.NoopTracker{},
	}
	_, err = instance.Chat(context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "hi"}},
		modelrepo.WithCacheHints(modelrepo.CacheHints{SessionKey: sessionKey}))
	require.NoError(t, err)

	sent, _ := body["contenox_session"].(string)
	return sent
}

// The session reaches the provider as the cache-affinity key the model layer
// routed on, so the backend this request went to is the one that key pinned.
func TestSystem_AClientNamesTheConversationOnTheWire(t *testing.T) {
	require.Equal(t, "conversation-7", sentSession(t, "conversation-7", true))
}

// A peer that never declared the extension must not receive the field: upstream
// Ollama has no such key.
func TestSystem_AClientNamesNoConversationToAPeerWithoutTheExtension(t *testing.T) {
	require.Empty(t, sentSession(t, "conversation-7", false))
}

func TestSystem_AClientNamesNoConversationWithoutOne(t *testing.T) {
	require.Empty(t, sentSession(t, "", true))
}

func TestUnit_SessionOfNeedsHintsAndADeclaration(t *testing.T) {
	require.Empty(t, sessionOf(nil, true))
	require.Empty(t, sessionOf(&modelrepo.ChatConfig{}, true))
	require.Empty(t, sessionOf(&modelrepo.ChatConfig{CacheHints: &modelrepo.CacheHints{SessionKey: "s"}}, false))
	require.Equal(t, "s", sessionOf(&modelrepo.ChatConfig{CacheHints: &modelrepo.CacheHints{SessionKey: "s"}}, true))
}
