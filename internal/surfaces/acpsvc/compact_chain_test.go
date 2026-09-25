package acpsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/stretchr/testify/require"
)

func TestUnit_CompactChain_UsesOverridesBeforeSystemCopy(t *testing.T) {
	dir := t.TempDir()
	tr := &Transport{deps: Deps{ContenoxDir: dir}}
	const filename = "chain-compact-default.json"
	for _, sub := range []string{"", agentdecl.GeneratedDirName, SystemDirName} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0700))
		body := fmt.Sprintf(`{"id":%q,"tasks":[{"id":"summarize","handler":"chat_completion"}]}`, "compact-"+sub)
		require.NoError(t, os.WriteFile(filepath.Join(dir, sub, filename), []byte(body), 0600))
	}
	for _, sub := range []string{"", agentdecl.GeneratedDirName, SystemDirName} {
		chain, err := tr.loadCompactChain()
		require.NoError(t, err)
		require.Equal(t, "compact-"+sub, chain.ID)
		require.NoError(t, os.Remove(filepath.Join(dir, sub, filename)))
	}
}

func TestUnit_CompactChain_InvalidOverrideDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, SystemDirName), 0700))
	const filename = "chain-compact-default.json"
	require.NoError(t, os.WriteFile(filepath.Join(dir, SystemDirName, filename), []byte(`{"id":"system","tasks":[{"id":"summarize"}]}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(`{"id":`), 0600))
	tr := &Transport{deps: Deps{ContenoxDir: dir}}
	_, err := tr.loadCompactChain()
	require.ErrorContains(t, err, "invalid chain JSON")
	require.ErrorContains(t, err, filepath.Join(dir, filename))
}
