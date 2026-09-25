package agentdecl_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/stretchr/testify/require"
)

func TestUnit_LocalRootCanSeedProtectedDeclarations(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".contenox")
	previous := vfs.ControlPlaneDenied()
	require.NoError(t, vfs.SetControlPlaneDenied(dir))
	t.Cleanup(func() { require.NoError(t, vfs.SetControlPlaneDenied(previous...)) })

	root := rootOf(t, dir)
	created, err := agentdecl.Preseed(context.Background(), root)
	require.NoError(t, err)
	require.NotEmpty(t, created.Created)
	child, err := root.Child("agents")
	require.NoError(t, err)
	_, err = child.FS.ReadDir(context.Background(), ".")
	require.NoError(t, err)
	view, err := vfs.OpenView(dir)
	require.NoError(t, err)
	_, err = view.Resolve(".")
	require.ErrorIs(t, err, vfs.ErrControlPlane)
	_, err = root.FS.Sub("../outside")
	require.Error(t, err)
}

// rootOf opens dir as a compilation root, failing the test when it cannot. A
// directory that does not exist is still a root: the code under test is what
// decides whether it holds anything.
func rootOf(t *testing.T, dir string) agentdecl.Root {
	t.Helper()
	root, err := agentdecl.LocalRoot(dir)
	require.NoError(t, err)
	return root
}

// rootsOf opens each directory as a root, in the order given, which is the
// precedence order the compiler reads them in.
func rootsOf(t *testing.T, dirs ...string) []agentdecl.Root {
	t.Helper()
	out := make([]agentdecl.Root, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, rootOf(t, dir))
	}
	return out
}

// syncAt is agentdecl.Sync with the generated directory named as a path, which
// is how these tests read the compiled files back off the disk they wrote.
func syncAt(t *testing.T, ctx context.Context, dirs []agentdecl.SourceDir, generated string, cfg agentdecl.Config, opts ...agentdecl.SyncOption) ([]agentdecl.SyncResult, error) {
	t.Helper()
	return agentdecl.Sync(ctx, dirs, rootOf(t, generated), cfg, opts...)
}
