package openvino

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
)

func TestUnit_OpenVINOCatalogListsLanguageModelEntrypoint(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "gemma-ov")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldFactory := sessionFactory
	sessionFactory = func(modeldconn.ModelRef, Config) (Session, error) { return nil, nil }
	t.Cleanup(func() { sessionFactory = oldFactory })

	got, err := (&catalogProvider{dir: root}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("models = %+v, want one", got)
	}
	if got[0].Name != "gemma-ov" {
		t.Fatalf("model name = %q, want gemma-ov", got[0].Name)
	}
}
