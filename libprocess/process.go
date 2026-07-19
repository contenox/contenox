package libprocess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/contenox/runtime/libtracker"
)

// State is the operational state of a supervised Process.
type State int

const (
	// Stopped means no command is currently running and none will be
	// started automatically.
	Stopped State = iota
	// Starting means Start has been called and the command is being spawned.
	Starting
	// Running means the command was spawned successfully and is executing.
	Running
	// Crashed means the command exited and the restart policy gave up
	// (restart disabled, a clean exit with Restart.Always unset, or the
	// consecutive-restart Limit was reached).
	Crashed
)

// String returns a human-readable representation of the State.
func (s State) String() string {
	switch s {
	case Stopped:
		return "Stopped"
	case Starting:
		return "Starting"
	case Running:
		return "Running"
	case Crashed:
		return "Crashed"
	default:
		return "Unknown"
	}
}

// ErrAlreadyRunning is returned by Start when the process is already
// Starting or Running.
var ErrAlreadyRunning = errors.New("libprocess: process is already running")

// ErrNotRunning is returned by Stop when the process is not Starting or
// Running.
var ErrNotRunning = errors.New("libprocess: process is not running")

// RestartPolicy controls whether and how a Process restarts after its
// command exits on its own (as opposed to being stopped via Stop).
type RestartPolicy struct {
	// Enabled restarts the process automatically after it exits.
	Enabled bool
	// Always restarts even after a clean exit (code 0). Without it, a clean
	// exit is treated as intentional completion and is never restarted —
	// only a nonzero exit triggers a restart.
	Always bool
	// Limit caps consecutive restart attempts following a nonzero exit
	// before the process is left Stopped→Crashed instead of restarting
	// again. Zero means unlimited. Ignored when Always is set, since an
	// Always restart loop is expected to run indefinitely.
	Limit int
	// Delay is waited before each automatic restart. Zero restarts
	// immediately.
	Delay time.Duration
}

// Config describes how to start and supervise a process.
type Config struct {
	// Command is the executable to run, resolved via exec.LookPath rules.
	Command string
	Args    []string
	// Dir is the working directory. Empty uses the caller's current directory.
	Dir string
	// Env holds extra "KEY=VALUE" entries appended to os.Environ() for the
	// spawned process.
	Env []string

	Stdout io.Writer // defaults to io.Discard
	Stderr io.Writer // defaults to io.Discard
	Stdin  io.Reader // defaults to nil (no stdin)

	Restart RestartPolicy

	// StopGrace bounds how long Stop waits after signaling the process
	// before force-killing it. Defaults to 5s.
	StopGrace time.Duration
}

// StateChange is delivered to a state-change hook (see WithStateHook) every
// time a Process transitions between states.
type StateChange struct {
	From, To State
	// Err is the error that caused an exit-driven transition, if any (e.g.
	// the *exec.ExitError from a nonzero exit, or the spawn error that took
	// the process straight to Crashed).
	Err error
	// ExitCode is the exited command's exit code, valid when To follows an
	// exit (Running -> Stopped, Running -> Crashed). -1 if not applicable.
	ExitCode int
	// Restarts is the consecutive-restart counter at the time of this
	// transition.
	Restarts int
}

// Option configures a Process at construction.
type Option func(*Process)

// WithStateHook registers fn to be called synchronously on every state
// transition. fn must not block or call back into the Process (Start/Stop)
// from the same goroutine that delivered the transition, since transitions
// are delivered while the Process's internal lock is not held but from the
// single supervising goroutine — a blocking hook stalls the watchdog for
// this process only.
//
// fn receives the supervision context so it can propagate cancellation and
// deadlines into whatever it does with the transition, and returns an error
// so a failed hook is observable: the error is reported to the tracker rather
// than swallowed. Returning an error never changes the supervised process's
// own outcome — a broken observer must not be able to fail a healthy process.
//
// This is a lighter-weight alternative to WithTracker for callers that just
// want the typed StateChange value on every transition; use WithTracker
// instead to integrate with this codebase's standard instrumentation seam
// (metrics, logging, tracing, audit trails).
func WithStateHook(fn func(context.Context, StateChange) error) Option {
	return func(p *Process) { p.hook = fn }
}

