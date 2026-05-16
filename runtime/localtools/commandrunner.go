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
	Stdin    string
}

type CommandRunner interface {
	Run(ctx context.Context, spec CommandSpec, stdout, stderr io.Writer) (exitCode int, err error)
}

func NewOSCommandRunner() CommandRunner {
	return osCommandRunner{}
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, spec CommandSpec, stdout, stderr io.Writer) (int, error) {
	var cmd *exec.Cmd
	if spec.UseShell {
		full := spec.Command
		if len(spec.Args) > 0 {
			full += " " + strings.Join(spec.Args, " ")
		}
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", full)
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
