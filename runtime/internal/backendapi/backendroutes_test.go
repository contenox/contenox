package backendapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/missionapi"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

func TestCreateBackendDuplicateReturnsSanitizedConflict(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "backendapi.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	backendapi.AddBackendRoutes(mux, backendservice.New(db), &stubStateService{})

	body := map[string]string{
		"name":    "first",
		"type":    "ollama",
		"baseUrl": "http://127.0.0.1:11434",
	}
	postJSON(t, mux, "/backends", body, http.StatusCreated)

	body["name"] = "second"
	rr := postJSON(t, mux, "/backends", body, http.StatusConflict)

	var got struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Type != "invalid_request_error" || got.Error.Code != "conflict" {
		t.Fatalf("error type/code = %q/%q, want invalid_request_error/conflict", got.Error.Type, got.Error.Code)
	}
	if !strings.Contains(got.Error.Message, `backend already exists for type "ollama" and base URL "http://127.0.0.1:11434"`) {
		t.Fatalf("unexpected message: %q", got.Error.Message)
	}
	for _, leaked := range []string{"libdb:", "UNIQUE constraint", "llm_backends", "2067"} {
		if strings.Contains(got.Error.Message, leaked) {
			t.Fatalf("response leaked %q: %q", leaked, got.Error.Message)
		}
	}
}

// TestListPaginationErrorsAgreeAcrossRoutes mounts two list routes that used
// to disagree onto one mux and asserts they now answer identical malformed
// pagination input identically.
//
// Before apiframework.ListParams these two hand-rolled the same parse with
// different error treatments: GET /backends wrapped apiframework
// .ErrUnprocessableEntity and answered 422, while GET /missions wrapped a
// bare fmt.Errorf, which apiframework.mapErrorToStatus could not classify and
// so resolved through its ListOperation fallback to 404 — "no such
// collection" for what is really a malformed query parameter. Same input,
// two answers, decided only by which handler you hit.
//
// It lives in this package because an external test package can import both
// route packages; the assertion is deliberately "the two agree, and agree on
// 400", not just "each returns 400".
func TestListPaginationErrorsAgreeAcrossRoutes(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "listparams.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	backendapi.AddBackendRoutes(mux, backendservice.New(db), &stubStateService{})
	missionapi.AddRoutes(mux, missionservice.New(db))

	for _, query := range []string{"cursor=garbage", "limit=not-a-number", "limit=0", "limit=-1"} {
		statuses := map[string]int{}
		for _, path := range []string{"/backends", "/missions"} {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
			statuses[path] = rr.Code
		}
		if statuses["/backends"] != statuses["/missions"] {
			t.Fatalf("?%s: GET /backends = %d but GET /missions = %d; the two must agree",
				query, statuses["/backends"], statuses["/missions"])
		}
		if statuses["/backends"] != http.StatusBadRequest {
			t.Fatalf("?%s: status = %d, want %d — a malformed parameter is not a missing resource",
				query, statuses["/backends"], http.StatusBadRequest)
		}
	}

	// A well-formed request on both routes still succeeds, so the assertion
	// above is not passing because the routes are uniformly broken.
	for _, path := range []string{"/backends", "/missions"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path+"?limit=5&cursor=2024-03-01T12:00:00Z", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s with valid pagination = %d, want %d: %s", path, rr.Code, http.StatusOK, rr.Body.String())
		}
	}
}

func postJSON(t *testing.T, mux *http.ServeMux, path string, body any, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d: %s", path, rr.Code, wantStatus, rr.Body.String())
	}
	return rr
}
