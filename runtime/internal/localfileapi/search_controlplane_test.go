package localfileapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

// TestUnit_Search_ExcludesControlPlaneRecursion pins the carveout on rg's WALK,
// not its root: a denied directory can never BE the search root (the Factory
// refuses it — covered in the vfs suite), but rg recurses, so a search rooted
// at a granted PARENT of the control plane would read the runtime's own
// config/policies/DB into results without the per-denied-dir glob excludes
// search.go builds. This is the residual surface the carveout slice flagged;
// the sibling-named lookalike stays searchable to prove the exclusion is
// segment-exact, not substring-sloppy.
func TestUnit_Search_ExcludesControlPlaneRecursion(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; the exclusion args are still covered by the build, skipping the live walk")
	}

	root := t.TempDir()
	cpDir := filepath.Join(root, "fakecp")
	lookalike := filepath.Join(root, "fakecp2")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.MkdirAll(lookalike, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "secret.txt"), []byte("needle inside control plane\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lookalike, "open.txt"), []byte("needle in lookalike\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plain.txt"), []byte("needle in workspace\n"), 0o644))

	require.NoError(t, vfs.SetControlPlaneDenied(cpDir))
	t.Cleanup(func() { require.NoError(t, vfs.SetControlPlaneDenied()) })

	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	AddWorkspaceSearchRoutes(mux, factory)

	req := httptest.NewRequest(http.MethodGet, "/workspace/search?q=needle", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "plain.txt", "the ordinary workspace file must match")
	require.Contains(t, body, "open.txt", "the sibling-named lookalike must stay searchable — the exclude is segment-exact")
	require.NotContains(t, body, "secret.txt", "rg must not recurse into the denied control-plane dir")
	require.False(t, strings.Contains(body, "fakecp/"), "no path under the denied dir may appear in any frame")
}
