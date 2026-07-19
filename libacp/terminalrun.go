package libacp

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// terminalDetachedTimeout bounds the out-of-band calls (release, kill, output)
// that RunTerminal must make on a context that is deliberately NOT the caller's.
// Those calls happen precisely when the caller's context is already dead, so they
// need a budget of their own; 5s is enough for a local peer to answer and short
// enough that a wedged peer cannot pin the caller's goroutine.
const terminalDetachedTimeout = 5 * time.Second

// TerminalPeer is the subset of the ACP client side that RunTerminal drives.
// *AgentSideConnection satisfies it; tests and alternative transports can supply
// their own implementation.
type TerminalPeer interface {
	CreateTerminal(context.Context, CreateTerminalRequest) (CreateTerminalResponse, error)
	TerminalOutput(context.Context, TerminalOutputRequest) (TerminalOutputResponse, error)
	WaitForTerminalExit(context.Context, WaitForTerminalExitRequest) (WaitForTerminalExitResponse, error)
	KillTerminal(context.Context, KillTerminalRequest) (KillTerminalResponse, error)
	ReleaseTerminal(context.Context, ReleaseTerminalRequest) (ReleaseTerminalResponse, error)
}

// Compile-time assertion: the real agent-side connection is a TerminalPeer.
var _ TerminalPeer = (*AgentSideConnection)(nil)

// TerminalResult is the reconciled outcome of one command run over a peer terminal.
//
// Cancelled and TimedOut are kept distinct on purpose: a deadline means the
// command ran out of its time budget, while a plain cancellation means something
// (usually session/cancel, i.e. the user) stopped the turn. Collapsing the two
// into "timeout" hands the model and the user a wrong causal story about why the
// command died. In both cases the terminal was killed before the result was read.
type TerminalResult struct {
	Output    string
	Truncated bool
	ExitCode  int
	Signal    *string
	Cancelled bool // ctx was cancelled; the terminal was killed
	TimedOut  bool // ctx hit its deadline; the terminal was killed
}

// RunTerminal creates a terminal on the peer, waits for it to exit, collects its
// output and releases it, returning the reconciled result.
//
// onCreated, when non-nil, is invoked after the terminal exists but before the
// wait begins. It is the seam for callers that need to surface the live terminal
// (for example by attaching it to a tool call in a UI); RunTerminal itself stays
// free of presentation concerns.
//
// The caller's ctx governs only the create and the wait. Release, kill and the
// output fetch deliberately run on detached contexts: they are needed most when
// ctx is already dead, and losing the output of a command that has already been
// paid for is worse than a few extra seconds of work. The terminal is always
// released before returning.
//
// A non-nil error means the run could not be completed as a protocol exchange
// (create failed, the wait failed for a reason other than ctx, or the output
// could not be read). Even then the returned result carries the Cancelled and
// TimedOut flags that were established, so callers can still report why the
// command stopped. Policy decisions — treating Truncated as a budget error,
// formatting banners, mapping to exit statuses — belong to the caller.
func RunTerminal(ctx context.Context, p TerminalPeer, req CreateTerminalRequest, onCreated func(terminalID string)) (TerminalResult, error) {
	createResp, err := p.CreateTerminal(ctx, req)
	if err != nil {
		return TerminalResult{ExitCode: -1}, fmt.Errorf("libacp terminal: create: %w", err)
	}
	termID := createResp.TerminalID

	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalDetachedTimeout)
		defer cancel()
		_, _ = p.ReleaseTerminal(rctx, ReleaseTerminalRequest{SessionID: req.SessionID, TerminalID: termID})
	}()

	if onCreated != nil {
		onCreated(termID)
	}

	exitResp, waitErr := p.WaitForTerminalExit(ctx, WaitForTerminalExitRequest{SessionID: req.SessionID, TerminalID: termID})

	var res TerminalResult
	if waitErr != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			res.TimedOut = true
		case ctx.Err() != nil:
			res.Cancelled = true
		default:
			res.ExitCode = -1
			return res, fmt.Errorf("libacp terminal: wait: %w", waitErr)
		}
		// The wait ended early, so the process is still running. Kill it on a
		// detached context before reading what it managed to produce.
		kctx, kcancel := context.WithTimeout(context.WithoutCancel(ctx), terminalDetachedTimeout)
		_, _ = p.KillTerminal(kctx, KillTerminalRequest{SessionID: req.SessionID, TerminalID: termID})
		kcancel()
	}

	octx, ocancel := context.WithTimeout(context.WithoutCancel(ctx), terminalDetachedTimeout)
	outputResp, oerr := p.TerminalOutput(octx, TerminalOutputRequest{SessionID: req.SessionID, TerminalID: termID})
	ocancel()
	if oerr != nil {
		res.ExitCode = -1
		return res, fmt.Errorf("libacp terminal: output: %w", oerr)
	}

	res.Output = outputResp.Output
	res.Truncated = outputResp.Truncated
	res.Signal = exitResp.Signal

	// The exit code has two possible carriers: the wait response, and the exit
	// status attached to the output. Peers populate one or the other, so fall
	// back rather than assuming either is authoritative.
	switch {
	case exitResp.ExitCode != nil:
		res.ExitCode = *exitResp.ExitCode
	case outputResp.ExitStatus != nil && outputResp.ExitStatus.ExitCode != nil:
		res.ExitCode = *outputResp.ExitStatus.ExitCode
	}
	if res.Signal == nil && outputResp.ExitStatus != nil {
		res.Signal = outputResp.ExitStatus.Signal
	}
	// A signalled process did not exit cleanly even if no code was reported.
	if res.Signal != nil && res.ExitCode == 0 {
		res.ExitCode = -1
	}
	if res.Cancelled || res.TimedOut {
		res.ExitCode = -1
	}
	return res, nil
}
