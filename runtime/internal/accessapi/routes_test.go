package accessapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/accessapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/vfs"
)

// nopKV forces the hitlservice under test to use its constructor fallback
// policy rather than any active-policy KV key.
type nopKV struct{}

func (nopKV) GetKV(context.Context, string, interface{}) error { return os.ErrNotExist }

const testPolicy = `{
  "default_action": "approve",
  "rules": [
    { "tools": "local_fs", "tool": "read_file",  "action": "deny",  "when": [{ "key": "path", "op": "glob", "value": ".ssh/**" }] },
    { "tools": "local_fs", "tool": "write_file", "action": "deny",  "when": [{ "key": "path", "op": "glob", "value": ".ssh/**" }] },
    { "tools": "local_fs", "tool": "read_file",  "action": "allow" },
    { "tools": "local_fs", "tool": "list_dir",   "action": "allow" }
  ]
}`

func newMux(t *testing.T, root string, withHITL bool) *http.ServeMux {
	t.Helper()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	var hitlFor func(string) hitlservice.Service
	if withHITL {
		policyDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(policyDir, "hitl-policy-test.json"), []byte(testPolicy), 0o644))
		hitlFor = func(policyName string) hitlservice.Service {
			return hitlservice.NewWithDefaultPolicy(
				hitlservice.NewFSPolicySource(policyDir), "tenant", nopKV{}, libtracker.NoopTracker{}, policyName)
		}
	}

	mux := http.NewServeMux()
	accessapi.AddRoutes(mux, factory, hitlFor)
	return mux
}

func postAccess(t *testing.T, srv *httptest.Server, query string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	url := srv.URL + "/workspace/access"
	if query != "" {
		url += "?" + query
	}
	resp, err := http.Post(url, "application/json", &buf)
	require.NoError(t, err)
	return resp
}

func TestEvaluateRoute_ShapeAndVerdicts(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ssh"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ssh", "id_rsa"), []byte("s"), 0o600))

	mux := newMux(t, root, true)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := postAccess(t, srv, "policy="+"hitl-policy-test.json", accessapi.EvaluateRequest{
		Paths: []string{"main.go", ".ssh/id_rsa", "../escape.txt"},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out accessapi.EvaluateResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	assert.Equal(t, "hitl-policy-test.json", out.PolicyName)
	require.Len(t, out.Verdicts, 3)

	byPath := map[string]int{}
	for i, v := range out.Verdicts {
		byPath[v.Path] = i
	}

	main := out.Verdicts[byPath["main.go"]]
	assert.True(t, main.Reachable)
	require.NotNil(t, main.Read)
	assert.Equal(t, "allow", main.Read.Action)
	require.NotNil(t, main.Write)
	assert.Equal(t, "approve", main.Write.Action)
	assert.Equal(t, "default_action", main.Write.Reason)
	assert.Nil(t, main.Write.Rule)

	secret := out.Verdicts[byPath[".ssh/id_rsa"]]
	assert.True(t, secret.Reachable)
	require.NotNil(t, secret.Read)
	assert.Equal(t, "deny", secret.Read.Action)
	assert.Equal(t, "matched_rule", secret.Read.Reason)
	require.NotNil(t, secret.Read.Rule)

	escape := out.Verdicts[byPath["../escape.txt"]]
	assert.False(t, escape.Reachable)
	assert.Nil(t, escape.Read)
	assert.Nil(t, escape.Write)
}

func TestEvaluateRoute_EmptyPathsIsOKWithEmptyVerdicts(t *testing.T) {
	root := t.TempDir()
	mux := newMux(t, root, true)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := postAccess(t, srv, "", accessapi.EvaluateRequest{})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out accessapi.EvaluateResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Empty(t, out.Verdicts)
	assert.NotEmpty(t, out.PolicyName, "the response should still name the resolved policy for an empty batch")
}

func TestEvaluateRoute_MalformedBodyIs400(t *testing.T) {
	root := t.TempDir()
	mux := newMux(t, root, true)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/workspace/access", "application/json", bytes.NewBufferString("{not json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestEvaluateRoute_BatchCapExceededIs422(t *testing.T) {
	root := t.TempDir()
	mux := newMux(t, root, true)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	paths := make([]string, 2001)
	for i := range paths {
		paths[i] = fmt.Sprintf("file-%d.txt", i)
	}
	resp := postAccess(t, srv, "", accessapi.EvaluateRequest{Paths: paths})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestEvaluateRoute_UnknownRootIs422(t *testing.T) {
	root := t.TempDir()
	mux := newMux(t, root, true)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := postAccess(t, srv, "root="+t.TempDir(), accessapi.EvaluateRequest{Paths: []string{"main.go"}})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestEvaluateRoute_NoHITLFactoryIs422(t *testing.T) {
	root := t.TempDir()
	mux := newMux(t, root, false)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := postAccess(t, srv, "", accessapi.EvaluateRequest{Paths: []string{"main.go"}})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestAddRoutes_NilFactoryRegistersNothing(t *testing.T) {
	mux := http.NewServeMux()
	accessapi.AddRoutes(mux, nil, nil)

	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/workspace/access", "application/json", bytes.NewBufferString(`{"paths":[]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
