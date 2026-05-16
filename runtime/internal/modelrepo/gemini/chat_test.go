package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/internal/modelrepo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestChatClient(srv *httptest.Server) *GeminiChatClient {
	return &GeminiChatClient{
		geminiClient: geminiClient{
			apiKey:     "test-key",
			modelName:  "gemini-test",
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			tracker:    libtracker.NoopTracker{},
		},
	}
}

func TestUnit_GeminiChat_ThinkingOnlyResponseIsNotAnError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"reasoning about the task","thought":true}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	res, err := newTestChatClient(srv).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})

	require.NoError(t, err, "Gemini returning only a thinking part (no final text, no tool call) must not be a hard 'empty content' error: that error is classified retryable, exhausts retries, fails the task, cascades acp_chat->recovery_chat->summarise_failure, and surfaces as a silent max_turn_requests with nothing rendered")
	assert.Equal(t, "", res.Message.Content)
	assert.Equal(t, "reasoning about the task", res.Message.Thinking)
	assert.Empty(t, res.ToolCalls)
}

func TestUnit_GeminiChat_EmptyPartsAreToleratedLikeOllama(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"thoughtSignature":"sig-only"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	res, err := newTestChatClient(srv).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})

	require.NoError(t, err, "a signature-only / empty turn on a normal finish reason must be tolerated as a degenerate end-of-turn signal, matching the Ollama handler, instead of cascading into a silent dead turn")
	assert.Equal(t, "", res.Message.Content)
	assert.Empty(t, res.ToolCalls)
}

func TestUnit_GeminiChat_NormalTextStillWorks(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hello there"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	res, err := newTestChatClient(srv).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})

	require.NoError(t, err)
	assert.Equal(t, "hello there", res.Message.Content)
}

func TestUnit_GeminiChat_BlockedPromptStillErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"candidates":[],"promptFeedback":{"blockReason":"SAFETY"}}`)
	}))
	defer srv.Close()

	_, err := newTestChatClient(srv).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})

	require.Error(t, err, "a genuinely blocked prompt (no candidates) must still surface an error, not be silently tolerated")
}
