package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
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

func TestUnit_GeminiChat_ClampsMaxOutputTokens(t *testing.T) {
	t.Parallel()

	var got geminiGenerateContentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	client := newTestChatClient(srv)
	client.maxOutputTokens = 128
	_, err := client.Chat(context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "hi"}},
		modelrepo.WithMaxTokens(999),
	)
	require.NoError(t, err)
	require.NotNil(t, got.GenerationConfig)
	require.NotNil(t, got.GenerationConfig.MaxOutputTokens)
	assert.Equal(t, 128, *got.GenerationConfig.MaxOutputTokens)
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

func TestUnit_BuildGeminiRequest_MapsThinkingConfig(t *testing.T) {
	t.Parallel()
	msgs := []modelrepo.Message{{Role: "user", Content: "hi"}}

	req, err := buildGeminiRequest("gemini-2.5-pro", msgs, nil, []modelrepo.ChatArgument{modelrepo.WithThink("medium")})
	require.NoError(t, err)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	assert.Equal(t, 8192, *req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	assert.Equal(t, "", req.GenerationConfig.ThinkingConfig.ThinkingLevel)

	req, err = buildGeminiRequest("gemini-3-flash", msgs, nil, []modelrepo.ChatArgument{modelrepo.WithThink("off")})
	require.NoError(t, err)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig)
	require.Nil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	assert.Equal(t, "minimal", req.GenerationConfig.ThinkingConfig.ThinkingLevel)

	req, err = buildGeminiRequest("gemini-3-pro", msgs, nil, []modelrepo.ChatArgument{modelrepo.WithThink("auto")})
	require.NoError(t, err)
	require.Nil(t, req.GenerationConfig.ThinkingConfig)

	req, err = buildGeminiRequest("gemini-2.5-pro", msgs, nil, []modelrepo.ChatArgument{modelrepo.WithThink("medium")}, false)
	require.NoError(t, err)
	require.Nil(t, req.GenerationConfig.ThinkingConfig, "provider with CanThink=false must omit Gemini thinking config")
}

func TestUnit_BuildGeminiRequest_RejectsEmptyContents(t *testing.T) {
	t.Parallel()

	_, err := buildGeminiRequest("gemini-3.1-pro-preview",
		[]modelrepo.Message{{Role: "system", Content: "system only"}},
		&geminiSystemInstruction{Parts: []geminiPart{{Text: "system only"}}},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to send empty contents")
	require.Contains(t, err.Error(), "provide at least one non-empty")

	_, err = buildGeminiRequest("gemini-3.1-pro-preview",
		[]modelrepo.Message{{Role: "user", Content: ""}},
		nil,
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to send empty contents")
}

func TestUnit_BuildGeminiRequest_WrapsSchemaLikeToolResultAsText(t *testing.T) {
	t.Parallel()

	const schemaResult = `{"$defs":{"LogoutCapabilities":{"type":"object"}},"properties":{"logout":{"$ref":"#/$defs/LogoutCapabilities"}}}`
	msgs := []modelrepo.Message{
		{
			Role: "assistant",
			ToolCalls: []modelrepo.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "webtools.web_get", Arguments: `{"url":"https://example.test/schema.json"}`},
			}},
		},
		{Role: "tool", ToolCallID: "call-1", Content: schemaResult},
	}

	req, err := buildGeminiRequest("gemini-3.1-pro-preview", msgs, nil, nil)
	require.NoError(t, err)
	require.Len(t, req.Contents, 2)
	resp := req.Contents[1].Parts[0].FunctionResponse.Response
	require.Equal(t, schemaResult, resp["content"])
	require.NotContains(t, resp, "$defs")
}

func TestUnit_BuildGeminiRequest_KeepsNormalObjectToolResultStructured(t *testing.T) {
	t.Parallel()

	msgs := []modelrepo.Message{
		{
			Role: "assistant",
			ToolCalls: []modelrepo.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "webtools.web_get", Arguments: `{"url":"https://example.test/data.json"}`},
			}},
		},
		{Role: "tool", ToolCallID: "call-1", Content: `{"status":"ok"}`},
	}

	req, err := buildGeminiRequest("gemini-3.1-pro-preview", msgs, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", req.Contents[1].Parts[0].FunctionResponse.Response["status"])
}
