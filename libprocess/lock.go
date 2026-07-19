package libprocess

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/contenox/runtime/liblease"
)

// ErrLockUnavailable is returned by Start when the supervision lock is held by
// another supervisor, so this one must not spawn the command.
var ErrLockUnavailable = errors.New("libprocess: supervision lock unavailable")

// ErrLockLost reports that the supervision lock was lost while the command was
// running. The Process terminates the command and ends up Crashed: another
// supervisor is entitled to take over, and two live instances of a
// singleton command is the outcome the lock exists to prevent.
var ErrLockLost = errors.New("libprocess: supervision lock lost")

// Lock gates supervision so that only one supervisor runs a given command at a
// time — the singleton-process case across a cluster, where a plain in-process
// mutex is not enough.
//
// It is deliberately the smallest surface that expresses "claim, keep proving
// you are alive, hand back", so it can be satisfied by a file lease, a
// Postgres advisory lock, an etcd/Consul session, or a Redis lock without any
// of them being a dependency of this package. LeaseLock adapts liblease.
//
// Implementations must be safe for concurrent use: Renew is called from the
// Process's own goroutine while Release may be called from a caller's Stop.
type Lock interface {
	// Acquire claims the lock. It must return an error — ideally wrapping
	// ErrLockUnavailable — when another holder has it, and must not block
	// indefinitely once ctx is done.
	Acquire(ctx context.Context) error
	// Renew extends the claim. Any error is treated as having lost the lock,
	// so implementations must return nil only when the claim is definitely
	// still held.
	Renew(ctx context.Context) error
	// Release relinquishes the claim. It is called once per supervised
	// lifetime and must tolerate being called when the lock is already lost.
	Release(ctx context.Context) error
}

// WithLock makes supervision conditional on holding lock: Start acquires it
// before spawning and fails with ErrLockUnavailable if another supervisor
// holds it, the claim is renewed every renewEvery for as long as the command
// runs, and it is released once the process reaches a terminal state. Losing
// the claim mid-run terminates the command (see ErrLockLost) rather than
// letting a second supervisor start a duplicate.
//
// renewEvery must be comfortably shorter than the lock's expiry so a slow
// renewal does not read as a takeover; a third of the TTL is a reasonable
// default. A non-positive renewEvery disables renewal, which suits locks that
// do not expire (e.g. a Postgres advisory lock held on a live session) but is
// unsafe for TTL-based ones.
func WithLock(lock Lock, renewEvery time.Duration) Option {
	return func(p *Process) {
		p.lock = lock
		p.renewEvery = renewEvery
	}
}

// LeaseLock adapts a liblease file lease to Lock, giving cooperative
// single-supervisor behaviour across processes sharing a filesystem.
//
// liblease provides liveness, not safety: a supervisor paused longer than ttl
// can have its lease taken over while it still believes it holds it. Renewal
// failure here terminates the supervised command, which is the fencing action
// liblease's contract asks of a holder that cannot renew, but callers needing
// hard mutual exclusion should gate the protected work on the fencing token
// (InstanceID) too.
func LeaseLock(path string, ttl time.Duration) Lock {
	return &leaseLock{path: path, ttl: ttl}
}

type leaseLock struct {
	path string
	ttl  time.Duration

	// mu guards the lease handle itself. Lock's contract requires
	// implementations to be safe for concurrent use — a Process renews from its
	// own goroutine while a caller's Stop can release — and liblease's internal
	// mutex protects the *Lease, not this pointer to it.
	mu    sync.Mutex
	lease *liblease.Lease
}

func (l *leaseLock) Acquire(ctx context.Context) error {
	lease, err := liblease.AcquireContext(ctx, l.path, l.ttl)
	if err != nil {
		if errors.Is(err, liblease.ErrHeld) {
			return errors.Join(ErrLockUnavailable, err)
		}
		// ErrAcquireTimeout deliberately does NOT map to ErrLockUnavailable:
		// it means the holder could not be determined, not that someone else
		// holds it, and reporting a stalled filesystem as "held elsewhere"
		// would send a supervisor down the wrong branch.
		return err
	}
	l.mu.Lock()
	l.lease = lease
	l.mu.Unlock()
	return nil
}

func (l *leaseLock) Renew(ctx context.Context) error {
	l.mu.Lock()
	lease := l.lease
	l.mu.Unlock()
	if lease == nil {
		return ErrLockLost
	}
	return lease.RenewContext(ctx)
}

func (l *leaseLock) Release(ctx context.Context) error {
	l.mu.Lock()
	lease := l.lease
	l.lease = nil
	l.mu.Unlock()
	if lease == nil {
		return nil
	}
	return lease.ReleaseContext(ctx)
}
