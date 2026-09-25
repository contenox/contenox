package contenoxcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/modeld/modelstore"
	"github.com/stretchr/testify/require"
)

func TestUnit_NativeList_OfflineInventory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CONTENOX_DATA_ROOT", root)
	dir := filepath.Join(modelstore.Dir(root, ""), "installed")
	require.NoError(t, os.MkdirAll(dir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.gguf"), []byte("weights"), 0600))
	cmd := newNativeListCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "installed")
	require.Contains(t, out.String(), "llama")
	_, err := os.Stat(filepath.Join(root, "modeld.lease"))
	require.True(t, os.IsNotExist(err))
}

func TestUnit_NativeVerbs_Dispatch(t *testing.T) {
	for _, name := range []string{"pull", "list", "ls", "show", "ps", "stop"} {
		require.True(t, reservedSubcommands[name], name)
		cmd, _, err := rootCmd.Find([]string{name})
		require.NoError(t, err)
		require.NotEqual(t, rootCmd, cmd, name)
	}
}
