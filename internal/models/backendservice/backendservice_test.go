package backendservice

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/google/uuid"
)

func TestUnit_BackendService_ValidateRejectsUnknownTypes(t *testing.T) {
	invalidBackend := &runtimetypes.Backend{
		Name:    "my-backend",
		BaseURL: "http://localhost:8000",
		Type:    "unsupported-type",
	}

	err := validate(invalidBackend)
	if err == nil || !strings.Contains(err.Error(), "Type must be") {
		t.Fatalf("expected validation error for unknown type, got: %v", err)
	}
}

func TestUnit_BackendService_ValidateRejectsRetiredLocalNodeType(t *testing.T) {
	for _, typ := range []string{"localnode", "llama", "openvino"} {
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

func TestUnit_BackendService_ValidateAcceptsModeldAndLocal(t *testing.T) {
	for _, typ := range []string{"modeld", "local"} {
		err := validate(&runtimetypes.Backend{
			Name:    typ,
			BaseURL: "local",
			Type:    typ,
		})
		if err != nil {
			t.Fatalf("expected backend type %q to be accepted, got error: %v", typ, err)
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

func TestUnit_BackendService_DuplicateNameReturnsDomainConflict(t *testing.T) {
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
		Name:    "first",
		Type:    "ollama",
		BaseURL: "http://127.0.0.1:11435",
	}

	if err := svc.Create(ctx, first); err != nil {
		t.Fatalf("create first backend: %v", err)
	}
	err := svc.Create(ctx, duplicate)
	if !errors.Is(err, libdb.ErrUniqueViolation) {
		t.Fatalf("duplicate error = %v, want ErrUniqueViolation", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `backend "first" already exists`) {
		t.Fatalf("unexpected duplicate message: %q", msg)
	}
	for _, leaked := range []string{"libdb:", "UNIQUE constraint", "llm_backends", "2067"} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("duplicate message leaked %q: %q", leaked, msg)
		}
	}
}

func TestUnit_BackendService_SameTypeAndURLIsAllowed(t *testing.T) {
	ctx, db := setupBackendServiceDB(t)
	svc := New(db)

	first := &runtimetypes.Backend{ID: uuid.NewString(), Name: "acct-a", Type: "openai", BaseURL: "https://api.openai.com/v1"}
	second := &runtimetypes.Backend{ID: uuid.NewString(), Name: "acct-b", Type: "openai", BaseURL: "https://api.openai.com/v1"}

	if err := svc.Create(ctx, first); err != nil {
		t.Fatalf("create first backend: %v", err)
	}
	if err := svc.Create(ctx, second); err != nil {
		t.Fatalf("a second entry over one upstream must be allowed: %v", err)
	}
}

func TestUnit_BackendService_UpdateDuplicateNameReturnsDomainConflict(t *testing.T) {
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
	if err := svc.Update(ctx, second); err != nil {
		t.Fatalf("repointing an entry at a served upstream must be allowed: %v", err)
	}

	second.Name = first.Name
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
