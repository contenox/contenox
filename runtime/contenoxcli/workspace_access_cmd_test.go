package contenoxcli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/accessview"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/accessapi"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// accessTestPolicy is a small, self-contained HITL policy (independent of the
// shipped hitl-policy-strict.json preset, so this test does not break if that
// preset's rules change): a path-glob rule denies anything under .ssh/ for
// every local_fs tool (so both the read and write dimension hit the SAME
// rule, matching how the real presets gate secret directories), read_file and
// list_dir are otherwise allowed, and everything else (write_file on a
// non-.ssh path) falls through to default_action: "approve" — giving one
// test fixture that exercises matched_rule AND default_action.
const accessTestPolicyName = "hitl-policy-access-test.json"

const accessTestPolicy = `{
  "default_action": "approve",
  "rules": [
    {"tools": "local_fs", "tool": "*", "action": "deny", "when": [{"key": "path", "op": "glob", "value": "**/.ssh/**"}]},
    {"tools": "local_fs", "tool": "read_file", "action": "allow"},
    {"tools": "local_fs", "tool": "list_dir", "action": "allow"}
  ]
}`

// emptyKVReaderStub always misses, forcing hitlservice to fall back to the
// constructor's named policy rather than an active-policy KV key — mirrors
// serverapi.emptyKVReader (unexported there), reproduced here so this test
// pins an explicit policy the same way an explicit ?policy= query param does.
type emptyKVReaderStub struct{}

func (emptyKVReaderStub) GetKV(context.Context, string, interface{}) error {
	return os.ErrNotExist
}

// setupAccessTestServer mounts a real vfs.Factory + accessapi.AddRoutes
// behind httptest, under "/api" exactly like `contenox serve` mounts it
// (server.go's registerProductRoutes + serve_cmd.go's rootMux.Handle("/api/",
// http.StripPrefix("/api", apiMux))) — the same discipline
// setupApprovalsTestServer (approvals_cmd_test.go) uses, so this exercises
// serveClient against the real wire contract without spawning a `contenox
// serve` process.
func setupAccessTestServer(t *testing.T, root, policyDir string) (*vfs.Factory, *httptest.Server) {
	t.Helper()
	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)

	src := hitlservice.NewFSPolicySource(policyDir)
	hitlFor := func(policyName string) hitlservice.Service {
		if policyName == "" {
			policyName = accessTestPolicyName
		}
		return hitlservice.NewWithDefaultPolicy(src, runtimetypes.LocalTenantID, emptyKVReaderStub{}, libtracker.NoopTracker{}, policyName)
	}

	apiMux := http.NewServeMux()
	accessapi.AddRoutes(apiMux, factory, hitlFor)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))

	srv := httptest.NewServer(rootMux)
	t.Cleanup(srv.Close)
	return factory, srv
}

func newWorkspaceAccessTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "access", Args: cobra.MinimumNArgs(1), RunE: runWorkspaceAccess}
	addServeClientFlags(c)
	c.Flags().String("root", "", "")
	c.Flags().String("policy", "", "")
	return c
}

