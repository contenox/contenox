//go:build windows

package libprocess

import (
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on Windows: there is no Setpgid equivalent, and
// job objects (the real answer for killing a process tree here) are more
// machinery than this package takes on. Wrapper commands may therefore leak
// grandchildren on Windows — see killProcessTree.
func setProcessGroup(cmd *exec.Cmd) {}

// signalGraceful signals the direct child only. Windows has no SIGINT for an
// arbitrary process, so os.Interrupt is unsupported by the runtime here and
// this falls through to the caller's kill escalation.
func signalGraceful(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

// killProcessTree kills the direct child only — see setProcessGroup for why.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
