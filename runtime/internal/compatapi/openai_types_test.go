package compatapi_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/compatapi"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

// TestChatMessage_ContentUnion covers both OpenAI wire forms for message
// content: the plain string and the content-parts array (text + image_url
// with a data: URI).
func TestChatMessage_ContentUnion(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}
	pngB64 := base64.StdEncoding.EncodeToString(pngBytes)

	t.Run("plain string content", func(t *testing.T) {
		var msg compatapi.ChatMessage
		if err := json.Unmarshal([]byte(`{"role":"user","content":"hello"}`), &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if msg.Role != "user" || msg.Content != "hello" {
			t.Errorf("got role=%q content=%q", msg.Role, msg.Content)
		}
		if len(msg.Images) != 0 {
			t.Errorf("expected no images, got %d", len(msg.Images))
		}
	})

	t.Run("content-parts array with text and data: URI image", func(t *testing.T) {
		body := `{"role":"user","content":[` +
			`{"type":"text","text":"what is this?"},` +
			`{"type":"image_url","image_url":{"url":"data:image/png;base64,` + pngB64 + `"}}]}`
		var msg compatapi.ChatMessage
		if err := json.Unmarshal([]byte(body), &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if msg.Content != "what is this?" {
			t.Errorf("got content %q", msg.Content)
		}
		if len(msg.Images) != 1 {
			t.Fatalf("expected 1 image, got %d", len(msg.Images))
		}
		if msg.Images[0].MimeType != "image/png" {
			t.Errorf("got mime type %q", msg.Images[0].MimeType)
		}
		if string(msg.Images[0].Data) != string(pngBytes) {
			t.Errorf("image bytes did not round-trip")
		}
	})

	t.Run("multiple text parts are joined", func(t *testing.T) {
		body := `{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`
		var msg compatapi.ChatMessage
		if err := json.Unmarshal([]byte(body), &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if msg.Content != "a\nb" {
			t.Errorf("got content %q", msg.Content)
		}
	})

	t.Run("remote image URL is rejected", func(t *testing.T) {
		body := `{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}]}`
		var msg compatapi.ChatMessage
		err := json.Unmarshal([]byte(body), &msg)
		if err == nil || !strings.Contains(err.Error(), "data: URI") {
			t.Errorf("expected data: URI rejection, got %v", err)
		}
	})

	t.Run("non-base64 data URI is rejected", func(t *testing.T) {
		body := `{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png,rawbytes"}}]}`
		var msg compatapi.ChatMessage
		err := json.Unmarshal([]byte(body), &msg)
		if err == nil || !strings.Contains(err.Error(), "base64") {
			t.Errorf("expected base64 rejection, got %v", err)
		}
	})

	t.Run("unknown part type is rejected", func(t *testing.T) {
		body := `{"role":"user","content":[{"type":"input_audio"}]}`
		var msg compatapi.ChatMessage
		err := json.Unmarshal([]byte(body), &msg)
		if err == nil || !strings.Contains(err.Error(), "unsupported content part") {
			t.Errorf("expected unsupported-part rejection, got %v", err)
		}
	})

	t.Run("response message still encodes content as a plain string", func(t *testing.T) {
		out, err := json.Marshal(compatapi.ChatMessage{Role: "assistant", Content: "done"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(out) != `{"role":"assistant","content":"done"}` {
			t.Errorf("got %s", out)
		}
	})
}

// TestChatCompletions_ImageParts drives the full handler with a content-parts
// request and asserts the decoded images reach the agent's chat history.
func TestChatCompletions_ImageParts(t *testing.T) {
	agent := &stubAgent{reply: "a cat"}
	chains := &stubChains{}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: chains,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47}
	pngB64 := base64.StdEncoding.EncodeToString(pngBytes)
	body := `{"model":"default","messages":[{"role":"user","content":[` +
		`{"type":"text","text":"describe this"},` +
		`{"type":"image_url","image_url":{"url":"data:image/png;base64,` + pngB64 + `"}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	hist, ok := agent.lastReq.InputValue.(taskengine.ChatHistory)
	if !ok {
		t.Fatalf("expected ChatHistory input, got %T", agent.lastReq.InputValue)
	}
	if len(hist.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(hist.Messages))
	}
	msg := hist.Messages[0]
	if msg.Content != "describe this" {
		t.Errorf("got content %q", msg.Content)
	}
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 image on the engine message, got %d", len(msg.Images))
	}
	if msg.Images[0].MimeType != "image/png" || string(msg.Images[0].Data) != string(pngBytes) {
		t.Errorf("image attachment did not survive the handler: %+v", msg.Images[0])
	}
}
