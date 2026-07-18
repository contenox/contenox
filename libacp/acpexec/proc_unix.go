//go:build unix

package acpexec

import (
	"errors"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the spawned subprocess the leader of its own process
// group. Real-world agents are frequently wrappers (the ACP registry's npx/uvx
// distribution methods, shell shims) whose actual agent runs one or two forks
// down; without a group, killing only the direct child leaves that grandchild
// alive — and, because it inherited our stdout/stderr pipes, blocks the
// cmd.Wait reaper forever.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessTree kills the subprocess's entire process group (see
// setProcessGroup), falling back to killing just the direct child if group
// delivery fails.
func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

// exitFromKill reports whether waitErr records death by SIGKILL — the only
// exit Close's kill escalation can actually have caused. The distinction
// matters because "the kill branch ran" does not imply "the kill did it": on
// a loaded machine the Wait reaper can lag past the grace period for a
// process that already exited on its own, and its genuine exit status must
// still surface.
func exitFromKill(waitErr error) bool {
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL
}
