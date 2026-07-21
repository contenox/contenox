package serverapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/operatorinbox"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// operatorInboxGatingMux builds the product mux the way serve does, with or
// without the operator-inbox dependency.
func operatorInboxGatingMux(t *testing.T, withInbox bool) *http.ServeMux {
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
	if withInbox {
		deps.OperatorInbox = operatorinbox.New(db)
	}

	mux := http.NewServeMux()
	if _, err := New(ctx, mux, "test-node", "local", &Config{}, deps); err != nil {
		t.Fatalf("serverapi.New: %v", err)
	}
	return mux
}

// TestServe_OperatorInboxRoutesGatedOnInbox proves /operator-inbox is registered
// only when serve passes the operator-inbox service, mirroring the other
// nil-gated route groups.
func TestServe_OperatorInboxRoutesGatedOnInbox(t *testing.T) {
	withInbox := operatorInboxGatingMux(t, true)
	rr := httptest.NewRecorder()
	withInbox.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/operator-inbox", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("with inbox: /operator-inbox = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	withoutInbox := operatorInboxGatingMux(t, false)
	rr = httptest.NewRecorder()
	withoutInbox.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/operator-inbox", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("without inbox: /operator-inbox = %d, want 404", rr.Code)
	}
}
