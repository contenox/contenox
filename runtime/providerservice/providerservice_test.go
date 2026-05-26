package providerservice

import (
	"context"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/runtimestate"
	"github.com/contenox/agent/runtime/runtimetypes"
)

func TestSetProviderConfig_OllamaCreatesHostedBackend(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "test.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := New(db, nil)
	if err := svc.SetProviderConfig(ctx, ProviderTypeOllama, false, &runtimestate.ProviderConfig{
		APIKey: "ollama-test-key",
	}); err != nil {
		t.Fatal(err)
	}

	store := runtimetypes.New(db.WithoutTransaction())
	backend, err := store.GetBackend(ctx, ProviderTypeOllama)
	if err != nil {
		t.Fatal(err)
	}
	if backend.Type != ProviderTypeOllama {
		t.Fatalf("backend.Type = %q, want %q", backend.Type, ProviderTypeOllama)
	}
	if backend.BaseURL != "https://ollama.com/api" {
		t.Fatalf("backend.BaseURL = %q, want https://ollama.com/api", backend.BaseURL)
	}
}
