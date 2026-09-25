package llama

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUnit_ChatTemplateFor_ReadsTheSidecarBesideTheModel pins the resolution a
// quantised model depends on: the GGUF quants of most community builds carry no
// tokenizer.chat_template, so the template has to travel beside the weights or
// llama.cpp renders tools through its ChatML fallback and the calls never parse.
func TestUnit_ChatTemplateFor_ReadsTheSidecarBesideTheModel(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ChatTemplateFor(model); got != "" {
		t.Fatalf("a model with no sidecar must report none, got %q", got)
	}
	if p := ChatTemplatePathFor(model); p != "" {
		t.Fatalf("no sidecar, no path, got %q", p)
	}

	template := "{{ bos_token }}{% for m in messages %}{{ m['content'] }}{% endfor %}"
	if err := os.WriteFile(filepath.Join(dir, ChatTemplateFilename), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ChatTemplateFor(model); got != template {
		t.Fatalf("sidecar = %q, want the file's contents", got)
	}
	if got := ChatTemplatePathFor(model); got != filepath.Join(dir, ChatTemplateFilename) {
		t.Fatalf("path = %q", got)
	}

	// A sidecar that is a directory, or an empty model path, is not a template.
	if err := os.MkdirAll(filepath.Join(t.TempDir(), ChatTemplateFilename), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ChatTemplateFor(""); got != "" {
		t.Fatalf("an empty model path has no template, got %q", got)
	}
}
