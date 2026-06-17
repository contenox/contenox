//go:build llamanode && llama_unsafe_abi

package llamaabi

import (
	"os"
	"strings"
	"testing"

	llamacpp "github.com/ollama/ollama/llama"
)

// TestShim_ModelChatTemplateAndApply proves the owned-ABI shim reads the model's
// OWN chat template from the GGUF and applies it via llama.cpp — model-driven,
// not a hardcoded chatml/llama3 render — plus the model-driven BOS policy.
func TestShim_ModelChatTemplateAndApply(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if path == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}
	m, err := llamacpp.LoadModelFromFile(path, llamacpp.ModelParams{UseMmap: true})
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	tmpl := ModelChatTemplate(m)
	if tmpl == "" {
		t.Fatal("model declares no chat template")
	}
	t.Logf("model chat_template[:80] = %q", tmpl[:min(80, len(tmpl))])

	out, err := ApplyChatTemplate(m, []ChatMessage{
		{Role: "system", Content: "You are precise."},
		{Role: "user", Content: "hello"},
	}, true)
	if err != nil {
		t.Fatalf("apply chat template: %v", err)
	}
	t.Logf("applied = %q", out)
	if strings.TrimSpace(out) == "" {
		t.Fatal("applied template is empty")
	}
	t.Logf("add_bos = %v", AddBOS(m))
}

// TestShim_ApplyChatTemplateToolsModelNative proves the model's OWN GGUF Jinja
// chat template renders tool definitions through minja — the capability the legacy
// llama_chat_apply_template lacks. The tiny Qwen template has a {%- if tools %}
// block, so the declared tool name must appear in the rendered prompt.
func TestShim_ApplyChatTemplateToolsModelNative(t *testing.T) {
	path := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if path == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}
	m, err := llamacpp.LoadModelFromFile(path, llamacpp.ModelParams{UseMmap: true})
	if err != nil {
		t.Fatalf("load model: %v", err)
	}

	messages := `[{"role":"user","content":"weather in SF?"}]`
	tools := `[{"type":"function","function":{"name":"get_weather","description":"Get current weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]`

	withTools, err := ApplyChatTemplateTools(m, messages, tools, true)
	if err != nil {
		t.Fatalf("apply chat template with tools: %v", err)
	}
	t.Logf("rendered-with-tools = %q", withTools)
	if !strings.Contains(withTools, "get_weather") {
		t.Fatalf("model-native tool rendering missing tool name: %q", withTools)
	}

	noTools, err := ApplyChatTemplateTools(m, messages, "", true)
	if err != nil {
		t.Fatalf("apply chat template without tools: %v", err)
	}
	if strings.Contains(noTools, "get_weather") {
		t.Fatalf("tool name leaked into no-tools render: %q", noTools)
	}
}
