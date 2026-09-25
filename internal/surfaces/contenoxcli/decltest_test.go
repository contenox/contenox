package contenoxcli

import (
	"testing"

	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/stretchr/testify/require"
)

// declRootForTest opens a directory as a declaration root for the CLI tests,
// which drive the same compile pass a run does.
func declRootForTest(t *testing.T, dir string) agentdecl.Root {
	t.Helper()
	root, err := agentdecl.LocalRoot(dir)
	require.NoError(t, err)
	return root
}