// WithErrorHandler registers fn to receive errors that arise inside the
// supervisor's own goroutines, where there is no caller to return them to: a
// respawn that fails partway through a restart policy, a supervision lock
// that cannot be renewed or released, a state hook that fails.
//
// Supervising another process's lifecycle generates exactly this class of
// error — asynchronous, after the Start call has already returned — and
// without a handler the only options are to swallow them or to depend on a
// tracker being wired. fn is called synchronously from the supervising
// goroutine and must not block or call back into the Process.
//
// fn never changes the supervised process's outcome; it is a reporting seam,
// not a policy one.
func WithErrorHandler(fn func(context.Context, error)) Option {
	return func(p *Process) { p.onError = fn }
}

// WithTracker wires an ActivityTracker to observe the supervised lifetime of
// one Start call: Start is called on tracker when Start begins, reportErr
// fires if the process ends up Crashed, reportChange fires with the
// command and final StateChange on any other terminal outcome, and end
// fires once the process reaches its terminal state (see Done) — covering
// every restart along the way as a single tracked operation, since a
// restarting process is one supervised run, not a new one per attempt.
// Without WithTracker, a Process uses libtracker.NoopTracker.
func WithTracker(tracker libtracker.ActivityTracker) Option {
	return func(p *Process) { p.tracker = tracker }
}

// Process supervises one instance of Config.Command. It is safe for
// concurrent use.
type Process struct {
	cfg Config

	mu       sync.Mutex
	state    State
	cmd      *exec.Cmd
	restarts int
	stopping bool          // Stop was called; suppresses auto-restart for the in-flight exit
	done     chan struct{} // closed when the process reaches a terminal state (Stopped or Crashed) with no restart pending
	stop     chan struct{} // closed by Stop; interrupts a pending restart delay

	hook    func(context.Context, StateChange) error
	onError func(context.Context, error)
	tracker libtracker.ActivityTracker

	// lock, when set, gates supervision (see WithLock). lostErr records a
	// lock loss so finish can report Crashed with the cause instead of the
	// clean Stopped that the induced shutdown would otherwise look like.
	lock       Lock
	renewEvery time.Duration
	lostErr    error

	// trackErr/trackChange/trackEnd are the closures returned by
	// tracker.Start for the current Start call's supervised lifetime; set
	// in Start, consumed exactly once by finish.
	trackErr    func(error)
	trackChange func(string, any)
	trackEnd    func()
}

