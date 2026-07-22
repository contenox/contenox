package openapidocs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/internal/openapigen"
)

// TestUnit_OpenAPISpec_Fresh regenerates the OpenAPI spec from the working
// tree and byte-compares it against the embedded openapi.json, so a route or
// annotation change that is not accompanied by a regenerated spec fails the
// unit suite instead of shipping a stale document.
func TestUnit_OpenAPISpec_Fresh(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	fresh, _, err := openapigen.Generate(root)
	if err != nil {
		t.Fatalf("openapigen.Generate: %v", err)
	}
	if !bytes.Equal(fresh, specJSON) {
		t.Fatal("openapi.json is stale — run: make openapi")
	}
}

// repoRoot walks up from the test's working directory (this package's
// directory under `go test`) to the directory containing go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
