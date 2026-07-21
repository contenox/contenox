package contenoxcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
	"github.com/stretchr/testify/require"
)

// rootsResponse mirrors the /workspace/roots wire shape for decoding.
type rootsResponse struct {
	Roots []struct {
		Path    string `json:"path"`
		Default bool   `json:"default"`
	} `json:"roots"`
}

func (r rootsResponse) paths() []string {
	out := make([]string, 0, len(r.Roots))
	for _, e := range r.Roots {
		out = append(out, e.Path)
	}
	return out
}

func getRoots(t *testing.T, base string) rootsResponse {
	t.Helper()
	resp, err := http.Get(base + "/workspace/roots")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body rootsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

// TestSystem_WorkspaceGrants_LANStoryEndToEnd proves the whole slice without a
// serve restart: a serve-shaped harness (real DB, real SQLite bus, the actual
// reloader + doorbell subscriber + REST mutators serve wires) is driven exactly
// as a LAN operator's browser would — GET the baseline, POST a new root, GET it
// reflected, confirm a dispatch/session cwd UNDER the new root now validates,
// DELETE it, confirm the cwd is refused again — and then the cross-process path:
// a SECOND process (a fresh bus publisher + a direct config write, standing in
// for `contenox workspace add` in another process) rings the doorbell and serve
// applies it live.
func TestSystem_WorkspaceGrants_LANStoryEndToEnd(t *testing.T) {
	ctx := context.Background()

	baseRoot := t.TempDir()
	grantRoot := t.TempDir()
	grantSub := filepath.Join(grantRoot, "project", "pkg")
	require.NoError(t, os.MkdirAll(grantSub, 0o755))

	dbPath := filepath.Join(t.TempDir(), "workspace-e2e.db")
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	store := runtimetypes.New(db.WithoutTransaction())
	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer func() { _ = bus.Close() }()

	// The serve-shaped wiring: a Factory holding only the base root, the reloader
	// (base ∪ durable grants), the doorbell subscriber, and the REST mutators.
	factory, err := vfs.NewFactory(baseRoot)
	require.NoError(t, err)
	reloader := newWorkspaceRootReloader(factory, []string{baseRoot}, store)
	stop, err := startWorkspaceRootReloader(ctx, bus, reloader)
	require.NoError(t, err)
	defer stop()

	mux := http.NewServeMux()
	localfileapi.AddWorkspaceRootsRoutes(mux, factory, reloader.mutators(bus))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resolvedBase, _ := vfs.ResolveRoot(baseRoot)
	resolvedGrant, _ := vfs.ResolveRoot(grantRoot)
	resolvedSub, _ := vfs.ResolveRoot(grantSub)

	// 1. Baseline: only the base root, marked default.
	baseline := getRoots(t, srv.URL)
	require.Equal(t, []string{resolvedBase}, baseline.paths())
	require.True(t, baseline.Roots[0].Default)

	// 2. A cwd under the not-yet-granted root is refused (the containment boundary).
	_, err = vfs.ResolveSessionCwd(factory, grantSub, "")
	require.Error(t, err, "a cwd under an ungranted root must be refused before the grant")

	// 3. POST the new root (the LAN operator's hands).
	body, _ := json.Marshal(map[string]string{"path": grantRoot})
	resp, err := http.Post(srv.URL+"/workspace/roots", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// 4. GET reflects the new root immediately (no restart, no bus delay — the POST
	// applied synchronously).
	afterAdd := getRoots(t, srv.URL)
	require.ElementsMatch(t, []string{resolvedBase, resolvedGrant}, afterAdd.paths())

	// 5. A dispatch/session cwd UNDER the new root now validates, resolving to the
	// SUBPATH (containment), which is exactly what a `mission fire --cwd` would do.
	resolved, err := vfs.ResolveSessionCwd(factory, grantSub, "")
	require.NoError(t, err)
	require.Equal(t, resolvedSub, resolved)

	// 6. DELETE the root.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/workspace/roots?path="+resolvedGrant, nil)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// 7. The cwd under the revoked root is refused again.
	afterDelete := getRoots(t, srv.URL)
	require.Equal(t, []string{resolvedBase}, afterDelete.paths())
	_, err = vfs.ResolveSessionCwd(factory, grantSub, "")
	require.Error(t, err, "after revoke, a cwd under the (former) root is refused again")

	// 8. Cross-process doorbell: a SECOND process (a fresh bus + a direct config
	// write, standing in for `contenox workspace add` running elsewhere) grants a
	// root and rings the doorbell. serve's subscriber re-reads the durable config
	// and applies it LIVE — the factory grows without any REST call and without a
	// restart.
	grant2 := t.TempDir()
	resolvedGrant2, _ := vfs.ResolveRoot(grant2)
	cliBus := libbus.NewSQLite(db.WithoutTransaction()) // a distinct publisher
	defer func() { _ = cliBus.Close() }()
	roots, err := workspacegrants.Add(ctx, store, grant2)
	require.NoError(t, err)
	require.NoError(t, workspacegrants.PublishChanged(ctx, cliBus, roots))

	require.Eventually(t, func() bool {
		_, ok := factory.Allows(grant2)
		return ok
	}, 10*time.Second, 50*time.Millisecond,
		"the cross-process doorbell never applied the grant to the live factory")

	final := getRoots(t, srv.URL)
	require.ElementsMatch(t, []string{resolvedBase, resolvedGrant2}, final.paths())
}
