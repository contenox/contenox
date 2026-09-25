package agentdecl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/stretchr/testify/require"
)

// The sync state keys every source by the path it was read from. A root reached
// through a symlink must therefore produce the same key as the directory it
// resolves to, or a checkout that moved behind a link would look like every
// declaration vanishing at once: the pass would retire each chain and chain
// discovery disables an agent whose chain file is gone.
func TestUnit_Sync_ResolvedRootKeySurvivesASymlinkedPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	declare(t, filepath.Join(root, agentdecl.NativeSourceDir), "triage.md", declTriage)
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(root, link))

	generated := filepath.Join(root, agentdecl.GeneratedDirName)

	dirs := agentdecl.DiscoverSourceDirs(ctx, rootsOf(t, link), nil)
	results, err := syncAt(t, ctx, dirs, generated, mustConfig(t))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, agentdecl.ActionCreated, results[0].Action)
	require.Equal(t, filepath.Join(root, agentdecl.NativeSourceDir, "triage.md"), results[0].Source,
		"a source is named by its resolved path")

	dirs = agentdecl.DiscoverSourceDirs(ctx, rootsOf(t, root), nil)
	results, err = syncAt(t, ctx, dirs, generated, mustConfig(t))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, agentdecl.ActionUnchanged, results[0].Action,
		"the same declaration reached by another spelling must not be compiled twice")
}
