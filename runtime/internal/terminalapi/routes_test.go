package terminalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/terminalservice"
	"github.com/stretchr/testify/require"
)

type fakeTerminalService struct {
	mu       sync.Mutex
	sessions map[string]*terminalservice.SessionInfo
	nextID   string
}

func newFakeTerminalService() *fakeTerminalService {
	return &fakeTerminalService{sessions: map[string]*terminalservice.SessionInfo{}, nextID: "term-1"}
}

func (f *fakeTerminalService) Create(_ context.Context, principal string, req terminalservice.CreateRequest) (*terminalservice.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.sessions[id] = &terminalservice.SessionInfo{
		ID:             id,
		Principal:      principal,
		CWD:            req.CWD,
		Shell:          req.Shell,
		Cols:           req.Cols,
		Rows:           req.Rows,
		Status:         "active",
		NodeInstanceID: "node-1",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	return &terminalservice.CreateResponse{ID: id}, nil
}

func (f *fakeTerminalService) Close(_ context.Context, principal, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || s.Principal != principal {
		return terminalservice.ErrSessionNotFound
	}
	delete(f.sessions, id)
	return nil
}

func (f *fakeTerminalService) CloseAll(context.Context) error { return nil }

func (f *fakeTerminalService) Attach(context.Context, string, string, io.ReadWriteCloser, <-chan terminalservice.ResizeMsg) error {
	return errors.New("not used")
}

func (f *fakeTerminalService) Get(_ context.Context, principal, id string) (*terminalservice.SessionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || s.Principal != principal {
		return nil, terminalservice.ErrSessionNotFound
	}
	return s, nil
}

func (f *fakeTerminalService) List(_ context.Context, principal string, _ *time.Time, _ int) ([]*terminalservice.SessionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*terminalservice.SessionInfo{}
	for _, s := range f.sessions {
		if s.Principal == principal {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeTerminalService) UpdateGeometry(_ context.Context, principal, id string, cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || s.Principal != principal {
		return terminalservice.ErrSessionNotFound
	}
	s.Cols = cols
	s.Rows = rows
	return nil
}

func (f *fakeTerminalService) ReapIdle(context.Context) error { return nil }

func TestAddRoutes_DisabledDoesNotRegister(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, newFakeTerminalService(), nil, false, "")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil)
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestTerminalRoutes_CreateListGetDelete pins the surviving substrate for the
// future ACP terminal capability-provider seam: create/list/get/delete plus
// the WS. PATCH /terminal/sessions/{id} was removed in Stage 6 of the beam
// ACP unification (zero consumers; resize now rides the WS control frame —
// see termConn.Read's "resize" message handling) and must 405, not succeed.
func TestTerminalRoutes_CreateListGetDelete(t *testing.T) {
	mux := http.NewServeMux()
	svc := newFakeTerminalService()
	AddRoutes(mux, svc, nil, true, "")

	rr := httptest.NewRecorder()
	req := jsonReq(http.MethodPost, "/terminal/sessions", map[string]any{
		"cwd":   "/tmp",
		"cols":  80,
		"rows":  24,
		"shell": "/bin/bash",
	})
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var created createSessionResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))
	require.Equal(t, "term-1", created.ID)
	require.Equal(t, "/api/terminal/sessions/term-1/ws", created.WSPath)

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "term-1")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/terminal/sessions/term-1", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, jsonReq(http.MethodPatch, "/terminal/sessions/term-1", map[string]any{"cols": 100, "rows": 40}))
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "PATCH is retired; the mux must not route it")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/terminal/sessions/term-1", nil))
	require.Equal(t, http.StatusNoContent, rr.Code)

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/terminal/sessions/term-1", nil))
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestTerminalRoutes_InvalidInputs pins that malformed pagination parameters
// are refused with 400. This route was the only one that ever rejected
// limit < 1, and it did so with 422; parsing now goes through
// apiframework.ListParams, which classifies every malformed pagination
// parameter as ErrInvalidParameterValue and so answers 400 everywhere.
func TestTerminalRoutes_InvalidInputs(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, newFakeTerminalService(), nil, true, "")

	for _, query := range []string{"limit=0", "limit=-1", "limit=not-a-number", "cursor=garbage"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/terminal/sessions?"+query, nil))
		require.Equal(t, http.StatusBadRequest, rr.Code, "query %q", query)
	}
}

type cappedTerminalService struct {
	fakeTerminalService
}

func (c *cappedTerminalService) Create(context.Context, string, terminalservice.CreateRequest) (*terminalservice.CreateResponse, error) {
	return nil, terminalservice.ErrTooManySessions
}

func TestTerminalRoutes_TooManySessionsReturns422(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, &cappedTerminalService{fakeTerminalService: *newFakeTerminalService()}, nil, true, "")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, jsonReq(http.MethodPost, "/terminal/sessions", map[string]any{"cwd": "/tmp"}))
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

func TestTerminalRoutes_TokenProtectsReadRoutes(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, newFakeTerminalService(), nil, true, "secret")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	mux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func jsonReq(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}
