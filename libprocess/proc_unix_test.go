//go:build unix

package libprocess_test

import (
	"os"
	"os/exec"
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

// The kill is not guaranteed to end the wait. A descendant that left the
// process group (setsid here, a double-forking daemon in the wild) survives
// the group kill and keeps the inherited stdout pipe open, and os/exec's Wait
// does not return until that pipe reaches EOF. Stop must give up after
// KillReapGrace and say why, rather than block its caller for as long as the
// escapee feels like living.
func TestUnit_Process_StopBoundsTheReapWhenAnEscapeeHoldsThePipes(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available; cannot simulate a descendant outside the process group")
	}

	pidFile := filepath.Join(t.TempDir(), "escapee.pid")
	// The escapee inherits our stdout pipe and leaves the process group, so
	// the group kill cannot reach it. Stdout is a non-*os.File writer on
	// purpose: that is what makes os/exec run a copier whose completion Wait
	// waits for.
	cmd, args := shArgs(`trap "" INT; setsid sh -c 'echo $$ > ` + pidFile + `; sleep 30' & while true; do sleep 0.05; done`)

	p := newProc(t, libprocess.Config{
		Command:       cmd,
		Args:          args,
		Stdout:        &libprocess.LockedBuffer{},
		StopGrace:     150 * time.Millisecond,
		KillReapGrace: 300 * time.Millisecond,
	})
	require.NoError(t, p.Start(t.Context()))

	var escapee int
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		escapee, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil && escapee > 0
	}, 3*time.Second, 20*time.Millisecond, "the escapee never reported its pid")
	t.Cleanup(func() { _ = syscall.Kill(escapee, syscall.SIGKILL) })

	type result struct {
		err     error
		elapsed time.Duration
	}
	res := make(chan result, 1)
	go func() {
		start := time.Now()
		err := p.Stop(t.Context())
		res <- result{err, time.Since(start)}
	}()

	select {
	case got := <-res:
		// Without the bound this call waits on the escapee's own 30s sleep.
		require.Error(t, got.err, "Stop returned success for a process it could not reap")
		require.Contains(t, got.err.Error(), "not reaped")
		require.Less(t, got.elapsed, 5*time.Second)
	case <-time.After(10 * time.Second):
		t.Fatal("Stop hung on an unreapable process: the post-kill wait is unbounded")
	}

	// Done is still open, which is the direct evidence that the wait had not
	// returned: an unbounded post-kill `<-done` would still be parked here,
	// for as long as the escapee holds the pipe.
	select {
	case <-p.Done():
		t.Fatal("the supervised lifetime concluded after all; this test no longer proves the bound")
	default:
	}
}
