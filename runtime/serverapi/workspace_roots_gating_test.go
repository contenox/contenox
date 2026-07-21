package serverapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
)

// workspaceRootsGatingMux builds the product mux the way serve does, with or
// without a WorkspaceRoots allowlist configured.
func workspaceRootsGatingMux(t *testing.T, withFactory bool) *http.ServeMux {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "serverapi.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state, err := runtimestate.New(ctx, db, libbus.NewInMem())
	if err != nil {
		t.Fatalf("state: %v", err)
	}

	deps := Dependencies{DB: db, State: state}
	if withFactory {
		factory, err := vfs.NewFactory(t.TempDir())
		if err != nil {
			t.Fatalf("vfs.NewFactory: %v", err)
		}
		deps.WorkspaceRoots = factory
	}

	mux := http.NewServeMux()
	if _, err := New(ctx, mux, "test-node", "local", &Config{}, deps); err != nil {
		t.Fatalf("serverapi.New: %v", err)
	}
	return mux
}

// TestServe_WorkspaceRootsRouteGatedOnFactory proves GET /workspace/roots is
// registered only when serve has a workspace-root allowlist configured,
// mirroring the other nil-gated route groups (see fleet_gating_test.go). With
// no allowlist there is nothing to report — and, more importantly, no
// allowlist to accidentally leak the shape of.
func TestServe_WorkspaceRootsRouteGatedOnFactory(t *testing.T) {
	withFactory := workspaceRootsGatingMux(t, true)
	rr := httptest.NewRecorder()
	withFactory.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/workspace/roots", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("with factory: /workspace/roots = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	withoutFactory := workspaceRootsGatingMux(t, false)
	rr = httptest.NewRecorder()
	withoutFactory.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/workspace/roots", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("without factory: /workspace/roots = %d, want 404", rr.Code)
	}
}
