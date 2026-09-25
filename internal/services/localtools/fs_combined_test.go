package localtools_test

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/stretchr/testify/require"
)

// TestUnit_LocalFSTools_OneNamespaceRoutesByTool pins the reason the two halves
// are bound into one repo: registering them under one name would let whichever
// was registered last answer for both, and a model asking to list a directory
// would get the content half's refusal (or the reverse) instead of the tool it
// named.
func TestUnit_LocalFSTools_OneNamespaceRoutesByTool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "note.txt", "content")
	content := localtools.NewLocalFSToolsForTest(root, nil)
	browse := localtools.NewLocalFSBrowseTools(root, nil)
	h := localtools.NewLocalFSToolsFromHalves(content, browse)
	ctx := context.Background()

	names, err := h.Supports(ctx)
	require.NoError(t, err)
	require.Equal(t, localtools.LocalFSToolsName, names[0], "the namespace is addressed by one name")
	require.ElementsMatch(t,
		[]string{"local_fs", "read_file", "read_file_range", "write_file", "edit_file", "sed",
			"list_dir", "grep", "find_files", "count_stats", "stat_file"},
		names, "both halves are advertised under the one name")

	tools, err := h.GetToolsForToolsByName(ctx, localtools.LocalFSToolsName)
	require.NoError(t, err)
	require.Len(t, tools, 10, "the model is shown one toolset, not two halves")

	listed, _, err := h.Exec(ctx, time.Now(), map[string]any{"path": "."}, false,
		&taskengine.ToolsCall{Name: localtools.LocalFSToolsName, ToolName: "list_dir"})
	require.NoError(t, err, "list_dir is served by the browse half")
	require.Contains(t, listed.(string), "note.txt")

	read, _, err := h.Exec(ctx, time.Now(), map[string]any{"path": "note.txt"}, false,
		&taskengine.ToolsCall{Name: localtools.LocalFSToolsName, ToolName: "read_file"})
	require.NoError(t, err, "read_file is served by the content half")
	require.Contains(t, read.(string), "content")

	schemas, err := h.GetSchemasForSupportedTools(ctx)
	require.NoError(t, err)
	require.Len(t, schemas, 1, "one namespace publishes one contract")
}

// TestUnit_LocalFSTools_BrowseHalvesAreInProcessKeepers pins which leaves an
// attached client is needed for: the ACP surface reads this to decide what a
// clientless session may still call.
func TestUnit_LocalFSTools_BrowseHalvesAreInProcessKeepers(t *testing.T) {
	for _, leaf := range []string{"list_dir", "grep", "find_files", "count_stats", "stat_file"} {
		require.Truef(t, localtools.IsLocalFSBrowseTool(leaf), "%q runs in process", leaf)
	}
	for _, leaf := range []string{"read_file", "read_file_range", "write_file", "edit_file", "sed", "unknown"} {
		require.Falsef(t, localtools.IsLocalFSBrowseTool(leaf), "%q is not a browse tool", leaf)
	}
}
