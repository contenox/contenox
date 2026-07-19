package libprocess_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libprocess"
	"github.com/contenox/runtime/libtracker"
	"github.com/stretchr/testify/require"
)

// fakeTracker records ActivityTracker calls for assertion.
type fakeTracker struct {
	mu       sync.Mutex
	starts   []string
	errs     []error
	changes  []string
	endCalls int
}

func (f *fakeTracker) Start(ctx context.Context, operation, subject string, kvArgs ...any) (func(error), func(string, any), func()) {
	f.mu.Lock()
	f.starts = append(f.starts, operation+":"+subject)
	f.mu.Unlock()
	return func(err error) {
			f.mu.Lock()
			f.errs = append(f.errs, err)
			f.mu.Unlock()
		}, func(id string, data any) {
			f.mu.Lock()
			f.changes = append(f.changes, id)
			f.mu.Unlock()
		}, func() {
			f.mu.Lock()
			f.endCalls++
			f.mu.Unlock()
		}
}

var _ libtracker.ActivityTracker = (*fakeTracker)(nil)

func shArgs(script string) (string, []string) {
	return "/bin/sh", []string{"-c", script}
}

func TestUnit_Process_StartRunsToCompletion(t *testing.T) {
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args})

	require.NoError(t, p.Start(t.Context()))

	select {
	case <-p.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("process did not finish in time")
	}
	require.Equal(t, libprocess.Stopped, p.State())
}

func TestUnit_Process_StartTwiceFails(t *testing.T) {
	cmd, args := shArgs("sleep 1")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args})

	require.NoError(t, p.Start(t.Context()))
	defer p.Stop(t.Context())

	err := p.Start(t.Context())
	require.ErrorIs(t, err, libprocess.ErrAlreadyRunning)
}

func TestUnit_Process_StopBeforeStartedFails(t *testing.T) {
	cmd, args := shArgs("sleep 1")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args})

	err := p.Stop(t.Context())
	require.ErrorIs(t, err, libprocess.ErrNotRunning)
}

func TestUnit_Process_StopGracefullySignalsThenReportsStopped(t *testing.T) {
	// Traps SIGINT and exits cleanly, so Stop's signal (not the grace-period
	// kill) is what ends the process.
	cmd, args := shArgs(`trap 'exit 0' INT; while true; do sleep 0.05; done`)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, StopGrace: 2 * time.Second})

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // let the shell install its trap before we signal it

	require.NoError(t, p.Stop(t.Context()))
	require.Equal(t, libprocess.Stopped, p.State())
}

func TestUnit_Process_StopKillsAfterGraceExpires(t *testing.T) {
	// Ignores SIGINT entirely, forcing Stop to fall back to Kill after
	// StopGrace.
	cmd, args := shArgs(`trap '' INT; while true; do sleep 0.05; done`)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, StopGrace: 150 * time.Millisecond})

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // let the shell install its trap before we signal it

	start := time.Now()
	require.NoError(t, p.Stop(t.Context()))
	elapsed := time.Since(start)

	require.GreaterOrEqual(t, elapsed, 150*time.Millisecond)
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, libprocess.Stopped, p.State())
}

func TestUnit_Process_CleanExitIsNotRestartedByDefault(t *testing.T) {
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{Enabled: true},
	})

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("process did not finish in time")
	}
	require.Equal(t, libprocess.Stopped, p.State())
	require.Equal(t, 0, p.Restarts())
}

func TestUnit_Process_NonzeroExitRestartsUntilLimitThenCrashes(t *testing.T) {
	var transitions []libprocess.State
	var mu sync.Mutex
	hook := libprocess.WithStateHook(func(_ context.Context, sc libprocess.StateChange) error {
		mu.Lock()
		transitions = append(transitions, sc.To)
		mu.Unlock()
		return nil
	})

	cmd, args := shArgs("exit 1")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{Enabled: true, Limit: 2},
	}, hook)

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not reach a terminal state in time")
	}

	require.Equal(t, libprocess.Crashed, p.State())
	require.Equal(t, 2, p.Restarts())

	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, transitions, libprocess.Crashed)
}

func TestUnit_Process_AlwaysRestartsOnCleanExit(t *testing.T) {
	var starts int32
	var mu sync.Mutex
	hook := libprocess.WithStateHook(func(_ context.Context, sc libprocess.StateChange) error {
		if sc.To == libprocess.Running {
			mu.Lock()
			starts++
			mu.Unlock()
		}
		return nil
	})

	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{Enabled: true, Always: true},
	}, hook)

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return starts >= 3
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, p.Stop(t.Context()))
}

