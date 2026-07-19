package libprocess

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// ErrNoStdio is returned by the Stdio methods when the supervised command was
// not configured with Config.PipeStdio, so there are no pipes to talk over.
var ErrNoStdio = errors.New("libprocess: process was not started with PipeStdio")

// Stdio is a running command's stdin/stdout pair presented as a single
// io.ReadWriteCloser: Read pulls from the command's stdout, Write pushes to
// its stdin. That is the shape a JSON-RPC peer over stdio expects (an ACP
// connection, an LSP client, an MCP stdio server), and the reason it exists
// here rather than in every caller: wiring Config.Stdout/Config.Stdin gives a
// supervisor a place to *dump* output, not a channel to *converse* over, so a
// supervised process could not host a protocol peer at all.
//
// A Stdio value belongs to exactly one spawn of the command. If the restart
// policy replaces the command, the old pipes are dead and a fresh Stdio takes
// their place — a transport cannot survive the death of the process on the
// other end of it. Callers that both restart and converse must therefore
// re-fetch Process.Stdio on every transition into Running (see WithStateHook)
// and rebuild their protocol session on top of it, exactly as a reconnecting
// client would.
//
// Closing the pipes is not this type's only closer: per exec.Cmd's contract
// cmd.Wait closes them when the command exits, and the supervisor's single
// Wait (in watch) is what makes reading from a dead command return io.EOF
// instead of blocking forever. Close here is therefore idempotent and
// tolerant of the pipes already being gone.
type Stdio struct {
	in  io.WriteCloser
	out io.ReadCloser

	inOnce  sync.Once
	outOnce sync.Once
	inErr   error
	outErr  error
}

var _ io.ReadWriteCloser = (*Stdio)(nil)

// Read reads from the command's stdout. Once the command exits — on its own,
// via Stop, or via context cancellation — Read returns io.EOF like any closed
// pipe, because the supervisor's Wait closes it.
func (s *Stdio) Read(b []byte) (int, error) {
	if s == nil {
		return 0, ErrNoStdio
	}
	return s.out.Read(b)
}

// Write writes to the command's stdin.
func (s *Stdio) Write(b []byte) (int, error) {
	if s == nil {
		return 0, ErrNoStdio
	}
	return s.in.Write(b)
}

// CloseStdin closes only the command's stdin, leaving stdout readable so any
// final output (a shutdown response, a farewell log line) can still be
// drained. It is the graceful-shutdown signal for well-behaved stdio peers —
// see CloseStdin, the GracefulStopFunc built on it.
func (s *Stdio) CloseStdin() error {
	if s == nil {
		return ErrNoStdio
	}
	s.inOnce.Do(func() { s.inErr = s.in.Close() })
	return s.inErr
}

// Close closes both pipes. It is idempotent, and safe to call after the
// command already exited (which closed them itself).
func (s *Stdio) Close() error {
	if s == nil {
		return ErrNoStdio
	}
	_ = s.CloseStdin()
	s.outOnce.Do(func() { s.outErr = s.out.Close() })
	return s.outErr
}

// Instance is the one running command a graceful-stop strategy is asked to
// shut down. It carries the pieces a strategy can legitimately act on — the
// OS process and, in PipeStdio mode, its transport — rather than the whole
// supervisor, so a strategy cannot reach back into lifecycle state it has no
// business changing.
type Instance struct {
	// Cmd is the started command. Cmd.Process is non-nil.
	Cmd *exec.Cmd
	// Stdio is the command's transport, or nil unless Config.PipeStdio is set.
	Stdio *Stdio
}

// GracefulStopFunc asks a running command to shut down cleanly. It must not
// block past ctx: Stop escalates to killing the process group once
// Config.StopGrace elapses regardless, so a strategy's job is only to *ask*.
// Returning an error is not fatal — Stop falls through to that same
// escalation — which is why a strategy may safely be best-effort on platforms
// where its mechanism does not exist.
type GracefulStopFunc func(ctx context.Context, inst Instance) error

// SignalGroup is the default GracefulStopFunc: it interrupts the command's
// whole process group (see setProcessGroup for why the group and not just the
// child). It is the right default for ordinary daemons and shell wrappers,
// which treat SIGINT as "wind down now".
func SignalGroup(_ context.Context, inst Instance) error {
	return signalGraceful(inst.Cmd)
}

// CloseStdin is the GracefulStopFunc for stdio protocol peers: it closes the
// command's stdin and nothing else. A signal is the wrong request to make of
// such a peer — a JSON-RPC agent driven over stdio has no reason to install a
// SIGINT handler, and many do not, so signalling means killing it mid-request
// — whereas EOF on stdin is the protocol-level statement "no more requests",
// which such peers answer by finishing what they are doing and exiting.
//
// It requires Config.PipeStdio; without it there is no stdin to close and it
// returns ErrNoStdio, which Stop treats like any other failed graceful
// attempt (fall through to the grace period, then kill).
func CloseStdin(_ context.Context, inst Instance) error {
	return inst.Stdio.CloseStdin()
}
