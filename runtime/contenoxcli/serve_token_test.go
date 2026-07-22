package contenoxcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// isolateHome points ~/.contenox at a temp dir so the token-file helpers operate
// on a scratch location, not the developer's real home. globalContenoxDir reads
// $HOME (os.UserHomeDir honors it on POSIX).
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestUnit_ServeToken_FileRoundTrip(t *testing.T) {
	home := isolateHome(t)

	require.Equal(t, "", readServeTokenFile(), "absent file reads as empty")

	require.NoError(t, writeServeTokenFile("s3cr3t"))
	require.Equal(t, "s3cr3t", readServeTokenFile(), "written token reads back trimmed")

	// It lands at ~/.contenox/serve-token.txt with owner-only perms.
	path := filepath.Join(home, ".contenox", serveTokenFileName)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "token file is owner-only")
}

func TestUnit_ServeToken_ReadTrimsWhitespace(t *testing.T) {
	home := isolateHome(t)
	path := filepath.Join(home, ".contenox", serveTokenFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("  padded-token\n\n"), 0o600))
	require.Equal(t, "padded-token", readServeTokenFile())
}

func TestUnit_ServeToken_ReadEmptyFileIsEmpty(t *testing.T) {
	home := isolateHome(t)
	path := filepath.Join(home, ".contenox", serveTokenFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))
	require.Equal(t, "", readServeTokenFile(), "a whitespace-only file is treated as no token")
}

func TestUnit_GenerateServeToken(t *testing.T) {
	a, err := generateServeToken()
	require.NoError(t, err)
	b, err := generateServeToken()
	require.NoError(t, err)
	require.Len(t, a, 48, "24 random bytes → 48 hex chars")
	require.Regexp(t, "^[0-9a-f]+$", a)
	require.NotEqual(t, a, b, "tokens are random")
}
