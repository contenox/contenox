package localfileapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentview"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nopKV forces the hitlservice to use its constructor fallback policy.
type nopKV struct{}

func (nopKV) GetKV(context.Context, string, interface{}) error { return os.ErrNotExist }

const testPolicy = `{
  "default_action": "approve",
  "rules": [
    { "tools": "local_fs", "tool": "read_file",  "action": "deny",    "when": [{ "key": "path", "op": "glob", "value": "secret/**" }] },
    { "tools": "local_fs", "tool": "write_file", "action": "deny",    "when": [{ "key": "path", "op": "glob", "value": "secret/**" }] },
    { "tools": "local_fs", "tool": "read_file",  "action": "allow" },
    { "tools": "local_fs", "tool": "list_dir",   "action": "allow" },
    { "tools": "local_fs", "tool": "write_file", "action": "approve" }
  ]
}`

// annotatedEntry mirrors the /files response element (the service Entry fields
// are promoted; `access` is the agent-view annotation).
type annotatedEntry struct {
	Path        string             `json:"path"`
	Name        string             `json:"name"`
	IsDirectory bool               `json:"isDirectory"`
	Size        int64              `json:"size"`
	Access      *agentview.Verdict `json:"access"`
}

func TestUnit_WorkspaceRoutes_AgentFilter(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "secret"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret", "keep"), []byte("s"), 0o644))

	// A symlink escaping the root, which must list as reachable:false.
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "t.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "t.txt"), filepath.Join(root, "escape.txt")))

	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "hitl-policy-test.json"), []byte(testPolicy), 0o644))
	hitlFor := func(policyName string) hitlservice.Service {
		return hitlservice.NewWithDefaultPolicy(
			hitlservice.NewFSPolicySource(policyDir), "tenant", nopKV{}, libtracker.NoopTracker{}, policyName)
	}

	mux := http.NewServeMux()
	require.NoError(t, localfileapi.AddWorkspaceRoutes(mux, factory, hitlFor))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(query string) (*http.Response, []byte) {
		resp, err := http.Get(srv.URL + "/files?" + query)
		require.NoError(t, err)
		buf := make([]byte, 0)
		var tmp [4096]byte
		for {
			n, e := resp.Body.Read(tmp[:])
			buf = append(buf, tmp[:n]...)
			if e != nil {
				break
			}
		}
		_ = resp.Body.Close()
		return resp, buf
	}

	// 1. Unfiltered /files is unchanged: no `access` field anywhere.
	resp, raw := get("path=.")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(raw), "\"access\"", "the unfiltered tree must not carry access annotations")

	// 1b. filter=full is byte-identical to the default.
	respFull, rawFull := get("path=.&filter=full")
	require.Equal(t, http.StatusOK, respFull.StatusCode)
	assert.Equal(t, string(raw), string(rawFull), "filter=full must match the default response byte-for-byte")

	// 2. filter=agent annotates every entry with a verdict.
	resp, raw = get("path=.&filter=agent&policy=" + url.QueryEscape("hitl-policy-test.json"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var entries []annotatedEntry
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.NotEmpty(t, entries)

	byName := map[string]annotatedEntry{}
	for _, e := range entries {
		require.NotNilf(t, e.Access, "entry %q must carry an access verdict under filter=agent", e.Name)
		byName[e.Name] = e
	}

	// main.go: reachable, read allow, write approve.
	require.Contains(t, byName, "main.go")
	assert.True(t, byName["main.go"].Access.Reachable)
	assert.Equal(t, hitlservice.ActionAllow, byName["main.go"].Access.Read)
	assert.Equal(t, hitlservice.ActionApprove, byName["main.go"].Access.Write)

	// secret dir: reachable, list_dir allowed, write (create-inside) denied.
	require.Contains(t, byName, "secret")
	assert.True(t, byName["secret"].Access.Reachable)
	assert.Equal(t, hitlservice.ActionDeny, byName["secret"].Access.Write)

	// escape.txt: the symlink escapes the root, so it is annotated
	// reachable:false with empty actions (returned, not omitted).
	require.Contains(t, byName, "escape.txt")
	assert.False(t, byName["escape.txt"].Access.Reachable)
	assert.Empty(t, string(byName["escape.txt"].Access.Read))
	assert.Empty(t, string(byName["escape.txt"].Access.Write))
}

func TestUnit_WorkspaceRoutes_AgentFilter_Unavailable(t *testing.T) {
	root := t.TempDir()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	mux := http.NewServeMux()
	// nil hitlFor => agent filter is not available.
	require.NoError(t, localfileapi.AddWorkspaceRoutes(mux, factory, nil))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/files?path=.&filter=agent")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.GreaterOrEqual(t, resp.StatusCode, 400, "filter=agent without a HITL factory must be rejected")
}