func TestUnit_Process_SpawnFailureReportsCrashed(t *testing.T) {
	p := newProc(t, libprocess.Config{Command: "libprocess-definitely-not-a-real-binary"})

	err := p.Start(t.Context())
	require.Error(t, err)
	require.Equal(t, libprocess.Crashed, p.State())
}

func TestUnit_Process_PidReflectsRunningProcess(t *testing.T) {
	cmd, args := shArgs("sleep 1")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args})

	require.Zero(t, p.Pid())
	require.NoError(t, p.Start(t.Context()))
	defer p.Stop(t.Context())

	require.Eventually(t, func() bool { return p.Pid() != 0 }, time.Second, 10*time.Millisecond)
}

func TestUnit_Process_EnvIsPassedToChild(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "libprocess-env-*")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cmd, args := shArgs(`echo -n "$LIBPROCESS_TEST_VAR" > "` + f.Name() + `"`)
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Env:     []string{"LIBPROCESS_TEST_VAR=hello"},
	})

	require.NoError(t, p.Start(context.Background()))
	select {
	case <-p.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("process did not finish in time")
	}

	got, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
}

func TestUnit_Process_TrackerObservesCleanExit(t *testing.T) {
	tracker := &fakeTracker{}
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args}, libprocess.WithTracker(tracker))

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("process did not finish in time")
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Equal(t, []string{"supervise:process"}, tracker.starts)
	require.Equal(t, []string{cmd}, tracker.changes)
	require.Empty(t, tracker.errs)
	require.Equal(t, 1, tracker.endCalls)
}

func TestUnit_Process_TrackerObservesCrash(t *testing.T) {
	tracker := &fakeTracker{}
	p := newProc(t, libprocess.Config{
		Command: "libprocess-definitely-not-a-real-binary",
	}, libprocess.WithTracker(tracker))

	err := p.Start(t.Context())
	require.Error(t, err)
	require.Equal(t, libprocess.Crashed, p.State())

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Equal(t, []string{"supervise:process"}, tracker.starts)
	require.Len(t, tracker.errs, 1)
	require.Error(t, tracker.errs[0])
	require.Empty(t, tracker.changes)
	require.Equal(t, 1, tracker.endCalls)
}

func TestUnit_Process_TrackerCoversFullRestartLifetimeAsOneOperation(t *testing.T) {
	tracker := &fakeTracker{}
	cmd, args := shArgs("exit 1")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{Enabled: true, Limit: 2},
	}, libprocess.WithTracker(tracker))

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not reach a terminal state in time")
	}
	require.Equal(t, libprocess.Crashed, p.State())

	// One Start call, restarting twice, is one tracked operation — not three.
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Equal(t, []string{"supervise:process"}, tracker.starts)
	require.Equal(t, 1, tracker.endCalls)
	require.Len(t, tracker.errs, 1)
}

// A Stop issued while the supervisor is parked in Restart.Delay must return
// and must not spend a replacement process that nobody would reap.
func TestUnit_Process_StopDuringRestartDelayDoesNotLeakReplacement(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	// First run exits nonzero to trigger a restart; any later run would block
	// for 30s, so a leaked replacement is unmistakable.
	cmd, args := shArgs("if [ -f " + marker + " ]; then sleep 30; else touch " + marker + "; exit 1; fi")
	p := newProc(t, libprocess.Config{
		Command:   cmd,
		Args:      args,
		Restart:   libprocess.RestartPolicy{Enabled: true, Delay: 500 * time.Millisecond},
		StopGrace: 200 * time.Millisecond,
	})

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() == 0 }, 2*time.Second, 10*time.Millisecond,
		"first instance never exited into the restart delay")

	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(t.Context()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Stop hung during the restart delay")
	}

	require.Equal(t, libprocess.Stopped, p.State())
	require.Equal(t, 0, p.Pid(), "a replacement process was spawned despite Stop")

	// The replacement must never appear, even after the delay would have elapsed.
	_, err := os.Stat(marker)
	require.NoError(t, err)
	time.Sleep(600 * time.Millisecond)
	require.Equal(t, 0, p.Pid())
}

