package contenoxcli

import (
	"testing"

	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestResolveTerminalConfig_DefaultsEnabledAndRoot(t *testing.T) {
	root := t.TempDir()
	cfg, err := resolveTerminalConfig(&serverapi.Config{}, root)
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, root, cfg.AllowedRoot)
}

func TestResolveTerminalConfig_CanBeDisabled(t *testing.T) {
	cfg, err := resolveTerminalConfig(&serverapi.Config{
		TerminalEnabled: "false",
	}, t.TempDir())
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.AllowedRoot)
}

func TestResolveServeChatConfig_DefaultsLocalShellOn(t *testing.T) {
	ctx, _, store := setupSQLiteStore(t)
	cmd := serveConfigTestCommand(t)

	cfg, err := resolveServeChatConfig(ctx, cmd, store, "workspace", t.TempDir())

	require.NoError(t, err)
	require.True(t, cfg.EnableLocalExec)
}

func TestResolveServeChatConfig_AllowsLocalShellOptOut(t *testing.T) {
	ctx, _, store := setupSQLiteStore(t)
	cmd := serveConfigTestCommand(t)
	require.NoError(t, cmd.Root().Flags().Set("shell", "false"))

	cfg, err := resolveServeChatConfig(ctx, cmd, store, "workspace", t.TempDir())

	require.NoError(t, err)
	require.False(t, cfg.EnableLocalExec)
}

func serveConfigTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "contenox"}
	root.Flags().Bool("shell", false, "")
	serve := &cobra.Command{Use: "serve"}
	root.AddCommand(serve)
	return serve
}
