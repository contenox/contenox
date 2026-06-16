package llama

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/modeld"
)

func withSessionFactory(t *testing.T, f SessionFactory) {
	t.Helper()
	old := sessionFactory
	sessionFactory = f
	t.Cleanup(func() {
		sessionFactory = old
		closeCachedSessionsForTest()
	})
}

func TestUnit_LocalNodeCatalog_HiddenWhenBackendNotCompiled(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	models, err := (&catalogProvider{dir: dir}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("models = %d, want 0 when session backend is unavailable", len(models))
	}
}

func TestUnit_LocalNodeCatalog_ListsGGUFWithProfile(t *testing.T) {
	withSessionFactory(t, func(string, Config) (Session, error) { return nil, nil })

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := []byte(`{
		"max_output_tokens": 512,
		"can_think": true,
		"runtime": {"num_ctx": 16384, "num_batch": 1024, "flash_attention": true}
	}`)
	if err := os.WriteFile(filepath.Join(modelDir, profileFileName), profile, 0o644); err != nil {
		t.Fatal(err)
	}

	models, err := (&catalogProvider{dir: dir}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	got := models[0]
	if got.Name != "coder" {
		t.Fatalf("Name = %q, want coder", got.Name)
	}
	if got.ContextLength != 16384 {
		t.Fatalf("ContextLength = %d, want runtime num_ctx", got.ContextLength)
	}
	if got.MaxOutputTokens != 512 {
		t.Fatalf("MaxOutputTokens = %d, want 512", got.MaxOutputTokens)
	}
	if !got.CanChat || !got.CanPrompt || !got.CanStream {
		t.Fatalf("expected chat/prompt/stream capabilities when session backend is available: %+v", got.CapabilityConfig)
	}
	if got.CanEmbed {
		t.Fatal("llama should not advertise embeddings yet")
	}
	if !got.CanThink {
		t.Fatal("expected can_think from profile")
	}
	if got.Meta["node"] != "llama" || got.Meta["runtime"] != "llamacpp" {
		t.Fatalf("unexpected meta: %+v", got.Meta)
	}
}

func TestUnit_LlamaCatalog_LocalTypeAliasesToLlama(t *testing.T) {
	withSessionFactory(t, func(string, Config) (Session, error) { return nil, nil })

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := modeld.NewCatalogProvider(modeld.BackendSpec{Type: "local", BaseURL: dir})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Type() != "llama" {
		t.Fatalf("catalog type = %q, want llama", catalog.Type())
	}
	models, err := catalog.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "coder" {
		t.Fatalf("models = %+v, want coder from llama catalog", models)
	}
}