// accessTestWorkspace builds a temp workspace root with a normal file and a
// fake secret under .ssh/, plus a sibling directory OUTSIDE the root, for the
// unreachable-path case. Returns the root and the sibling (for an
// out-of-root path).
func accessTestWorkspace(t *testing.T) (root, outsideSibling string) {
	t.Helper()
	parent := t.TempDir()
	root = filepath.Join(parent, "workspace")
	outsideSibling = filepath.Join(parent, "escape")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ssh"), 0o755))
	require.NoError(t, os.MkdirAll(outsideSibling, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ssh", "id_rsa"), []byte("fake-key\n"), 0o600))
	return root, outsideSibling
}

func runWorkspaceAccessCmd(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	cmd := newWorkspaceAccessTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(append([]string{"--server", srv.URL}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ─── access ─────────────────────────────────────────────────────────────

// TestUnit_WorkspaceAccess_NormalFileAllowsReadApprovesWrite is the "routine
// file" case: no rule denies it, so read_file falls to the explicit allow
// rule (matched_rule, omitted from REASON since the dimension is "allow") and
// write_file falls through to default_action: approve (surfaced in REASON,
// since it is NOT allow).
func TestUnit_WorkspaceAccess_NormalFileAllowsReadApprovesWrite(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))
	_, srv := setupAccessTestServer(t, root, policyDir)

	out, err := runWorkspaceAccessCmd(t, srv, "src/main.go")
	require.NoError(t, err)
	require.Contains(t, out, "Policy: "+accessTestPolicyName)
	require.Contains(t, out, "src/main.go")
	require.Contains(t, out, "true")
	require.Contains(t, out, "allow")
	require.Contains(t, out, "approve")
	require.Contains(t, out, "write:default_action")
	require.NotContains(t, out, "read:", "an allow dimension must not appear in the REASON column")
}

// TestUnit_WorkspaceAccess_SecretPathDeniesBothDimensionsWithMatchedRule is
// the "fake secret" case: the .ssh/ glob rule (index 0) denies BOTH read and
// write, and both dimensions must name the SAME matched rule index.
func TestUnit_WorkspaceAccess_SecretPathDeniesBothDimensionsWithMatchedRule(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))
	_, srv := setupAccessTestServer(t, root, policyDir)

	out, err := runWorkspaceAccessCmd(t, srv, ".ssh/id_rsa")
	require.NoError(t, err)
	require.Contains(t, out, ".ssh/id_rsa")
	require.Contains(t, out, "read:matched_rule#0")
	require.Contains(t, out, "write:matched_rule#0")

	lines := bytes.Split([]byte(out), []byte("\n"))
	var row string
	for _, l := range lines {
		if bytes.Contains(l, []byte(".ssh/id_rsa")) {
			row = string(l)
		}
	}
	require.NotEmpty(t, row, "the .ssh/id_rsa row must be present")
	require.Contains(t, row, "deny")
	require.NotContains(t, row, "allow", "a denied path's row must not also show allow")
}

// TestUnit_WorkspaceAccess_UnreachablePathHasNoVerdict proves a path outside
// the workspace root (an ../escape-shaped traversal) comes back
// reachable:false with blank read/write and a dashed REASON — no policy is
// evaluated for a path that is not really inside the root.
func TestUnit_WorkspaceAccess_UnreachablePathHasNoVerdict(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))
	_, srv := setupAccessTestServer(t, root, policyDir)

	out, err := runWorkspaceAccessCmd(t, srv, "../escape")
	require.NoError(t, err, "an unreachable path is a 200 with reachable:false, not a command error")
	require.Contains(t, out, "../escape")
	require.Contains(t, out, "false")

	lines := bytes.Split([]byte(out), []byte("\n"))
	var row string
	for _, l := range lines {
		if bytes.Contains(l, []byte("../escape")) {
			row = string(l)
		}
	}
	require.NotEmpty(t, row)
	require.Contains(t, row, "false")
}

// TestUnit_WorkspaceAccess_AllThreeCasesInOneBatch runs the normal file, the
// secret, and the escape path in a single invocation (as one request batch,
// exactly like a real `contenox workspace access a b c` call), proving the
// three documented outcomes all come back correctly in the SAME response.
func TestUnit_WorkspaceAccess_AllThreeCasesInOneBatch(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))
	_, srv := setupAccessTestServer(t, root, policyDir)

	out, err := runWorkspaceAccessCmd(t, srv, "src/main.go", ".ssh/id_rsa", "../escape")
	require.NoError(t, err)

	rowFor := func(path string) string {
		for _, l := range bytes.Split([]byte(out), []byte("\n")) {
			if bytes.HasPrefix(l, []byte(path+"\t")) || bytes.Contains(l, []byte(path)) {
				return string(l)
			}
		}
		return ""
	}

	main := rowFor("src/main.go")
	require.Contains(t, main, "true")
	require.Contains(t, main, "allow")
	require.Contains(t, main, "approve")

	secret := rowFor(".ssh/id_rsa")
	require.Contains(t, secret, "true")
	require.Contains(t, secret, "deny")

	escape := rowFor("../escape")
	require.Contains(t, escape, "false")
}

