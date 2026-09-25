package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblease"
)

const (
	meterLeaseTickPart = 3
	meterLeaseRetry    = 5 * time.Second
)

// meterLeaseTTL is how long the writer role survives without a renewal; a
// follower may take it over once it lapses. It is a variable so a test can watch
// a takeover happen rather than wait out the deployment's window.
var meterLeaseTTL = 30 * time.Second

// meterWriter is the one writer of the metering tables. A usage snapshot is the
// previous snapshot plus this turn, so two writers computing it from the same
// predecessor lose one turn's charge; the lease makes the deployment single
// writer, and the metre converges on every turn rather than on a subset.
//
// This is the deployment's consistency model everywhere it crosses instances:
// state converges, and no request waits for that convergence. A follower does
// not hold the writer's record to write it — it publishes the turn and the
// writer counts it, so a charge lands a delivery later. The check on that path
// is the local clock against the last renewal, which is a comparison rather than
// a read of the lease file, and the lease is renewed on a ticker rather than
// during a request. Nothing here makes a turn wait on a lock.
//
// A writer whose renewal had already lapsed can charge a turn its successor
// counts too. That is over-count, not corruption: it converges when the window
// rolls, and the ceilings are soft. Excluding it entirely would mean a lock held
// across the request.
//
// Without a lease path every instance is a writer, which is correct for the one
// process the CLI serves and wrong for a deployment of several.
type meterWriter struct {
	usage runtimetypes.UsageStore
	path  string

	mu    sync.Mutex
	lease *liblease.Lease
}

func newMeterWriter(usage runtimetypes.UsageStore, path string) *meterWriter {
	return &meterWriter{usage: usage, path: path}
}

// Record appends the turn if this instance is the writer and reports whether it
// did. The role is read once per call, so no charge is both written here and
// sent to the writer: a follower publishes instead, and its turns reach the
// holder over the bus the way a peer's already do.
//
// The check is the local clock against the last renewal, which is a comparison
// and a mutex rather than a read of the lease file. Serving must not wait on a
// lock, so nothing on this path touches the filesystem.
func (m *meterWriter) Record(ctx context.Context, u runtimetypes.ProxyUsage) (bool, error) {
	if m == nil || m.usage == nil {
		return false, nil
	}
	if !m.holds() {
		return false, nil
	}
	return true, m.usage.RecordUsage(ctx, u)
}

// holds reports whether this instance is the writer. An unleased meter has no
// election to lose, so every instance is the writer: that is the single-process
// deployment, and it is the one case where the local write and the bus feed
// cannot both fire.

func (m *meterWriter) holds() bool {
	if m == nil || m.path == "" || m.usage == nil {
		return true
	}
	m.mu.Lock()
	lease := m.lease
	m.mu.Unlock()
	return lease != nil && !lease.Expired()
}

// Start begins contending for the writer role and keeps it renewed. It returns
// whether this instance holds the lease now, which is only a starting state: a
// follower keeps asking, so the role moves when the holder stops.
func (m *meterWriter) Start(ctx context.Context) (bool, error) {
	if m == nil || m.path == "" || m.usage == nil {
		return true, nil
	}
	held, err := m.acquire(ctx)
	go m.keep(ctx)
	return held, err
}

// keep renews while held and contends while not, so an instance that starts as a
// follower takes over once the holder's lease lapses.
func (m *meterWriter) keep(ctx context.Context) {
	ticker := time.NewTicker(meterLeaseTTL / meterLeaseTickPart)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.release()
			return
		case <-ticker.C:
			if m.holds() {
				if err := m.renew(ctx); err != nil {
					m.drop()
				}
				continue
			}
			_, _ = m.acquire(ctx)
		}
	}
}

func (m *meterWriter) acquire(ctx context.Context) (bool, error) {
	acquireCtx, cancel := context.WithTimeout(ctx, meterLeaseRetry)
	defer cancel()

	lease, err := liblease.AcquireContext(acquireCtx, m.path, meterLeaseTTL,
		liblease.WithMeta(map[string]string{"role": "meter-writer"}))
	if err != nil {
		if errors.Is(err, liblease.ErrHeld) || errors.Is(err, liblease.ErrAcquireTimeout) {
			return false, nil
		}
		return false, fmt.Errorf("gateway: meter lease: %w", err)
	}

	m.mu.Lock()
	previous := m.lease
	m.lease = lease
	m.mu.Unlock()
	if previous != nil {
		_ = previous.ReleaseContext(context.WithoutCancel(ctx))
	}
	return true, nil
}

// renew extends the role. It runs on its own deadline rather than the caller's
// context: a renewal is about this instance staying the writer, and must not be
// abandoned because the request that happened to start the loop is done.
func (m *meterWriter) renew(ctx context.Context) error {
	renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), meterLeaseRetry)
	defer cancel()

	m.mu.Lock()
	lease := m.lease
	m.mu.Unlock()
	if lease == nil {
		return nil
	}
	return lease.RenewContext(renewCtx)
}

// drop gives up the writer role after a renewal failed. The lease is somebody
// else's now, so the handle is discarded rather than released.
func (m *meterWriter) drop() {
	m.mu.Lock()
	m.lease = nil
	m.mu.Unlock()
}

func (m *meterWriter) release() {
	m.mu.Lock()
	lease := m.lease
	m.lease = nil
	m.mu.Unlock()
	if lease != nil {
		_ = lease.Release()
	}
}
