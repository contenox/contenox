package localfileapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
	"github.com/stretchr/testify/require"
)

// stubGrants is an in-test stand-in for serve's reloader: Add/Remove mutate a
// live Factory via SetRoots, exactly as the real mutators do, so the POST/DELETE
// handlers' response (re-read from that Factory) reflects the change. It lets the
// handler wiring be tested without the config DB or the bus.
type stubGrants struct {
	factory   *vfs.Factory
	roots     []string
	rejectAdd bool
}

func (s *stubGrants) mutators() *localfileapi.RootsMutators {
	return &localfileapi.RootsMutators{
		Add: func(_ context.Context, path string) error {
			if s.rejectAdd {
				return fmt.Errorf("%w: %q is a file, not a directory", workspacegrants.ErrInvalidGrant, path)
			}
			s.roots = append(s.roots, path)
			return s.factory.SetRoots(s.roots)
		},
		Remove: func(_ context.Context, path string) error {
			kept := make([]string, 0, len(s.roots))
			for _, r := range s.roots {
				if r != path {
					kept = append(kept, r)
				}
			}
			s.roots = kept
			return s.factory.SetRoots(s.roots)
		},
	}
}

func decodeRoots(t *testing.T, body []byte) []string {
	t.Helper()
	var resp struct {
		Roots []struct {
			Path string `json:"path"`
		} `json:"roots"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	out := make([]string, 0, len(resp.Roots))
	for _, r := range resp.Roots {
		out = append(out, r.Path)
	}
	return out
}

func TestUnit_WorkspaceRoots_PostGrantsAndDeleteRevokes(t *testing.T) {
	base := t.TempDir()
	grant := t.TempDir()
	factory, err := vfs.NewFactory(base)
	require.NoError(t, err)
	resolvedBase, _ := vfs.ResolveRoot(base)
	resolvedGrant, _ := vfs.ResolveRoot(grant)

	stub := &stubGrants{factory: factory, roots: []string{base}}
	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, stub.mutators())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Baseline: only the base root.
	resp, err := http.Get(srv.URL + "/workspace/roots")
	require.NoError(t, err)
	baseline := readBody(t, resp)
	require.Equal(t, []string{resolvedBase}, decodeRoots(t, baseline))

	// POST grants a new root; the response already reflects it.
	body, _ := json.Marshal(map[string]string{"path": grant})
	resp, err = http.Post(srv.URL+"/workspace/roots", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.ElementsMatch(t, []string{resolvedBase, resolvedGrant}, decodeRoots(t, readBody(t, resp)))

	// DELETE revokes it.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/workspace/roots?path="+resolvedGrant, nil)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{resolvedBase}, decodeRoots(t, readBody(t, resp)))
}

func TestUnit_WorkspaceRoots_PostValidation(t *testing.T) {
	base := t.TempDir()
	factory, err := vfs.NewFactory(base)
	require.NoError(t, err)

	t.Run("missing path is a 4xx", func(t *testing.T) {
		stub := &stubGrants{factory: factory, roots: []string{base}}
		mux := http.NewServeMux()
		localfileapi.AddWorkspaceRootsRoutes(mux, factory, stub.mutators())
		srv := httptest.NewServer(mux)
		defer srv.Close()

		body, _ := json.Marshal(map[string]string{"path": "   "})
		resp, err := http.Post(srv.URL+"/workspace/roots", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		require.GreaterOrEqual(t, resp.StatusCode, 400)
		require.Less(t, resp.StatusCode, 500, "a missing path is the client's fault")
	})

	t.Run("an invalid grant maps to 422", func(t *testing.T) {
		stub := &stubGrants{factory: factory, roots: []string{base}, rejectAdd: true}
		mux := http.NewServeMux()
		localfileapi.AddWorkspaceRootsRoutes(mux, factory, stub.mutators())
		srv := httptest.NewServer(mux)
		defer srv.Close()

		body, _ := json.Marshal(map[string]string{"path": "/etc/hosts"})
		resp, err := http.Post(srv.URL+"/workspace/roots", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
			"a grant wrapping ErrInvalidGrant is a 422, not a 500")
	})

	t.Run("DELETE without a path is a 4xx", func(t *testing.T) {
		stub := &stubGrants{factory: factory, roots: []string{base}}
		mux := http.NewServeMux()
		localfileapi.AddWorkspaceRootsRoutes(mux, factory, stub.mutators())
		srv := httptest.NewServer(mux)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/workspace/roots", nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.GreaterOrEqual(t, resp.StatusCode, 400)
		require.Less(t, resp.StatusCode, 500)
	})
}

// TestUnit_WorkspaceRoots_NilMutatorsIsReadOnly proves that without mutators the
// write verbs are not registered — POST/DELETE 405 — while GET still serves.
func TestUnit_WorkspaceRoots_NilMutatorsIsReadOnly(t *testing.T) {
	base := t.TempDir()
	factory, err := vfs.NewFactory(base)
	require.NoError(t, err)
	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/workspace/roots", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "no mutators means no POST route")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/workspace/roots", nil))
	require.Equal(t, http.StatusOK, rr.Code, "GET still serves read-only")
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	return buf.Bytes()
}