// New returns a Process configured by cfg. The process is not started.
//
// It returns an error rather than panicking or deferring to Start so that
// misconfiguration is caught at construction, and so this package can add
// validation later without changing the signature.
func New(cfg Config, opts ...Option) (*Process, error) {
	if cfg.Command == "" {
		return nil, errors.New("libprocess: Config.Command is required")
	}
	if cfg.StopGrace <= 0 {
		cfg.StopGrace = 5 * time.Second
	}
	p := &Process{
		cfg:     cfg,
		state:   Stopped,
		done:    make(chan struct{}),
		tracker: libtracker.NoopTracker{},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.renewEvery > 0 && p.lock == nil {
		return nil, errors.New("libprocess: WithLock renewal interval set without a Lock")
	}
	if p.lock != nil && p.renewEvery <= 0 && p.cfg.Restart.Enabled {
		// A restarting supervisor holds the claim across many command
		// lifetimes; without renewal a TTL-based lock silently lapses and a
		// second supervisor starts a duplicate — the exact failure the lock
		// is there to prevent.
		return nil, errors.New("libprocess: WithLock requires a positive renewal interval when restarts are enabled")
	}
	return p, nil
}

// Start spawns the command and returns once it has been launched
// successfully. A background goroutine then supervises it: waiting for exit
// and applying the restart policy. Start returns ErrAlreadyRunning if the
// process is already Starting or Running.
//
// When a Lock is configured (see WithLock) Start claims it before spawning
// and returns the acquisition error — leaving the Process Stopped and no
// command running — if another supervisor holds it.
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.state == Starting || p.state == Running {
		p.mu.Unlock()
		return ErrAlreadyRunning
	}
	lock := p.lock
	p.mu.Unlock()

	// Acquire before touching any lifetime state so a failed claim is a clean
	// no-op: no tracked operation is opened and no terminal transition is
	// owed to a caller whose Start never took effect.
	if lock != nil {
		if err := lock.Acquire(ctx); err != nil {
			return err
		}
	}

	p.mu.Lock()
	p.stopping = false
	p.restarts = 0
	p.lostErr = nil
	p.done = make(chan struct{})
	p.stop = make(chan struct{})
	done, stop := p.done, p.stop
	p.trackErr, p.trackChange, p.trackEnd = p.tracker.Start(ctx, "supervise", "process", "command", p.cfg.Command)
	p.mu.Unlock()

	if lock != nil && p.renewEvery > 0 {
		go p.renew(ctx, lock, done, stop)
	}

	return p.spawn(ctx)
}

// renew keeps the supervision claim alive for one supervised lifetime and
// terminates the command if the claim is lost, so a supervisor that can no
// longer prove ownership stops competing with whoever takes over.
func (p *Process) renew(ctx context.Context, lock Lock, done, stop <-chan struct{}) {
	ticker := time.NewTicker(p.renewEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := lock.Renew(ctx); err != nil {
				p.loseLock(ctx, fmt.Errorf("%w: %w", ErrLockLost, err))
				return
			}
		}
	}
}

// loseLock tears down the running command after a lost claim, recording cause
// so the terminal transition reports Crashed rather than a clean Stopped.
func (p *Process) loseLock(ctx context.Context, cause error) {
	p.mu.Lock()
	p.lostErr = cause
	p.mu.Unlock()
	p.reportErr(ctx, cause)
	// Fencing must happen even if the supervision context is already dead:
	// the whole point is that this supervisor stops competing with whoever
	// took the lock over.
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.cfg.StopGrace)
	defer cancel()
	// Stop reports ErrNotRunning if the command already reached a terminal
	// state on its own; that is a benign race, not a failure to fence.
	_ = p.Stop(stopCtx)
}

// spawn launches the command once and, on success, starts the watchdog
// goroutine that owns the process for the rest of its lifetime.
func (p *Process) spawn(ctx context.Context) error {
	p.setState(ctx, Starting, nil, -1)

	cmd := exec.Command(p.cfg.Command, p.cfg.Args...)
	cmd.Dir = p.cfg.Dir
	if len(p.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), p.cfg.Env...)
	}
	cmd.Stdout = p.cfg.Stdout
	cmd.Stderr = p.cfg.Stderr
	cmd.Stdin = p.cfg.Stdin
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		p.mu.Lock()
		p.cmd = nil
		restarts := p.restarts
		p.mu.Unlock()
		p.finish(ctx, Crashed, fmt.Errorf("libprocess: start %q: %w", p.cfg.Command, err), -1, restarts)
		return err
	}

	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()
	p.setState(ctx, Running, nil, -1)

	go p.watch(ctx)
	return nil
}