// Restarts is documented as counting from the last Start, so the restart Limit
// must not stay exhausted across supervised lifetimes.
func TestUnit_Process_StartResetsRestartCounter(t *testing.T) {
	cmd, args := shArgs("exit 1")
	p := newProc(t, libprocess.Config{
		Command: cmd,
		Args:    args,
		Restart: libprocess.RestartPolicy{Enabled: true, Limit: 1},
	})

	runToCrash := func() int {
		require.NoError(t, p.Start(t.Context()))
		select {
		case <-p.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("process did not reach a terminal state in time")
		}
		require.Equal(t, libprocess.Crashed, p.State())
		return p.Restarts()
	}

	require.Equal(t, 1, runToCrash(), "first lifetime should use its one allowed restart")
	require.Equal(t, 1, runToCrash(), "second lifetime must get a fresh restart budget")
}

// Done and Stop are the two ways a caller learns the supervised lifetime is
// over, so the terminal State must already be published when either returns.
// finish enforces this by ordering the state write and the lock release
// before close(done); the window is too narrow to fail reliably in a test, so
// this asserts the invariant rather than proving the ordering.
func TestUnit_Process_TerminalStateIsVisibleWhenDoneFires(t *testing.T) {
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args})

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not finish in time")
	}
	require.Equal(t, libprocess.Stopped, p.State(),
		"Done fired before the terminal state was published")
}

func TestUnit_Process_TerminalStateIsVisibleWhenStopReturns(t *testing.T) {
	cmd, args := shArgs("while true; do sleep 0.05; done")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args, StopGrace: time.Second})

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, p.Stop(t.Context()))
	require.Equal(t, libprocess.Stopped, p.State(),
		"Stop returned before the terminal state was published")
}

// newProc constructs a Process and fails the test on a configuration error,
// keeping the construction-time error contract out of every test body.
func newProc(t *testing.T, cfg libprocess.Config, opts ...libprocess.Option) *libprocess.Process {
	t.Helper()
	p, err := libprocess.New(cfg, opts...)
	require.NoError(t, err)
	return p
}

func TestUnit_New_RejectsInvalidConfig(t *testing.T) {
	t.Run("empty command", func(t *testing.T) {
		_, err := libprocess.New(libprocess.Config{})
		require.Error(t, err)
	})

	t.Run("renewal interval without a lock", func(t *testing.T) {
		_, err := libprocess.New(libprocess.Config{Command: "/bin/true"},
			libprocess.WithLock(nil, time.Second))
		require.Error(t, err)
	})

	// A restarting supervisor holds its claim across many command lifetimes,
	// so a TTL lock with no renewal lapses and a second supervisor starts a
	// duplicate — the exact failure the lock exists to prevent.
	t.Run("lock without renewal while restarting", func(t *testing.T) {
		_, err := libprocess.New(libprocess.Config{
			Command: "/bin/true",
			Restart: libprocess.RestartPolicy{Enabled: true},
		}, libprocess.WithLock(newFakeLock(), 0))
		require.Error(t, err)
	})
}

// A respawn failing partway through a restart policy happens long after Start
// returned, so the error handler is the only way a caller can learn about it.
func TestUnit_Process_ErrorHandlerReceivesFailedRestart(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sh-copy")
	raw, err := os.ReadFile("/bin/sh")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(bin, raw, 0o755))

	errs := make(chan error, 8)
	p := newProc(t, libprocess.Config{
		Command: bin,
		Args:    []string{"-c", "exit 1"},
		Restart: libprocess.RestartPolicy{Enabled: true, Delay: 400 * time.Millisecond},
	}, libprocess.WithErrorHandler(func(_ context.Context, err error) {
		select {
		case errs <- err:
		default:
		}
	}))

	require.NoError(t, p.Start(t.Context()))
	// Remove the binary while the supervisor waits out the restart delay, so
	// the respawn cannot succeed.
	require.Eventually(t, func() bool { return p.Pid() == 0 }, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, os.Remove(bin))

	select {
	case got := <-errs:
		require.ErrorContains(t, got, "restart")
	case <-time.After(5 * time.Second):
		t.Fatal("a failed respawn was never reported to the error handler")
	}
}

// A hook is an observer: its failure must be reported, and must not change
// the supervised process's own outcome.
func TestUnit_Process_ErrorHandlerReceivesHookFailure(t *testing.T) {
	errs := make(chan error, 8)
	hookErr := errors.New("hook exploded")
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args},
		libprocess.WithStateHook(func(_ context.Context, _ libprocess.StateChange) error {
			return hookErr
		}),
		libprocess.WithErrorHandler(func(_ context.Context, err error) {
			select {
			case errs <- err:
			default:
			}
		}))

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not finish in time")
	}

	require.Equal(t, libprocess.Stopped, p.State(), "a failing hook must not fail a healthy process")
	select {
	case got := <-errs:
		require.ErrorIs(t, got, hookErr)
	default:
		t.Fatal("hook failure was swallowed")
	}
}
