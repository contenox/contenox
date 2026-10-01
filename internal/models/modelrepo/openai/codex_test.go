package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	_ "github.com/contenox/contenox/internal/models/modelrepo/openai"
	"github.com/stretchr/testify/require"
)

type codexTransport func(*http.Request) (*http.Response, error)

func (f codexTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func codexFixture(t *testing.T, handler http.HandlerFunc) modelrepo.CatalogProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &http.Client{Transport: codexTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "chatgpt.com", r.URL.Host)
		require.Equal(t, "https", r.URL.Scheme)
		require.True(t, strings.HasPrefix(r.URL.Path, "/backend-api/codex/"))
		forward := r.Clone(r.Context())
		url := *r.URL
		forward.URL = &url
		forward.URL.Scheme = "http"
		forward.URL.Host = strings.TrimPrefix(server.URL, "http://")
		forward.URL.Path = strings.TrimPrefix(r.URL.Path, "/backend-api/codex")
		return server.Client().Transport.RoundTrip(forward)
	})}
	catalog, err := modelrepo.NewCatalogProvider(
		modelrepo.BackendSpec{Type: modelauth.ProviderType, BaseURL: modelauth.BaseURL},
		modelrepo.WithCatalogHTTPClient(client),
		modelrepo.WithCatalogAuthorizer(func(context.Context) (http.Header, error) {
			return http.Header{"Authorization": {"Bearer secret"}, "Chatgpt-Account-Id": {"account"}}, nil
		}),
	)
	require.NoError(t, err)
	return catalog
}

func codexChat(t *testing.T, c modelrepo.CatalogProvider) modelrepo.LLMChatClient {
	t.Helper()
	p := c.ProviderFor(modelrepo.ObservedModel{Name: "codex-test", CapabilityConfig: modelrepo.CapabilityConfig{CanChat: true, CanStream: true}})
	client, err := p.GetChatConnection(context.Background(), "")
	require.NoError(t, err)
	return client
}

func TestUnit_CodexCatalog(t *testing.T) {
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "0.158.0", r.URL.Query().Get("client_version"))
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.Equal(t, "account", r.Header.Get("chatgpt-account-id"))
		fmt.Fprint(w, `{"models":[{"slug":"codex-test","visibility":"list","context_window":32000,"input_modalities":["text","image"],"supported_reasoning_levels":[{"effort":"low"}]},{"slug":"hidden","visibility":"hide"}]}`)
	})
	models, err := c.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	p := c.ProviderFor(models[0])
	require.Equal(t, modelauth.ProviderType, p.GetType())
	require.True(t, p.CanChat())
	require.True(t, p.CanThink())
	require.False(t, p.CanEmbed())
	require.True(t, p.CanVision())
	require.False(t, p.CanAudio())
}

func TestUnit_CodexToolRoundTrip(t *testing.T) {
	for _, output := range []string{"result", ""} {
		t.Run("output="+output, func(t *testing.T) { testCodexToolRoundTrip(t, output) })
	}
}

func testCodexToolRoundTrip(t *testing.T, toolOutput string) {
	t.Helper()
	calls := 0
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/responses", r.URL.Path)
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.JSONEq(t, `true`, string(body["stream"]))
		require.JSONEq(t, `false`, string(body["store"]))
		for _, key := range []string{"max_output_tokens", "temperature", "top_p", "seed"} {
			require.NotContains(t, body, key)
		}
		require.Contains(t, string(body["include"]), "reasoning.encrypted_content")
		var input []map[string]any
		require.NoError(t, json.Unmarshal(body["input"], &input))
		require.IsType(t, []any{}, input[0]["content"])
		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 {
			fmt.Fprint(w, "data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"probe\"}}\n\n")
			fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"delta\":\"{}\"}\n\n")
			fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"reasoning\",\"id\":\"rs_1\",\"encrypted_content\":\"opaque\",\"summary\":[]},{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"probe\",\"arguments\":\"{}\"}],\"usage\":{\"input_tokens\":10,\"output_tokens\":3}}}\n\n")
		} else {
			require.Equal(t, "reasoning", input[1]["type"])
			require.Equal(t, "opaque", input[1]["encrypted_content"])
			require.Equal(t, "function_call", input[2]["type"])
			require.Equal(t, "call_1", input[2]["call_id"])
			require.Equal(t, "function_call_output", input[3]["type"])
			require.Contains(t, input[3], "output")
			require.Equal(t, toolOutput, input[3]["output"])
			fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"result\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
		}
	})
	p := codexChat(t, c)
	inputs := []modelrepo.Message{{Role: "user", Content: "probe"}}
	first, err := p.Chat(context.Background(), inputs)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.NotNil(t, first.Message.Continuation)
	require.Equal(t, 13, first.Usage.TotalTokens)
	first.Message.ToolCalls = first.ToolCalls
	raw, err := json.Marshal(first.Message)
	require.NoError(t, err)
	var resumed modelrepo.Message
	require.NoError(t, json.Unmarshal(raw, &resumed))
	inputs = append(inputs, resumed, modelrepo.Message{Role: "tool", ToolCallID: "call_1", Content: toolOutput})
	last, err := p.Chat(context.Background(), inputs)
	require.NoError(t, err)
	require.Equal(t, "result", last.Message.Content)
	require.Equal(t, 2, calls)
}

