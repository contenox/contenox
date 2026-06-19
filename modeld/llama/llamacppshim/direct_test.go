//go:build llamacpp_direct

package llamacppshim

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDirectShimBackendInfo(t *testing.T) {
	info := SystemInfo()
	if strings.TrimSpace(info) == "" {
		t.Fatal("SystemInfo returned empty string")
	}
	devices := Devices()
	if len(devices) == 0 {
		t.Fatal("expected at least one ggml backend device")
	}
	t.Logf("system_info=%s", info)
	for _, dev := range devices {
		t.Logf("device[%d] type=%s name=%q desc=%q free=%d total=%d",
			dev.Index, dev.Type, dev.Name, dev.Description, dev.MemoryFree, dev.MemoryTotal)
	}
}

func TestDirectShimLoadTinyModel(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if path == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to test direct model load/tokenize")
	}
	model, err := LoadModel(path, ModelConfig{UseMmap: true})
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()

	if got := model.ContextTrain(); got <= 0 {
		t.Fatalf("ContextTrain = %d, want > 0", got)
	}
	toks, err := model.Tokenize("hello from contenox", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) == 0 {
		t.Fatal("Tokenize returned no tokens")
	}
	t.Logf("model=%s n_ctx_train=%d tokens=%v", model.Description(), model.ContextTrain(), toks)
}

func TestDirectShimParseGenericToolCalls(t *testing.T) {
	parsed, err := ParseChatResponse(
		`{"tool_calls":[{"id":"call_1","name":"lookup","arguments":{"query":"x"}}]}`,
		false,
		ChatSyntax{Format: commonChatFormatGeneric()},
		"",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Content != "" || parsed.Thinking != "" {
		t.Fatalf("parsed content/thinking = %q/%q, want empty", parsed.Content, parsed.Thinking)
	}
	var calls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(parsed.ToolCallsJSON), &calls); err != nil {
		t.Fatalf("tool calls json: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Type != "function" || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments != `{"query":"x"}` {
		t.Fatalf("tool calls = %+v", calls)
	}
}
