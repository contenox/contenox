package libprocess_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libprocess"
	"github.com/stretchr/testify/require"
)

// fakeLock is a programmable Lock: it can refuse acquisition and can be made
// to start failing renewals to simulate a takeover by another supervisor.
type fakeLock struct {
	mu           sync.Mutex
	acquireErr   error
	renewErr     error
	acquired     int
	released     int
	renewed      int
	renewFailedC chan struct{}
}

func newFakeLock() *fakeLock { return &fakeLock{renewFailedC: make(chan struct{})} }

func (f *fakeLock) Acquire(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acquireErr != nil {
		return f.acquireErr
	}
	f.acquired++
	return nil
}

func (f *fakeLock) Renew(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.renewErr != nil {
		select {
		case <-f.renewFailedC:
		default:
			close(f.renewFailedC)
		}
		return f.renewErr
	}
	f.renewed++
	return nil
}

func (f *fakeLock) Release(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
	return nil
}

func (f *fakeLock) loseIt(err error) {
	f.mu.Lock()
	f.renewErr = err
	f.mu.Unlock()
}

func (f *fakeLock) counts() (acquired, released int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquired, f.released
}

var _ libprocess.Lock = (*fakeLock)(nil)

// A supervisor that cannot claim the lock must not spawn the command at all —
// that is the entire purpose of the lock.
func TestUnit_Process_LockUnavailableDoesNotSpawn(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	lock := newFakeLock()
	lock.acquireErr = libprocess.ErrLockUnavailable

	cmd, args := shArgs("touch " + marker)
	p := newProc(t, libprocess.Config{Command: cmd, Args: args},
		libprocess.WithLock(lock, 10*time.Millisecond))

	err := p.Start(t.Context())
	require.ErrorIs(t, err, libprocess.ErrLockUnavailable)
	require.Equal(t, libprocess.Stopped, p.State(), "a refused Start must leave the Process untouched")
	require.Equal(t, 0, p.Pid())

	require.NoFileExists(t, marker, "command ran despite the lock being held elsewhere")
	acquired, released := lock.counts()
	require.Equal(t, 0, acquired)
	require.Equal(t, 0, released, "nothing was claimed, so nothing may be released")
}

// The lock must be released once the supervised lifetime ends, or the next
// supervisor can never take over.
func TestUnit_Process_LockReleasedOnTerminalState(t *testing.T) {
	lock := newFakeLock()
	cmd, args := shArgs("exit 0")
	p := newProc(t, libprocess.Config{Command: cmd, Args: args},
		libprocess.WithLock(lock, time.Hour)) // no renewal during this test

	require.NoError(t, p.Start(t.Context()))
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not reach a terminal state in time")
	}

	acquired, released := lock.counts()
	require.Equal(t, 1, acquired)
	require.Equal(t, 1, released)
}

// Losing the claim mid-run must terminate the command: another supervisor is
// now entitled to start one, and two live instances is what the lock prevents.
func TestUnit_Process_LockLossTerminatesCommand(t *testing.T) {
	lock := newFakeLock()
	var mu sync.Mutex
	var terminal libprocess.StateChange
	hook := libprocess.WithStateHook(func(_ context.Context, sc libprocess.StateChange) error {
		if sc.To == libprocess.Crashed || sc.To == libprocess.Stopped {
			mu.Lock()
			terminal = sc
			mu.Unlock()
		}
		return nil
	})

	cmd, args := shArgs("sleep 30")
	p := newProc(t, libprocess.Config{
		Command: cmd, Args: args,
		StopGrace: 200 * time.Millisecond,
		// Restart must not resurrect a command we lost the right to run.
		Restart: libprocess.RestartPolicy{Enabled: true, Always: true},
	}, libprocess.WithLock(lock, 10*time.Millisecond), hook)

	require.NoError(t, p.Start(t.Context()))
	require.Eventually(t, func() bool { return p.Pid() != 0 }, 2*time.Second, 10*time.Millisecond)

	takeover := errors.New("taken over by another instance")
	lock.loseIt(takeover)

	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("losing the lock did not terminate the supervised command")
	}

	require.Equal(t, libprocess.Crashed, p.State(),
		"a fenced process is Crashed, not a clean Stopped")
	mu.Lock()
	got := terminal
	mu.Unlock()
	require.ErrorIs(t, got.Err, libprocess.ErrLockLost)
	require.ErrorIs(t, got.Err, takeover, "the underlying cause must survive for the caller")

	require.Equal(t, 0, p.Pid(), "the command outlived the lock it required")
	_, released := lock.counts()
	require.Equal(t, 1, released)

	// The Always restart policy must not bring it back without the lock.
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0, p.Pid())
	require.Equal(t, libprocess.Crashed, p.State())
}

// LeaseLock must give real cross-supervisor exclusion over a shared path.
func TestUnit_LeaseLock_ExcludesSecondSupervisor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "supervisor.lease")
	cmd, args := shArgs("sleep 30")

	first := newProc(t, libprocess.Config{Command: cmd, Args: args, StopGrace: time.Second},
		libprocess.WithLock(libprocess.LeaseLock(path, 30*time.Second), 5*time.Second))
	require.NoError(t, first.Start(t.Context()))
	t.Cleanup(func() { _ = first.Stop(t.Context()) })
	require.Eventually(t, func() bool { return first.Pid() != 0 }, 2*time.Second, 10*time.Millisecond)

	second := newProc(t, libprocess.Config{Command: cmd, Args: args},
		libprocess.WithLock(libprocess.LeaseLock(path, 30*time.Second), 5*time.Second))
	err := second.Start(t.Context())
	require.ErrorIs(t, err, libprocess.ErrLockUnavailable)
	require.Equal(t, 0, second.Pid())

	// Once the holder stops and releases, the lease is claimable again.
	require.NoError(t, first.Stop(t.Context()))
	require.NoError(t, second.Start(t.Context()))
	t.Cleanup(func() { _ = second.Stop(t.Context()) })
	require.Eventually(t, func() bool { return second.Pid() != 0 }, 2*time.Second, 10*time.Millisecond)
}
