package contenoxcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/transport"
)

func TestUnit_DisplayModelNameStripsGeminiResourcePrefix(t *testing.T) {
	if got := displayModelName("models/gemini-3.1-pro-preview"); got != "gemini-3.1-pro-preview" {
		t.Fatalf("displayModelName stripped = %q", got)
	}
	if got := displayModelName("openai/gpt-5"); got != "openai/gpt-5" {
		t.Fatalf("displayModelName must not strip non-Gemini-looking names: %q", got)
	}
}

func TestUnit_ModelList_HidesInactiveEmptyLocalBackend(t *testing.T) {
	if !hideInactiveLocalBackend("openvino", "llama", "", nil) {
		t.Fatal("inactive openvino backend should be hidden while modeld serves llama")
	}
	if !hideInactiveLocalBackend("llama", "", "", nil) {
		t.Fatal("local backend should be hidden when modeld is not running")
	}
	if hideInactiveLocalBackend("llama", "llama", "", nil) {
		t.Fatal("active llama backend should stay visible even when empty")
	}
	if hideInactiveLocalBackend("openvino", "llama", "permission denied", nil) {
		t.Fatal("backend errors should stay visible")
	}
	if hideInactiveLocalBackend("ollama", "", "", nil) {
		t.Fatal("non-modeld backends should not be hidden by modeld state")
	}
}

func TestUnit_LocalModelInventoryScansInstalledArtifactsWithoutModeld(t *testing.T) {
	root := t.TempDir()
	llamaDir := filepath.Join(root, "qwen")
	if err := os.MkdirAll(llamaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(llamaDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanLocalModelRoot("local-llama", "llama", root, transport.DaemonStatus{})
	if err != nil {
		t.Fatalf("scanLocalModelRoot: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %+v, want one", got)
	}
	if got[0].Model != "qwen" || got[0].Path != modelPath || got[0].Status != "installed" {
		t.Fatalf("entry = %+v", got[0])
	}
}

func TestUnit_LocalModelInventoryScansLlamaVariant(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "qwen")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseModelPath := filepath.Join(baseDir, "model.gguf")
	if err := os.WriteFile(baseModelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	variantDir := filepath.Join(root, "qwen-acme")
	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "acme.gguf"), []byte("adapter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "contenox-variant.json"), []byte(`{
		"base_model":"qwen",
		"adapters":[{"name":"acme-style","path":"acme.gguf"}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanLocalModelRoot("local-llama", "llama", root, transport.DaemonStatus{})
	if err != nil {
		t.Fatalf("scanLocalModelRoot: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %+v, want base + variant", got)
	}
	var variant *localModelInventoryEntry
	for i := range got {
		if got[i].Model == "qwen-acme" {
			variant = &got[i]
		}
	}
	if variant == nil {
		t.Fatalf("variant entry missing: %+v", got)
	}
	// The variant reuses the base model.gguf (no duplicated weights) and shows
	// its base + adapter in the detail annotation.
	if variant.Path != baseModelPath {
		t.Fatalf("variant path = %q, want reused base gguf %q", variant.Path, baseModelPath)
	}
	if !strings.Contains(variant.Detail, "qwen") || !strings.Contains(variant.Detail, "acme-style") {
		t.Fatalf("variant detail = %q, want base + adapter", variant.Detail)
	}
}

func TestUnit_LocalModelInventoryScansOpenVINOVariant(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "qwen3-8b-ov")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	variantDir := filepath.Join(root, "qwen3-8b-ov-acme")
	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "acme.safetensors"), []byte("adapter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "contenox-variant.json"), []byte(`{
		"backend":"openvino",
		"base_model":"qwen3-8b-ov",
		"adapters":[{"name":"acme-style","path":"acme.safetensors"}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stray foreign-backend marker in the same root must be skipped, not
	// annotated as an openvino variant.
	foreignDir := filepath.Join(root, "qwen3-8b-ov-foreign")
	if err := os.MkdirAll(foreignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignDir, "contenox-variant.json"), []byte(`{
		"backend":"llama",
		"base_model":"qwen3-8b-ov",
		"adapters":[{"name":"x","path":"x.gguf"}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanLocalModelRoot("local-ov", "openvino", root, transport.DaemonStatus{})
	if err != nil {
		t.Fatalf("scanLocalModelRoot: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %+v, want base + variant (foreign marker skipped)", got)
	}
	var variant *localModelInventoryEntry
	for i := range got {
		if got[i].Model == "qwen3-8b-ov-acme" {
			variant = &got[i]
		}
		if got[i].Model == "qwen3-8b-ov-foreign" {
			t.Fatalf("foreign-backend marker must not be surfaced as an openvino variant: %+v", got[i])
		}
	}
	if variant == nil {
		t.Fatalf("variant entry missing: %+v", got)
	}
	// The variant reuses the base IR directory (no duplicated IR) and shows its
	// base + adapter in the detail annotation.
	if variant.Path != baseDir {
		t.Fatalf("variant path = %q, want reused base IR dir %q", variant.Path, baseDir)
	}
	if !strings.Contains(variant.Detail, "qwen3-8b-ov") || !strings.Contains(variant.Detail, "acme-style") {
		t.Fatalf("variant detail = %q, want base + adapter", variant.Detail)
	}
}

func TestUnit_LocalModelInventoryScansOpenVINOLanguageModelEntrypoint(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "gemma-ov")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanLocalModelRoot("local-ov", "openvino", root, transport.DaemonStatus{})
	if err != nil {
		t.Fatalf("scanLocalModelRoot: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %+v, want one", got)
	}
	if got[0].Model != "gemma-ov" || got[0].Path != modelDir || got[0].Status != "installed" {
		t.Fatalf("entry = %+v", got[0])
	}
}

func TestUnit_LocalModelInventoryMarksActiveSlot(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "qwen-ov")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	status := transport.DaemonStatus{
		State: transport.SlotReady,
		Active: &transport.ActiveModel{
			Type: "openvino",
			Path: modelDir,
		},
	}
	got, err := scanLocalModelRoot("local-ov", "openvino", root, status)
	if err != nil {
		t.Fatalf("scanLocalModelRoot: %v", err)
	}
	if len(got) != 1 || got[0].Status != "active:Ready" {
		t.Fatalf("entries = %+v, want active ready", got)
	}
}
