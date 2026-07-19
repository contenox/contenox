//go:build unix

package libprocess

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the spawned command the leader of its own process
// group. Supervised commands are frequently wrappers (npx/uvx shims, `sh -c`,
// language launchers) whose real work runs one or two forks down; without a
// group, signalling only the direct child leaves that grandchild alive — and,
// because it inherited our stdout/stderr pipes, it can block the cmd.Wait
// reaper indefinitely, which would hang Stop.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalGraceful asks the whole process group to shut down cleanly, falling
// back to the direct child when group delivery is unavailable.
func signalGraceful(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
		return cmd.Process.Signal(os.Interrupt)
	}
	return nil
}

// killProcessTree force-kills the command's entire process group (see
// setProcessGroup), falling back to the direct child if group delivery fails.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

// exitFromKill reports whether waitErr records death by SIGKILL — the only
// exit Stop's kill escalation can actually have caused. The distinction
// matters because "the kill branch ran" does not imply "the kill did it": on a
// loaded machine the Wait reaper can lag past the grace period for a process
// that already exited on its own, and that process's genuine exit status is a
// real failure the caller must still see. Suppressing every exit error after
// the kill branch would silently swallow it.
func exitFromKill(waitErr error) bool {
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL
}

// exitFromGracefulSignal reports whether waitErr records death by the signal
// signalGraceful sends. It is consulted only when the default graceful
// strategy is in use (see SignalGroup): a command that dies from the interrupt
// we ourselves delivered did what we asked, and reporting "signal: interrupt"
// as Stop's outcome would turn every successful shutdown of a command that
// does not install a SIGINT handler into an error.
func exitFromGracefulSignal(waitErr error) bool {
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGINT
}