// watch waits for the running command to exit and applies the restart
// policy. It is the sole writer of p.cmd/p.restarts/p.done for the lifetime
// of one spawn, so it does not need to hold p.mu across the wait.
func (p *Process) watch(ctx context.Context) {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()

	err := cmd.Wait()
	exitCode := -1
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	case err == nil:
		exitCode = 0
	}

	p.mu.Lock()
	stopping := p.stopping
	p.cmd = nil
	p.mu.Unlock()

	// Crashed is reserved for "the restart policy gave up" (limit reached);
	// a nonzero exit with no restart policy configured is still just
	// Stopped, with ExitCode carrying the detail.
	restart := !stopping && p.cfg.Restart.Enabled && (p.cfg.Restart.Always || exitCode != 0)

	if restart && !p.cfg.Restart.Always && p.cfg.Restart.Limit > 0 {
		p.mu.Lock()
		limitReached := p.restarts >= p.cfg.Restart.Limit
		if !limitReached {
			p.restarts++
		}
		restarts := p.restarts
		p.mu.Unlock()
		if limitReached {
			p.finish(ctx, Crashed, fmt.Errorf("libprocess: restart limit (%d) reached", p.cfg.Restart.Limit), exitCode, restarts)
			return
		}
	} else if restart {
		p.mu.Lock()
		p.restarts++
		p.mu.Unlock()
	}

	if !restart {
		p.finish(ctx, Stopped, err, exitCode, p.Restarts())
		return
	}

	if p.cfg.Restart.Delay > 0 {
		p.mu.Lock()
		stopCh := p.stop
		p.mu.Unlock()
		// The command is dead but a replacement is owed, so report Starting
		// for the whole restart window rather than leaving a stale Running.
		p.setState(ctx, Starting, nil, exitCode)

		select {
		case <-time.After(p.cfg.Restart.Delay):
		case <-stopCh:
			// Stop arrived while no command was running: it is blocked on
			// done, and only this goroutine can close it.
			p.finish(ctx, Stopped, err, exitCode, p.Restarts())
			return
		case <-ctx.Done():
			p.finish(ctx, Stopped, ctx.Err(), exitCode, p.Restarts())
			return
		}
	}

	// Stop may also have arrived during the delay-free path or in the window
	// between the timer firing and the respawn; re-check before spending a
	// process that nobody would be able to reap.
	p.mu.Lock()
	stopping = p.stopping
	p.mu.Unlock()
	if stopping {
		p.finish(ctx, Stopped, err, exitCode, p.Restarts())
		return
	}

	// spawn transitions to Crashed and finishes on its own if it fails to
	// launch; on success it starts a fresh watch goroutine for the new
	// instance, so this goroutine's job ends either way. The error is
	// reported rather than returned because there is no caller left to
	// return it to — Start returned long ago.
	if serr := p.spawn(ctx); serr != nil {
		p.reportErr(ctx, fmt.Errorf("libprocess: restart %q: %w", p.cfg.Command, serr))
	}
}

// finish publishes the terminal transition, releases the supervision lock,
// closes p.done, and ends this Start call's tracked operation (see
// WithTracker). It must only be called from watch or spawn, which together
// are the sole owners of a Start call's lifecycle, and exactly once per Start.
//
// Ordering is a contract, not an accident: everything a caller is entitled to
// observe once Done fires — the terminal State, a released lock — must be in
// place before done closes. Closing first lets `<-p.Done(); p.State()` read a
// stale Running, and lets a caller re-acquire the lock before this supervisor
// has actually let go of it.
func (p *Process) finish(ctx context.Context, to State, err error, exitCode, restarts int) {
	p.mu.Lock()
	trackErr, trackChange, trackEnd := p.trackErr, p.trackChange, p.trackEnd
	lock, lostErr := p.lock, p.lostErr
	done := p.done
	p.mu.Unlock()

	// A lock loss induces the shutdown that lands here, so the raw outcome
	// looks like a clean stop. Report the real cause instead.
	if lostErr != nil {
		to, err = Crashed, lostErr
	}

	if lock != nil {
		if rerr := lock.Release(context.WithoutCancel(ctx)); rerr != nil {
			rerr = fmt.Errorf("libprocess: release supervision lock: %w", rerr)
			p.reportErr(ctx, rerr)
			if err == nil {
				err = rerr
			}
		}
	}

	p.setStateWithRestarts(ctx, to, err, exitCode, restarts)
	close(done)

	if to == Crashed {
		trackErr(err)
	} else {
		trackChange(p.cfg.Command, map[string]any{
			"state": to.String(), "exitCode": exitCode, "restarts": restarts,
		})
	}
	trackEnd()
}

