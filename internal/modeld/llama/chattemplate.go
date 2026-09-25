package llama

import (
	"os"
	"path/filepath"
	"strings"
)

// ChatTemplateFilename is the template a model directory may carry beside its
// weights. A GGUF written by the model's own publisher usually embeds
// tokenizer.chat_template, but the quantisation community strips it, and
// llama.cpp then falls back to its built-in ChatML template in silence: tools
// are never rendered in the model's dialect, the parser generated from that
// fallback recognises no call markers, and a model that can call tools answers
// in prose instead. The sidecar is how such a model keeps its dialect.
const ChatTemplateFilename = "chat_template.jinja"

// ChatTemplatePathFor resolves the template sidecar next to a model file, or ""
// when the directory carries none.
func ChatTemplatePathFor(modelPath string) string {
	if strings.TrimSpace(modelPath) == "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(modelPath), ChatTemplateFilename)
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return ""
	}
	return p
}

// ChatTemplateFor reads the template sidecar for a model, or "" when there is
// none to read. A sidecar that exists but cannot be read is reported as absent
// rather than fatal: llama.cpp still renders through whatever the GGUF carries,
// and a capability probe reports the difference instead of refusing the model.
func ChatTemplateFor(modelPath string) string {
	p := ChatTemplatePathFor(modelPath)
	if p == "" {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(data)
}
