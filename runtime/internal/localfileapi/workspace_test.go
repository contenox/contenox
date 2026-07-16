package localfileapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_WorkspaceRoutes_PerRootAndAllowlist(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "in-a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "in-b.txt"), []byte("b"), 0o644))

	factory, err := vfs.NewFactory(rootA, rootB)
	require.NoError(t, err)

	mux := http.NewServeMux()
	require.NoError(t, localfileapi.AddWorkspaceRoutes(mux, factory))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	list := func(root string) (*http.Response, []localfileservice.Entry) {
		u := srv.URL + "/files?path=."
		if root != "" {
			u += "&root=" + url.QueryEscape(root)
		}
		resp, err := http.Get(u)
		require.NoError(t, err)
		var entries []localfileservice.Entry
		if resp.StatusCode == http.StatusOK {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&entries))
		}
		_ = resp.Body.Close()
		return resp, entries
	}

	names := func(entries []localfileservice.Entry) []string {
		var out []string
		for _, e := range entries {
			out = append(out, e.Name)
		}
		return out
	}

	// Default root (no `root` param) serves rootA.
	resp, entries := list("")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, names(entries), "in-a.txt")

	// Explicit allowlisted rootB serves rootB.
	resp, entries = list(rootB)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, names(entries), "in-b.txt")
	assert.NotContains(t, names(entries), "in-a.txt")

	// A non-allowlisted root is refused.
	resp, _ = list(t.TempDir())
	assert.GreaterOrEqual(t, resp.StatusCode, 400, "a root outside the allowlist must be rejected")
}
