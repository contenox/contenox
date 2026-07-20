package serverapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// fleetGatingMux builds the product mux the way serve does, with or without the
// instance Manager dependency.
func fleetGatingMux(t *testing.T, withInstances bool) *http.ServeMux {
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
	if withInstances {
		instances := agentinstance.New(agentregistryservice.New(db))
		t.Cleanup(func() { _ = instances.Close() })
		deps.Instances = instances
	}

	mux := http.NewServeMux()
	if _, err := New(ctx, mux, "test-node", "local", &Config{}, deps); err != nil {
		t.Fatalf("serverapi.New: %v", err)
	}
	return mux
}

// TestServe_FleetRoutesGatedOnInstances proves /fleet is registered only when
// serve passes its instance Manager, mirroring the other nil-gated route groups.
func TestServe_FleetRoutesGatedOnInstances(t *testing.T) {
	withFleet := fleetGatingMux(t, true)
	rr := httptest.NewRecorder()
	withFleet.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/fleet", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("with instances: /fleet = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	withoutFleet := fleetGatingMux(t, false)
	rr = httptest.NewRecorder()
	withoutFleet.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/fleet", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("without instances: /fleet = %d, want 404", rr.Code)
	}
}
