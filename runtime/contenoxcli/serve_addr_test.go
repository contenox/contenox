package contenoxcli

import (
	"testing"

	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/stretchr/testify/require"
)

func TestUnit_ResolveServeAddr(t *testing.T) {
	t.Run("default is loopback-only", func(t *testing.T) {
		require.Equal(t, defaultServeAddr, resolveServeAddr(false, ""))
	})

	t.Run("--remote binds all interfaces", func(t *testing.T) {
		require.Equal(t, remoteBindAddr, resolveServeAddr(true, ""))
		require.False(t, serverapi.IsLoopbackAddress(resolveServeAddr(true, "")),
			"--remote must produce a non-loopback bind (which then requires a TOKEN)")
	})

	t.Run("an explicit ADDR always wins over --remote", func(t *testing.T) {
		require.Equal(t, "192.168.1.50", resolveServeAddr(true, "192.168.1.50"))
		require.Equal(t, "127.0.0.1", resolveServeAddr(true, " 127.0.0.1 "), "explicit ADDR is honored and trimmed")
	})

	t.Run("ADDR without --remote is honored", func(t *testing.T) {
		require.Equal(t, "0.0.0.0", resolveServeAddr(false, "0.0.0.0"))
	})
}