// Stop signals the running process to exit gracefully, waiting up to
// Config.StopGrace before force-killing it, and suppresses any pending
// auto-restart. It blocks until the process has fully exited. Stop returns
// ErrNotRunning if the process is not Starting or Running. It is safe to
// call Stop more than once concurrently; only the first has effect.
//
// ctx bounds only the graceful wait: cancelling it escalates straight to
// killing the process group instead of abandoning the shutdown, so Stop
// always leaves the process actually stopped rather than returning while an
// orphan keeps running. It still blocks until the process has exited.
func (p *Process) Stop(ctx context.Context) error {
	p.mu.Lock()
	if p.state != Starting && p.state != Running {
		p.mu.Unlock()
		return ErrNotRunning
	}
	first := !p.stopping
	p.stopping = true
	cmd := p.cmd
	done := p.done
	stopCh := p.stop
	if first && stopCh != nil {
		// Wakes a watch goroutine parked in a restart delay, which would
		// otherwise spawn a replacement nobody reaps and leave Stop blocked
		// on done forever.
		close(stopCh)
	}
	p.mu.Unlock()

	if cmd == nil {
		// Either between Starting and the spawn goroutine publishing p.cmd,
		// or inside a restart delay. Both end by closing done.
		<-done
		return nil
	}

	if err := signalGraceful(cmd); err != nil {
		// Signal not supported (e.g. some platforms) or process already
		// gone: fall through to the grace-timeout kill path, which handles
		// both.
		_ = err
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// The caller is out of patience before StopGrace elapsed: stop
		// waiting politely and escalate immediately.
	case <-time.After(p.cfg.StopGrace):
	}

	// Kill the whole group: a wrapper that ignored the graceful signal would
	// otherwise leave its real workload running and holding our pipes, which
	// blocks the Wait reaper and so blocks done forever.
	killProcessTree(cmd)
	<-done
	return nil
}

// State returns the current operational state.
func (p *Process) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Restarts returns the number of automatic restarts performed since the
// last call to Start. Start resets it to zero, so the restart Limit applies
// per supervised lifetime rather than for the life of the Process value.
func (p *Process) Restarts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restarts
}

// Pid returns the current process ID, or 0 if not running.
func (p *Process) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Done returns a channel that is closed when the process reaches a terminal
// state (Stopped or Crashed) with no restart pending. A new channel is
// created on each Start, so callers that need to observe multiple lifetimes
// should re-fetch Done after each Start.
func (p *Process) Done() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

// reportErr surfaces an error raised inside the supervisor's own goroutines
// to the handler registered by WithErrorHandler. It is the single place such
// errors go, so none of them are silently dropped.
func (p *Process) reportErr(ctx context.Context, err error) {
	if err == nil {
		return
	}
	p.mu.Lock()
	onError := p.onError
	p.mu.Unlock()
	if onError != nil {
		onError(ctx, err)
	}
}

func (p *Process) setState(ctx context.Context, to State, err error, exitCode int) {
	p.mu.Lock()
	restarts := p.restarts
	p.mu.Unlock()
	p.setStateWithRestarts(ctx, to, err, exitCode, restarts)
}

func (p *Process) setStateWithRestarts(ctx context.Context, to State, err error, exitCode, restarts int) {
	p.mu.Lock()
	from := p.state
	p.state = to
	hook := p.hook
	p.mu.Unlock()
	if hook == nil {
		return
	}
	// A hook is an observer: its failure is reported but never propagated
	// into the supervised process's own outcome.
	if herr := hook(ctx, StateChange{From: from, To: to, Err: err, ExitCode: exitCode, Restarts: restarts}); herr != nil {
		p.reportErr(ctx, fmt.Errorf("libprocess: state hook for %s: %w", to, herr))
	}
}
