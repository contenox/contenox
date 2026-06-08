package localtools

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

var ErrOutputBudgetExceeded = errors.New("local_shell: command output exceeded the remaining context budget")

type CommandSpec struct {
	Command  string
	Args     []string
	Cwd      string
	Timeout  time.Duration
	UseShell bool
	Shell    PlatformShell
	Stdin    string
}

type CommandRunner interface {
	Run(ctx context.Context, spec CommandSpec, stdout, stderr io.Writer) (exitCode int, err error)
}

func NewOSCommandRunner() CommandRunner {
	return NewOSCommandRunnerWithShell(DetectPlatformShell())
}

func NewOSCommandRunnerWithShell(shell PlatformShell) CommandRunner {
	return osCommandRunner{shell: shell.WithDefaults()}
}

type osCommandRunner struct {
	shell PlatformShell
}

func (r osCommandRunner) Run(ctx context.Context, spec CommandSpec, stdout, stderr io.Writer) (int, error) {
	var cmd *exec.Cmd
	if spec.UseShell {
		shell := spec.Shell
		if !shell.IsSet() {
			shell = r.shell
		}
		program, args, _ := shell.WrapCommand(spec.Command, spec.Args)
		cmd = exec.CommandContext(ctx, program, args...)
	} else {
		cmd = exec.CommandContext(ctx, spec.Command, spec.Args...)
	}
	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	return 0, nil
}
