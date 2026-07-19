//go:build unix

package libprocess

import (
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
