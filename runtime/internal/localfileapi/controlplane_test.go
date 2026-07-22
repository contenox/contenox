package localfileapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnit_WorkspaceRoutes_ControlPlaneShownButUnenterable is the composed proof
// of the control-plane carveout on the /files explorer surface — the live hole
// containment opened today (a granted root whose CHILD is ~/.contenox).
//
// Show-vs-hide decision (documented, and enforced here): the control-plane entry
// is SHOWN in its parent's listing but is UNENTERABLE. Hiding it would lie about
// the filesystem — the directory is really there, and a listing that omits it is
// a listing the operator cannot trust. The refusal happens on ENTER (and on any
// read of a path inside it), returned as a 403 Forbidden carrying the plain
// teaching text, because the carveout lives in the vfs containment primitive
// (vfs.Contain / View.Resolve) that localfileservice funnels every path through —
// so nothing special is done to the listing, and the SAME guard protects the
// local_fs agent tool and a session cwd. Beam enters a subdirectory by keeping
// `root` fixed and passing the child as a relative `path`, which is exactly the
// traversal this guards; the Factory's root-selection check alone would not have
// covered it.
func TestUnit_WorkspaceRoutes_ControlPlaneShownButUnenterable(t *testing.T) {
	root := t.TempDir() // the granted workspace root (stands in for a real home dir)
	require.NoError(t, os.WriteFile(filepath.Join(root, "project.txt"), []byte("x"), 0o644))

	// The runtime's control plane, a DIRECT CHILD of the granted root — the shape
	// serve produces (workspaceRoot = filepath.Dir(contenoxDir)).
	controlPlane := filepath.Join(root, ".contenox")
	require.NoError(t, os.MkdirAll(controlPlane, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(controlPlane, "local.db"), []byte("secret"), 0o600))

	// A sibling whose name merely shares the prefix must remain fully browsable.
	lookalike := filepath.Join(root, ".contenox2")
	require.NoError(t, os.MkdirAll(lookalike, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lookalike, "ok.txt"), []byte("ok"), 0o644))

	require.NoError(t, vfs.SetControlPlaneDenied(controlPlane))
	t.Cleanup(func() { _ = vfs.SetControlPlaneDenied() })

	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	require.NoError(t, localfileapi.AddWorkspaceRoutes(mux, factory, nil))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(path string) (*http.Response, string) {
		u := srv.URL + "/files?root=" + url.QueryEscape(root) + "&path=" + url.QueryEscape(path)
		resp, err := http.Get(u)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, string(body)
	}

	t.Run("listing the parent SHOWS the control-plane entry (honest, not hidden)", func(t *testing.T) {
		resp, body := get(".")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var entries []localfileservice.Entry
		require.NoError(t, json.Unmarshal([]byte(body), &entries))
		names := map[string]bool{}
		for _, e := range entries {
			names[e.Name] = true
		}
		assert.True(t, names[".contenox"], "the control-plane dir must appear in its parent's listing, not be silently hidden")
		assert.True(t, names["project.txt"])
		assert.True(t, names[".contenox2"])
	})

	t.Run("entering the control-plane dir is refused with the teaching error", func(t *testing.T) {
		resp, body := get(".contenox")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "entering the control plane is 403 Forbidden, not a mystery 404/500")
		assert.True(t, strings.Contains(strings.ToLower(body), "control plane"),
			"the refusal body must name the boundary plainly; got %q", body)
	})

	t.Run("reading a file inside the control plane is refused", func(t *testing.T) {
		u := srv.URL + "/files/content?root=" + url.QueryEscape(root) + "&path=" + url.QueryEscape(".contenox/local.db")
		resp, err := http.Get(u)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.True(t, strings.Contains(strings.ToLower(string(body)), "control plane"))
	})

	t.Run("the sibling lookalike (.contenox2) is fully enterable", func(t *testing.T) {
		resp, body := get(".contenox2")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var entries []localfileservice.Entry
		require.NoError(t, json.Unmarshal([]byte(body), &entries))
		var found bool
		for _, e := range entries {
			if e.Name == "ok.txt" {
				found = true
			}
		}
		assert.True(t, found, ".contenox2 is a sibling, not the control plane, and must browse normally")
	})
}
