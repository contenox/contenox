package backendservice

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
)

func TestUnit_BackendService_ValidateRejectsUnknownTypes(t *testing.T) {
	invalidBackend := &runtimetypes.Backend{
		Name:    "my-backend",
		BaseURL: "http://localhost:8000",
		Type:    "unsupported-type",
	}

	err := validate(invalidBackend)
	if err == nil || !strings.Contains(err.Error(), "Type must be ollama") {
		t.Fatalf("expected validation error for unknown type, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateAcceptsLlama(t *testing.T) {
	err := validate(&runtimetypes.Backend{
		Name:    "llama",
		BaseURL: "/tmp/models",
		Type:    "llama",
	})
	if err != nil {
		t.Fatalf("expected llama backend type to be accepted, got: %v", err)
	}
}

// TestUnit_BackendService_ValidateAcceptsModeld locks in that "modeld" (local
// sentinel or remote host:port) is an accepted backend type. It regressed
// silently — the CLI's `backend add --type modeld` help text and examples
// documented this as first-class, but this switch was never updated when the
// modeld backend type was introduced, so every real "backend add --type
// modeld" invocation failed validation until this fix.
func TestUnit_BackendService_ValidateAcceptsModeld(t *testing.T) {
	for _, baseURL := range []string{"local", "192.168.1.50:9090"} {
		err := validate(&runtimetypes.Backend{
			Name:    "gpu-box",
			BaseURL: baseURL,
			Type:    "modeld",
		})
		if err != nil {
			t.Fatalf("expected modeld backend type (BaseURL=%q) to be accepted, got: %v", baseURL, err)
		}
	}
}

func TestUnit_BackendService_ValidateAcceptsLocalAsLlamaAlias(t *testing.T) {
	err := validate(&runtimetypes.Backend{
		Name:    "local",
		BaseURL: "/tmp/models",
		Type:    "local",
	})
	if err != nil {
		t.Fatalf("expected local backend type alias to be accepted, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateRejectsRetiredLocalNodeType(t *testing.T) {
	for _, typ := range []string{"localnode"} {
		err := validate(&runtimetypes.Backend{
			Name:    typ,
			BaseURL: "/tmp/models",
			Type:    typ,
		})
		if err == nil {
			t.Fatalf("expected retired backend type %q to be rejected", typ)
		}
	}
}

func TestUnit_BackendService_ValidateRequiresNameAndURL(t *testing.T) {
	err := validate(&runtimetypes.Backend{BaseURL: "http://host", Type: "ollama"})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name validation error, got: %v", err)
	}

	err = validate(&runtimetypes.Backend{Name: "my-backend", Type: "ollama"})
	if err == nil || !strings.Contains(err.Error(), "baseURL is required") {
		t.Fatalf("expected baseURL validation error, got: %v", err)
	}
}

func TestUnit_BackendService_DuplicateTypeAndURLReturnsDomainConflict(t *testing.T) {
	ctx, db := setupBackendServiceDB(t)
	svc := New(db)

	first := &runtimetypes.Backend{
		ID:      uuid.NewString(),
		Name:    "first",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11434",
	}
	duplicate := &runtimetypes.Backend{
		ID:      uuid.NewString(),
		Name:    "second",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11434",
	}

	if err := svc.Create(ctx, first); err != nil {
		t.Fatalf("create first backend: %v", err)
	}
	err := svc.Create(ctx, duplicate)
	if !errors.Is(err, libdb.ErrUniqueViolation) {
		t.Fatalf("duplicate error = %v, want ErrUniqueViolation", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `backend already exists for type "ollama" and base URL "http://127.0.0.1:11434"`) {
		t.Fatalf("unexpected duplicate message: %q", msg)
	}
	for _, leaked := range []string{"libdb:", "UNIQUE constraint", "llm_backends", "2067"} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("duplicate message leaked %q: %q", leaked, msg)
		}
	}
}

func TestUnit_BackendService_UpdateDuplicateTypeAndURLReturnsDomainConflict(t *testing.T) {
	ctx, db := setupBackendServiceDB(t)
	svc := New(db)

	first := &runtimetypes.Backend{ID: uuid.NewString(), Name: "first", Type: "ollama", BaseURL: "http://127.0.0.1:11434"}
	second := &runtimetypes.Backend{ID: uuid.NewString(), Name: "second", Type: "ollama", BaseURL: "http://127.0.0.1:11435"}
	if err := svc.Create(ctx, first); err != nil {
		t.Fatalf("create first backend: %v", err)
	}
	if err := svc.Create(ctx, second); err != nil {
		t.Fatalf("create second backend: %v", err)
	}

	second.BaseURL = first.BaseURL
	err := svc.Update(ctx, second)
	if !errors.Is(err, libdb.ErrUniqueViolation) {
		t.Fatalf("update duplicate error = %v, want ErrUniqueViolation", err)
	}
	if strings.Contains(err.Error(), "llm_backends") || strings.Contains(err.Error(), "UNIQUE constraint") {
		t.Fatalf("update duplicate message leaked storage detail: %q", err.Error())
	}
}

func setupBackendServiceDB(t *testing.T) (context.Context, libdb.DBManager) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "backendservice.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}
