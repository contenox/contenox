//go:build llamanode

package chattmpl

import (
	"strings"
	"testing"
)

// TestSystem_ChatTmpl_RendersToolsViaJinja proves minja executes a model's own
// Jinja chat template including the {%- if tools %} block that real GGUF templates
// (e.g. Qwen) use — the exact capability the legacy llama_chat_apply_template
// lacks. No model file is needed: minja is header-only.
func TestSystem_ChatTmpl_RendersToolsViaJinja(t *testing.T) {
	tmpl := "" +
		"{%- if tools %}{{- 'TOOLS:' }}{%- for t in tools %}{{- ' ' + t.function.name }}{%- endfor %}{{- '\n' }}{%- endif %}" +
		"{%- for m in messages %}{{- m.role + ': ' + m.content + '\n' }}{%- endfor %}" +
		"{%- if add_generation_prompt %}{{- 'assistant: ' }}{%- endif %}"

	messages := `[{"role":"user","content":"weather in SF?"}]`
	tools := `[{"type":"function","function":{"name":"get_weather","description":"get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`

	out, err := Render(tmpl, "<s>", "</s>", messages, tools, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "get_weather") {
		t.Fatalf("rendered prompt missing tool name (tools not executed): %q", out)
	}
	if !strings.Contains(out, "user: weather in SF?") {
		t.Fatalf("rendered prompt missing user message: %q", out)
	}
	if !strings.Contains(out, "assistant:") {
		t.Fatalf("rendered prompt missing assistant generation prompt: %q", out)
	}

	noTools, err := Render(tmpl, "<s>", "</s>", messages, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(noTools, "TOOLS:") {
		t.Fatalf("tools block rendered without tools present: %q", noTools)
	}
}