func TestUnit_CodexDoesNotReplayForeignContinuation(t *testing.T) {
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		require.NotContains(t, string(raw), "opaque")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
	})
	p := codexChat(t, c)
	for _, tag := range []*modelrepo.Continuation{
		{Provider: "other", Model: "codex-test", Items: json.RawMessage(`[{"encrypted_content":"opaque"}]`)},
		{Provider: modelauth.ProviderType, Model: "other", Items: json.RawMessage(`[{"encrypted_content":"opaque"}]`)},
	} {
		_, err := p.Chat(context.Background(), []modelrepo.Message{{Role: "assistant", Content: "hello", Continuation: tag}})
		require.NoError(t, err)
	}
}

func TestUnit_CodexStreamFailures(t *testing.T) {
	for _, body := range []string{"data: {broken}\n\n", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", "data: {\"type\":\"response.incomplete\"}\n\n"} {
		t.Run(strings.ReplaceAll(body, "\n", ""), func(t *testing.T) {
			c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			p := codexChat(t, c)
			_, err := p.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})
			require.Error(t, err)
		})
	}
}

func TestUnit_CodexErrorsAndOriginValidation(t *testing.T) {
	for _, status := range []int{400, 401, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"detail":"model unavailable secret account"}`)
			})
			_, err := c.ListModels(context.Background())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
			if status == 401 {
				require.ErrorIs(t, err, modelauth.ErrLoginRequired)
				require.ErrorIs(t, err, modelrepo.ErrModelAccessDenied)
			}
			if status == 429 {
				require.ErrorIs(t, err, modelrepo.ErrRateLimited)
			}
		})
	}
	_, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: modelauth.ProviderType, BaseURL: "https://elsewhere.test"})
	require.Error(t, err)
	_, err = modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: modelauth.ProviderType, BaseURL: modelauth.BaseURL, APIKey: "key"})
	require.Error(t, err)
}

func TestUnit_CodexInstructionsAndImages(t *testing.T) {
	for _, system := range []string{"", "Review the image. Do not edit files."} {
		t.Run(system, func(t *testing.T) {
			c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Instructions *string `json:"instructions"`
					Input        []struct {
						Role    string `json:"role"`
						Content []struct {
							Type     string `json:"type"`
							Text     string `json:"text"`
							ImageURL string `json:"image_url"`
						} `json:"content"`
					} `json:"input"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.NotNil(t, body.Instructions)
				require.Equal(t, system, *body.Instructions)
				require.Len(t, body.Input, 1)
				require.Equal(t, "user", body.Input[0].Role)
				require.Len(t, body.Input[0].Content, 2)
				require.Equal(t, "input_text", body.Input[0].Content[0].Type)
				require.Equal(t, "Describe this", body.Input[0].Content[0].Text)
				require.Equal(t, "input_image", body.Input[0].Content[1].Type)
				require.Equal(t, "data:image/png;base64,AQID", body.Input[0].Content[1].ImageURL)
				fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
			})
			p := c.ProviderFor(modelrepo.ObservedModel{Name: "codex-test", CapabilityConfig: modelrepo.CapabilityConfig{CanChat: true, CanVision: true}})
			require.True(t, p.CanVision())
			client, err := p.GetChatConnection(context.Background(), "")
			require.NoError(t, err)
			_, err = client.Chat(context.Background(), []modelrepo.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: "Describe this", Images: []modelrepo.ImagePart{{MimeType: "image/png", Data: []byte{1, 2, 3}}}},
			})
			require.NoError(t, err)
		})
	}
}

func TestUnit_CodexConnectionCapabilities(t *testing.T) {
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected HTTP request") })
	p := c.ProviderFor(modelrepo.ObservedModel{Name: "codex-test"})
	_, err := p.GetChatConnection(context.Background(), "")
	require.Error(t, err)
	_, err = p.GetPromptConnection(context.Background(), "")
	require.Error(t, err)
	_, err = p.GetStreamConnection(context.Background(), "")
	require.Error(t, err)
	_, err = p.GetEmbedConnection(context.Background(), "")
	require.Error(t, err)
}

func TestUnit_CodexClassifiesContextErrors(t *testing.T) {
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"secret account"}}`)
	})
	_, err := codexChat(t, c).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hello"}})
	require.ErrorIs(t, err, modelrepo.ErrContextLengthExceeded)
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "account")
}

func TestUnit_CodexHTTPDiagnostics(t *testing.T) {
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-test")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"code":"invalid_request_error","message":"Item rs_123 is missing its following item; secret account"}}`)
	})
	_, err := c.ListModels(context.Background())
	require.ErrorContains(t, err, "Item rs_123 is missing its following item")
	require.ErrorContains(t, err, "invalid_request_error")
	require.ErrorContains(t, err, "req-test")
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "account")
	var httpErr *modelrepo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
}

func TestUnit_CodexChatCancellation(t *testing.T) {
	started := make(chan struct{})
	c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := codexChat(t, c)
	done := make(chan error, 1)
	go func() {
		_, err := client.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "hello"}})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop the stream")
	}
}

func TestUnit_CodexStreamClassifiesProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		code string
		want error
	}{
		{"context_length_exceeded", modelrepo.ErrContextLengthExceeded},
		{"rate_limit_error", modelrepo.ErrRateLimited},
	} {
		t.Run(tc.code, func(t *testing.T) {
			c := codexFixture(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":%q,\"message\":\"request rejected\"}}}\n\n", tc.code)
			})
			_, err := codexChat(t, c).Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})
			require.ErrorIs(t, err, tc.want)
		})
	}
}