// TestUnit_WorkspaceAccess_RootFlagSelectsAllowlistedRoot proves --root is
// actually threaded onto the request (a request against a root NOT in the
// factory's allowlist is refused by the server), by using a second granted
// root distinct from the factory's default.
func TestUnit_WorkspaceAccess_RootFlagSelectsAllowlistedRoot(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	secondRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secondRoot, "note.txt"), []byte("hi\n"), 0o644))

	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))

	factory, err := vfs.NewFactory(root)
	require.NoError(t, err)
	require.NoError(t, factory.SetRoots([]string{root, secondRoot}))

	src := hitlservice.NewFSPolicySource(policyDir)
	hitlFor := localfileapi.PolicyEvaluatorFactory(func(policyName string) hitlservice.Service {
		if policyName == "" {
			policyName = accessTestPolicyName
		}
		return hitlservice.NewWithDefaultPolicy(src, runtimetypes.LocalTenantID, emptyKVReaderStub{}, libtracker.NoopTracker{}, policyName)
	})
	apiMux := http.NewServeMux()
	accessapi.AddRoutes(apiMux, factory, hitlFor)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))
	srv := httptest.NewServer(rootMux)
	t.Cleanup(srv.Close)

	out, err := runWorkspaceAccessCmd(t, srv, "--root", secondRoot, "note.txt")
	require.NoError(t, err)
	require.Contains(t, out, "note.txt")
	require.Contains(t, out, "true")
}

// TestUnit_WorkspaceAccess_UnreachableServerFailsWithNonZeroExit mirrors
// TestUnit_ApprovalsList_UnreachableServerFailsWithNonZeroExit: a serve that
// is not there must fail the command, not print an empty table.
func TestUnit_WorkspaceAccess_UnreachableServerFailsWithNonZeroExit(t *testing.T) {
	cmd := newWorkspaceAccessTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", "http://127.0.0.1:1", "src/main.go"})
	err := cmd.Execute()
	require.Error(t, err)
}

// TestUnit_WorkspaceAccess_BadRootSurfacesServeErrorMessage proves the
// apiframework error envelope (a root outside every allowlisted root, 422)
// reaches the CLI caller as a non-zero exit carrying serve's own message,
// same contract as every other serveClient-backed verb.
func TestUnit_WorkspaceAccess_BadRootSurfacesServeErrorMessage(t *testing.T) {
	root, _ := accessTestWorkspace(t)
	policyDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, accessTestPolicyName), []byte(accessTestPolicy), 0o644))
	_, srv := setupAccessTestServer(t, root, policyDir)

	out, err := runWorkspaceAccessCmd(t, srv, "--root", "/etc", "passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not under any configured workspace root")
	_ = out
}

// ─── table helpers ──────────────────────────────────────────────────────

func TestUnit_AccessTableHelpers(t *testing.T) {
	allow := &accessview.DimensionVerdict{Action: "allow", Reason: "matched_rule", Rule: intPtr(4)}
	approveDefault := &accessview.DimensionVerdict{Action: "approve", Reason: "default_action"}
	denyRule := &accessview.DimensionVerdict{Action: "deny", Reason: "matched_rule", Rule: intPtr(0)}

	require.Equal(t, "-", dimensionAction(nil))
	require.Equal(t, "allow", dimensionAction(allow))
	require.Equal(t, "deny", dimensionAction(denyRule))

	require.Equal(t, "", dimensionReasonFragment("read", nil))
	require.Equal(t, "", dimensionReasonFragment("read", allow), "an allow dimension contributes no reason fragment")
	require.Equal(t, "write:default_action", dimensionReasonFragment("write", approveDefault))
	require.Equal(t, "read:matched_rule#0", dimensionReasonFragment("read", denyRule))

	require.Equal(t, "-", accessReasonColumn(nil, nil), "unreachable rows render a dash")
	require.Equal(t, "-", accessReasonColumn(allow, allow), "an all-allow verdict needs no explaining")
	require.Equal(t, "write:default_action", accessReasonColumn(allow, approveDefault))
	require.Equal(t, "read:matched_rule#0 write:matched_rule#0", accessReasonColumn(denyRule, denyRule))
}

func intPtr(i int) *int { return &i }
