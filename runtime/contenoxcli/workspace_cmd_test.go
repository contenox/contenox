package contenoxcli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/workspacegrants"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// workspaceTestRoot builds an isolated root command carrying the persistent --db
// flag (as the real rootCmd does) with sub attached, so tests exercise the real
// RunE logic without touching the package-global rootCmd's flag/parent state.
func workspaceTestRoot(sub *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "contenox", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("db", "", "SQLite database path")
	root.AddCommand(sub)
	return root
}

func runWorkspace(t *testing.T, dbPath string, args ...string) (string, error) {
	t.Helper()
	sub := &cobra.Command{Use: "workspace"}
	sub.AddCommand(&cobra.Command{Use: "add", Args: cobra.ExactArgs(1), RunE: runWorkspaceAdd})
	sub.AddCommand(&cobra.Command{Use: "remove", Args: cobra.ExactArgs(1), RunE: runWorkspaceRemove})
	sub.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: runWorkspaceList})
	root := workspaceTestRoot(sub)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"--db", dbPath, "workspace"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func storeAt(t *testing.T, dbPath string) (context.Context, runtimetypes.Store, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDBAt(ctx, dbPath)
	require.NoError(t, err)
	return ctx, runtimetypes.New(db.WithoutTransaction()), func() { _ = db.Close() }
}

func TestUnit_workspaceIsReservedSubcommand(t *testing.T) {
	require.True(t, reservedSubcommands["workspace"], `"workspace" must be reserved so it dispatches as a subcommand`)
	require.True(t, firstNonFlagIsReserved([]string{"workspace", "add", "/x"}))
}

func TestUnit_WorkspaceCLI_AddListRemoveRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace-cli.db")
	grant := t.TempDir()

	out, err := runWorkspace(t, dbPath, "add", grant)
	require.NoError(t, err)
	require.Contains(t, out, filepath.Clean(grant), "add echoes the granted root")

	// The grant is durable in the shared config DB.
	ctx, store, done := storeAt(t, dbPath)
	require.Equal(t, []string{filepath.Clean(grant)}, workspacegrants.ReadGrants(ctx, store))
	done()

	out, err = runWorkspace(t, dbPath, "list")
	require.NoError(t, err)
	require.Contains(t, out, filepath.Clean(grant))

	out, err = runWorkspace(t, dbPath, "remove", grant)
	require.NoError(t, err)
	require.Contains(t, out, "no workspace-root grants configured")

	ctx, store, done = storeAt(t, dbPath)
	require.Empty(t, workspacegrants.ReadGrants(ctx, store))
	done()
}

func TestUnit_WorkspaceCLI_AddNonexistentIsError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace-cli.db")
	out, err := runWorkspace(t, dbPath, "add", filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "does not exist") || strings.Contains(out, "does not exist"),
		"a non-existent grant must be refused with a teaching error")
}

func TestUnit_WorkspaceCLI_ListEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace-cli.db")
	out, err := runWorkspace(t, dbPath, "list")
	require.NoError(t, err)
	require.Contains(t, out, "no workspace-root grants configured")
}
