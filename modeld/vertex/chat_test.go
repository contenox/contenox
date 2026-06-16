package vertex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
	"github.com/stretchr/testify/require"
)

func TestUnit_VertexChatClient_Chat(t *testing.T) {
	t.Parallel()

	var got vertexRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "), "expected ADC bearer token")
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.True(t, strings.HasSuffix(r.URL.Path, ":generateContent"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vertexResponse{
			Candidates: []struct {
				Content      vertexContent `json:"content"`
				FinishReason string        `json:"finishReason,omitempty"`
			}{
				{Content: vertexContent{
					Role:  "model",
					Parts: []vertexPart{{Text: "hello back"}},
				}},
			},
		})
	}))
	defer srv.Close()

	client := &vertexChatClient{
		vertexClient: vertexClient{
			baseURL:         srv.URL + "/v1/projects/test/locations/us-central1",
			publisher:       "google",
			modelName:       "gemini-flash-latest",
			maxOutputTokens: 512,
			httpClient: &http.Client{
				Transport: bearerInjectTransport{
					serverURL: srv.URL,
					token:     "fake-adc-token",
				},
			},
			tracker: libtracker.NoopTracker{},
			tokenFn: func(_ context.Context) (string, error) { return "fake-adc-token", nil },
		},
	}

	result, err := client.Chat(context.Background(), []modeld.Message{
		{Role: "user", Content: "hello"},
	}, modeld.WithMaxTokens(1000))
	require.NoError(t, err)
	require.Equal(t, "hello back", result.Message.Content)
	require.Equal(t, "assistant", result.Message.Role)
	require.NotNil(t, got.GenerationConfig)
	require.NotNil(t, got.GenerationConfig.MaxOutputTokens)
	require.Equal(t, 512, *got.GenerationConfig.MaxOutputTokens)
}

func TestUnit_BuildVertexRequest_MapsThinkingConfig(t *testing.T) {
	t.Parallel()
	msgs := []modeld.Message{{Role: "user", Content: "hi"}}

	req, err := buildVertexRequest("gemini-2.5-pro", msgs, []modeld.ChatArgument{modeld.WithThink("xhigh")})
	require.NoError(t, err)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	require.Equal(t, -1, *req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	require.Equal(t, "", req.GenerationConfig.ThinkingConfig.ThinkingLevel)

	req, err = buildVertexRequest("gemini-3-pro", msgs, []modeld.ChatArgument{modeld.WithThink("medium")})
	require.NoError(t, err)
	require.NotNil(t, req.GenerationConfig.ThinkingConfig)
	require.Nil(t, req.GenerationConfig.ThinkingConfig.ThinkingBudget)
	require.Equal(t, "high", req.GenerationConfig.ThinkingConfig.ThinkingLevel)

	req, err = buildVertexRequest("gemini-3-pro", msgs, []modeld.ChatArgument{modeld.WithThink("auto")})
	require.NoError(t, err)
	require.Nil(t, req.GenerationConfig.ThinkingConfig)

	req, err = buildVertexRequest("gemini-2.5-pro", msgs, []modeld.ChatArgument{modeld.WithThink("high")}, false)
	require.NoError(t, err)
	require.Nil(t, req.GenerationConfig.ThinkingConfig, "provider with CanThink=false must omit Vertex thinking config")
}

func TestUnit_BuildVertexRequest_RejectsEmptyContents(t *testing.T) {
	t.Parallel()

	_, err := buildVertexRequest("gemini-3.1-pro-preview",
		[]modeld.Message{{Role: "system", Content: "system only"}},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to send empty contents")
	require.Contains(t, err.Error(), "provide at least one non-empty")

	_, err = buildVertexRequest("gemini-3.1-pro-preview",
		[]modeld.Message{{Role: "user", Content: ""}},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to send empty contents")
}

func TestUnit_BuildVertexRequest_WrapsSchemaLikeToolResultAsText(t *testing.T) {
	t.Parallel()

	const schemaResult = `{"$defs":{"LogoutCapabilities":{"type":"object"}},"properties":{"logout":{"$ref":"#/$defs/LogoutCapabilities"}}}`
	msgs := []modeld.Message{
		{
			Role: "assistant",
			ToolCalls: []modeld.ToolCall{{
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

	req, err := buildVertexRequest("gemini-3.1-pro-preview", msgs, nil)
	require.NoError(t, err)
	require.Len(t, req.Contents, 2)
	resp := req.Contents[1].Parts[0].FunctionResponse.Response
	require.Equal(t, schemaResult, resp["content"])
	require.NotContains(t, resp, "$defs")
}

func TestUnit_BuildVertexRequest_KeepsNormalObjectToolResultStructured(t *testing.T) {
	t.Parallel()

	msgs := []modeld.Message{
		{
			Role: "assistant",
			ToolCalls: []modeld.ToolCall{{
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

	req, err := buildVertexRequest("gemini-3.1-pro-preview", msgs, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", req.Contents[1].Parts[0].FunctionResponse.Response["status"])
}
