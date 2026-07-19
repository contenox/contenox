//go:build unix

package libprocess_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/contenox/runtime/libprocess"
	"github.com/stretchr/testify/require"
)

// Supervised commands are commonly wrappers (npx/uvx shims, sh -c) whose real
// workload is a grandchild. Stop must reap that whole tree: a survivor keeps
// holding the inherited stdio pipes, which is exactly what wedges cmd.Wait.
func TestUnit_Process_StopKillsGrandchildOfWrapper(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	// The wrapper ignores SIGINT, so only the group kill can end this tree.
	cmd, args := shArgs(`trap "" INT; sleep 30 & echo $! > ` + pidFile + `; wait`)

	p := newProc(t, libprocess.Config{
		Command:   cmd,
		Args:      args,
		StopGrace: 200 * time.Millisecond,
	})
	require.NoError(t, p.Start(t.Context()))

	var grandchild int
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		grandchild, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil && grandchild > 0
	}, 3*time.Second, 20*time.Millisecond, "wrapper never reported its grandchild")

	require.NoError(t, syscall.Kill(grandchild, 0), "grandchild should be alive before Stop")

	done := make(chan error, 1)
	go func() { done <- p.Stop(t.Context()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung: the grandchild is still holding the inherited pipes")
	}

	// Signal 0 only probes for existence. ESRCH means it is gone; EPERM would
	// mean it lives on under another owner, which is still a leak.
	require.Eventually(t, func() bool {
		return syscall.Kill(grandchild, 0) == syscall.ESRCH
	}, 3*time.Second, 20*time.Millisecond, "grandchild survived Stop (process-group kill did not reach it)")
}
