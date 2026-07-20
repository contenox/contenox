package serverapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// missionGatingMux builds the product mux the way serve does, with or without the
// mission service dependency.
func missionGatingMux(t *testing.T, withMissions bool) *http.ServeMux {
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
	if withMissions {
		deps.Missions = missionservice.New(db)
	}

	mux := http.NewServeMux()
	if _, err := New(ctx, mux, "test-node", "local", &Config{}, deps); err != nil {
		t.Fatalf("serverapi.New: %v", err)
	}
	return mux
}

// TestServe_MissionRoutesGatedOnMissions proves /missions is registered only when
// serve passes its mission service, mirroring the other nil-gated route groups.
func TestServe_MissionRoutesGatedOnMissions(t *testing.T) {
	withMissions := missionGatingMux(t, true)
	rr := httptest.NewRecorder()
	withMissions.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/missions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("with missions: /missions = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	withoutMissions := missionGatingMux(t, false)
	rr = httptest.NewRecorder()
	withoutMissions.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/missions", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("without missions: /missions = %d, want 404", rr.Code)
	}
}
