package localfileapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

// workspaceRootsResponse mirrors the unexported wire shape localfileapi
// encodes, so the test can decode without reaching into the package.
type workspaceRootsResponse struct {
	Roots []struct {
		Path    string `json:"path"`
		Default bool   `json:"default"`
	} `json:"roots"`
}

func getWorkspaceRoots(t *testing.T, mux *http.ServeMux) (*http.Response, workspaceRootsResponse) {
	t.Helper()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/workspace/roots")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var body workspaceRootsResponse
	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	}
	return resp, body
}

// TestUnit_WorkspaceRootsRoutes_SingleRoot proves the single-root case reports
// exactly one entry, marked default.
func TestUnit_WorkspaceRootsRoutes_SingleRoot(t *testing.T) {
	root := t.TempDir()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	resp, body := getWorkspaceRoots(t, mux)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, body.Roots, 1)

	resolvedRoot, err := vfs.ResolveRoot(root)
	require.NoError(t, err)
	require.Equal(t, resolvedRoot, body.Roots[0].Path)
	require.True(t, body.Roots[0].Default, "the sole configured root must be marked default")
}

// TestUnit_WorkspaceRootsRoutes_MultiRoot proves a multi-root allowlist
// reports every root, with exactly the first (configured default) marked.
func TestUnit_WorkspaceRootsRoutes_MultiRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	rootC := t.TempDir()
	factory, err := vfs.NewFactory(rootA, rootB, rootC)
	require.NoError(t, err)

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	resp, body := getWorkspaceRoots(t, mux)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, body.Roots, 3)

	resolvedA, err := vfs.ResolveRoot(rootA)
	require.NoError(t, err)

	defaults := 0
	paths := map[string]bool{}
	for _, r := range body.Roots {
		paths[r.Path] = true
		if r.Default {
			defaults++
			require.Equal(t, resolvedA, r.Path, "the default flag must mark the first configured root")
		}
	}
	require.Equal(t, 1, defaults, "exactly one root must be marked default")

	resolvedB, err := vfs.ResolveRoot(rootB)
	require.NoError(t, err)
	resolvedC, err := vfs.ResolveRoot(rootC)
	require.NoError(t, err)
	require.True(t, paths[resolvedB])
	require.True(t, paths[resolvedC])
}

// TestUnit_WorkspaceRootsRoutes_ExcludesControlPlanePath proves a directory
// never handed to vfs.NewFactory (standing in for the control-plane
// ContenoxDir, which serve wires separately and never passes into
// buildWorkspaceFactory) does not appear in the response — the allowlist is
// exactly what was configured, nothing implied or scanned in.
func TestUnit_WorkspaceRootsRoutes_ExcludesControlPlanePath(t *testing.T) {
	root := t.TempDir()
	controlPlaneDir := t.TempDir() // never passed to NewFactory
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	resp, body := getWorkspaceRoots(t, mux)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resolvedControlPlane, err := vfs.ResolveRoot(controlPlaneDir)
	require.NoError(t, err)
	for _, r := range body.Roots {
		require.NotEqual(t, resolvedControlPlane, r.Path, "a path never added to the allowlist must never appear as a root")
	}
}

// TestUnit_WorkspaceRootsRoutes_NilFactoryRegistersNothing proves the nil
// factory case (no workspace-root allowlist configured) registers no route at
// all, so the mux's own not-found handling applies — mirroring
// AddWorkspaceRoutes's own non-nil requirement, and the other nil-gated route
// groups in serverapi. This stands in for the "empty roots" configuration:
// vfs.NewFactory itself refuses to build a Factory with zero roots (see
// vfs.TestUnit_Factory_EmptyIsError), so nil-factory is the only way an empty
// allowlist can reach this handler.
func TestUnit_WorkspaceRootsRoutes_NilFactoryRegistersNothing(t *testing.T) {
	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, nil, nil)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/workspace/roots", nil))
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestUnit_WorkspaceRootsRoutes_PathsAreAbsolute is a light shape check: every
// reported root must be an absolute, cleaned filesystem path (what a folder
// picker can use as-is), not the operator-typed string verbatim.
func TestUnit_WorkspaceRootsRoutes_PathsAreAbsolute(t *testing.T) {
	root := t.TempDir()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	_, body := getWorkspaceRoots(t, mux)
	require.Len(t, body.Roots, 1)
	require.True(t, filepath.IsAbs(body.Roots[0].Path))
	require.Equal(t, filepath.Clean(body.Roots[0].Path), body.Roots[0].Path)
}

// TestUnit_WorkspaceRootsRoutes_MethodAndPath proves the route is registered
// under exactly GET /workspace/roots (not, say, matched by an unrelated verb).
func TestUnit_WorkspaceRootsRoutes_MethodAndPath(t *testing.T) {
	root := t.TempDir()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, nil)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/workspace/roots", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}
