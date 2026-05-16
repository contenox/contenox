package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	libacp "github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/localtools"
)

var acpTerminalOutputByteLimit int64 = 1 * 1024 * 1024

type acpCommandRunner struct {
	transport func() *Transport
}

func NewACPCommandRunner(transport func() *Transport) localtools.CommandRunner {
	return &acpCommandRunner{transport: transport}
}

func (a *acpCommandRunner) Run(ctx context.Context, spec localtools.CommandSpec, stdout, stderr io.Writer) (int, error) {
	t := a.transport()
	if t == nil {
		return -1, errors.New("acpsvc: no active ACP transport")
	}
	if !t.getClientCaps().Terminal {
		return localtools.NewOSCommandRunner().Run(ctx, spec, stdout, stderr)
	}

	command := spec.Command
	cmdArgs := spec.Args
	if spec.UseShell {
		full := spec.Command
		if len(spec.Args) > 0 {
			full += " " + strings.Join(spec.Args, " ")
		}
		command = "/bin/sh"
		cmdArgs = []string{"-c", full}
	}

	req := libacp.CreateTerminalRequest{
		Command:         command,
		Args:            cmdArgs,
		OutputByteLimit: &acpTerminalOutputByteLimit,
	}
	sid := resolveACPSessionID(ctx, t)
	if sid != "" {
		req.SessionID = sid
	}
	if spec.Cwd != "" {
		req.Cwd = spec.Cwd
	} else if sid != "" {
		internal := sessionIDFromCtx(ctx)
		t.sessionMu.Lock()
		for _, entry := range t.sessions {
			if entry.InternalSessionID == internal && entry.Cwd != "" {
				req.Cwd = entry.Cwd
				break
			}
		}
		t.sessionMu.Unlock()
	}

	createResp, err := t.conn.CreateTerminal(ctx, req)
	if err != nil {
		return -1, fmt.Errorf("acpsvc terminal: create: %w", err)
	}
	termID := createResp.TerminalID
	defer func() {
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = t.conn.ReleaseTerminal(rctx, libacp.ReleaseTerminalRequest{SessionID: req.SessionID, TerminalID: termID})
	}()

	if sid != "" {
		if tcID := toolCallIDFromCtx(ctx); tcID != "" {
			t.sendUpdate(ctx, terminalAttachNotification(sid, tcID, termID))
		}
	}

	exitResp, waitErr := t.conn.WaitForTerminalExit(ctx, libacp.WaitForTerminalExitRequest{SessionID: req.SessionID, TerminalID: termID})

	timedOut := false
	if waitErr != nil {
		if ctx.Err() != nil {
			timedOut = true
			kctx, kcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer kcancel()
			_, _ = t.conn.KillTerminal(kctx, libacp.KillTerminalRequest{SessionID: req.SessionID, TerminalID: termID})
		} else {
			return -1, fmt.Errorf("acpsvc terminal: wait: %w", waitErr)
		}
	}

	octx, ocancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ocancel()
	outputResp, oerr := t.conn.TerminalOutput(octx, libacp.TerminalOutputRequest{SessionID: req.SessionID, TerminalID: termID})
	if oerr != nil {
		if timedOut {
			return -1, fmt.Errorf("acpsvc terminal: command timed out")
		}
		return -1, fmt.Errorf("acpsvc terminal: output: %w", oerr)
	}

	if outputResp.Truncated {
		return -1, localtools.ErrOutputBudgetExceeded
	}

	if outputResp.Output != "" {
		_, _ = io.WriteString(stdout, outputResp.Output)
	}

	if timedOut {
		_, _ = io.WriteString(stdout, "\n[command killed: timeout exceeded]")
		return -1, fmt.Errorf("acpsvc terminal: command timed out")
	}

	exitCode := 0
	switch {
	case exitResp.ExitCode != nil:
		exitCode = *exitResp.ExitCode
	case outputResp.ExitStatus != nil && outputResp.ExitStatus.ExitCode != nil:
		exitCode = *outputResp.ExitStatus.ExitCode
	}
	if exitResp.Signal != nil {
		_, _ = io.WriteString(stdout, fmt.Sprintf("\n[terminated by signal %s]", *exitResp.Signal))
		if exitCode == 0 {
			exitCode = -1
		}
	}
	return exitCode, nil
}
