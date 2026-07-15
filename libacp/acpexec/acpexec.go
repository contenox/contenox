// Package acpexec spawns a subprocess and wires its stdin/stdout together as
// a single io.ReadWriteCloser, the transport shape libacp.NewAgentSideConnection
// and libacp.NewClientSideConnection both expect. It exists so an ACP peer
// (an editor driving a real agent binary, or a test driving the Rust
// reference agent/client binaries) can be reached over stdio without any
// caller having to hand-roll pipe plumbing and shutdown bookkeeping.
package acpexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// defaultKillGrace is how long Close waits, after closing the subprocess's
// stdin, for it to exit on its own before escalating to Process.Kill.
const defaultKillGrace = 5 * time.Second

// Option configures Spawn. See WithStderr and WithKillGrace.
type Option func(*config)

type config struct {
	stderr    io.Writer
	killGrace time.Duration
}

// WithStderr forwards the subprocess's stderr to w as it's written, instead
// of the default (io.Discard). Passing a *LockedBuffer lets a caller recover
// the subprocess's stderr for a failure message even though it is written
// from a different goroutine than the one that later reads it.
func WithStderr(w io.Writer) Option {
	return func(c *config) { c.stderr = w }
}

// WithKillGrace overrides how long Close waits for the subprocess to exit on
// its own (after closing its stdin) before it escalates to Process.Kill.
// The default is 5 seconds.
func WithKillGrace(d time.Duration) Option {
	return func(c *config) { c.killGrace = d }
}

// Process is a spawned subprocess wired up as an io.ReadWriteCloser: Read
// pulls from its stdout, Write pushes to its stdin, and Close begins its
// shutdown sequence (see Close). It is the concrete type Spawn returns,
// rather than a bare io.ReadWriteCloser, so callers that need it (tests,
// mainly) can still reach Wait's exit error without a type assertion.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	grace  time.Duration

	waitDone chan struct{}
	waitErr  error

	closeOnce sync.Once
	closeErr  error
}

var _ io.ReadWriteCloser = (*Process)(nil)

// Spawn starts cmd and returns it as a Process. cmd's Stdin/Stdout are
// claimed via exec.Cmd.StdinPipe/StdoutPipe — callers must not have set them
// already. Stderr is discarded unless WithStderr overrides it.
//
// If ctx is cancelled before the subprocess exits on its own, Spawn closes
// it down exactly as Close would (grace period, then kill) rather than
// leaking a running process past the caller's context.
//
// Spawn's own error (a pipe-setup or Start failure) always means no process
// was left running; the caller has nothing to clean up in that case.
func Spawn(ctx context.Context, cmd *exec.Cmd, opts ...Option) (*Process, error) {
	cfg := config{stderr: io.Discard, killGrace: defaultKillGrace}
	for _, opt := range opts {
		opt(&cfg)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acpexec: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acpexec: stdout pipe: %w", err)
	}
	cmd.Stderr = cfg.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acpexec: start %s: %w", cmd.Path, err)
	}

	p := &Process{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		grace:    cfg.killGrace,
		waitDone: make(chan struct{}),
	}

	// Reap the process as soon as it exits, whoever causes that (it exiting
	// on its own, or Close/ctx cancellation killing it). This goroutine, not
	// Close, owns the only call to cmd.Wait: per exec.Cmd's contract, Wait
	// closes the pipes once the process exits, so calling it is only safe
	// once nothing further will read from them without also observing that
	// close — a single, always-running Wait satisfies that for every caller
	// (Read draining stdout, Close waiting on waitDone) at once.
	go func() {
		p.waitErr = cmd.Wait()
		close(p.waitDone)
	}()

	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = p.Close()
			case <-p.waitDone:
			}
		}()
	}

	return p, nil
}

// Read reads from the subprocess's stdout. Once the subprocess exits (on its
// own, or via Close/ctx cancellation), Read returns io.EOF like any closed
// pipe.
func (p *Process) Read(b []byte) (int, error) { return p.stdout.Read(b) }

// Write writes to the subprocess's stdin.
func (p *Process) Write(b []byte) (int, error) { return p.stdin.Write(b) }

// Close begins graceful shutdown: it closes the subprocess's stdin (many
// well-behaved stdio agents/clients treat that as "no more requests" and
// exit on their own), waits up to the configured grace period (default 5s,
// see WithKillGrace) for it to do so, and kills it if it hasn't. Close
// always waits for the process to actually be reaped before returning.
//
// Close is idempotent: it runs this sequence exactly once (via sync.Once)
// and every call, including ones after the process already exited on its
// own, returns the same result — the subprocess's exit error, or nil for a
// clean exit.
func (p *Process) Close() error {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()

		select {
		case <-p.waitDone:
		case <-time.After(p.grace):
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			<-p.waitDone
		}

		_ = p.stdout.Close()
		p.closeErr = p.waitErr
	})
	return p.closeErr
}

// LockedBuffer is a concurrency-safe io.Writer around a bytes.Buffer, meant
// for WithStderr: a subprocess writes to it from its own reader goroutine
// (installed by Spawn) while the caller reads String() later, typically only
// once something has already gone wrong and the process's stderr is wanted
// for a failure message.
type LockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *LockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything written so far.
func (b *LockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
