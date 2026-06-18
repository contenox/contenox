// Package owner manages lease-based ownership of the local runtime's resident
// state. On Join, one instance becomes the owner — it holds the lease and renews
// it in the background — while others become followers that know who the owner
// is and (later) where to reach it.
//
// The owner self-fences: if it cannot renew before the lease expires, Lost()
// fires and the caller MUST stop touching owned state, because another instance
// may have taken over. This is the in-process, no-daemon, cross-platform owner
// election described in
// docs/blueprints/local-runtime-owner-coordination.md.
package owner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/contenox/runtime/liblease"
)

// Role is whether this instance owns the lease.
type Role int

const (
	// RoleFollower means another instance currently owns the lease.
	RoleFollower Role = iota
	// RoleOwner means this instance holds and renews the lease.
	RoleOwner
)

func (r Role) String() string {
	if r == RoleOwner {
		return "owner"
	}
	return "follower"
}

// Config controls a Join.
type Config struct {
	// LeasePath is the lease file; one owner per path (per user + data root).
	LeasePath string
	// TTL is the lease duration.
	TTL time.Duration
	// RenewInterval is how often the owner renews; defaults to TTL/3.
	RenewInterval time.Duration
	// Endpoint, if set, is advertised to followers via the lease record so they
	// can reach the owner once it serves a transport (P3). Empty = not serving.
	Endpoint string
	// Backend, if set, is the inference backend this owner serves ("llama" /
	// "openvino" / "none"), advertised via the lease so the runtime can tell which
	// mode the daemon is in without a network round-trip.
	Backend string
}

// EndpointMetaKey is the lease metadata key used to advertise the owner's
// transport endpoint.
const EndpointMetaKey = "endpoint"

// BackendMetaKey is the lease metadata key used to advertise the inference
// backend the owner serves (mirrored by runtime/internal/modeldprobe).
const BackendMetaKey = "backend"

// leaseMeta builds the lease metadata advertised to followers. Empty values are
// omitted so a non-serving owner publishes no endpoint/backend.
func leaseMeta(cfg Config) map[string]string {
	meta := map[string]string{}
	if cfg.Endpoint != "" {
		meta[EndpointMetaKey] = cfg.Endpoint
	}
	if cfg.Backend != "" {
		meta[BackendMetaKey] = cfg.Backend
	}
	return meta
}

// Owner is the result of a Join: either this instance (RoleOwner) with a running
// renew loop, or a handle describing the current owner (RoleFollower).
type Owner struct {
	role   Role
	id     string
	holder liblease.Record

	lease  *liblease.Lease
	cancel context.CancelFunc

	lost     chan struct{}
	lostErr  error
	lostOnce sync.Once
	relOnce  sync.Once
	relErr   error
}

// Join attempts to acquire ownership of the lease at cfg.LeasePath. If the lease
// is free it returns an Owner in RoleOwner and starts renewing in the
// background; if a live owner already holds it, it returns RoleFollower with the
// current holder. ctx bounds the renew loop's lifetime; cancelling it stops
// renewal (call Release for a clean handover).
func Join(ctx context.Context, cfg Config) (*Owner, error) {
	if cfg.TTL <= 0 {
		return nil, errors.New("owner: ttl must be positive")
	}
	interval := cfg.RenewInterval
	if interval <= 0 {
		interval = cfg.TTL / 3
	}
	if interval <= 0 {
		interval = time.Second
	}

	var opts []liblease.Option
	if meta := leaseMeta(cfg); len(meta) > 0 {
		opts = append(opts, liblease.WithMeta(meta))
	}

	l, err := liblease.Acquire(cfg.LeasePath, cfg.TTL, opts...)
	if err != nil {
		if errors.Is(err, liblease.ErrHeld) {
			rec, ierr := liblease.Inspect(cfg.LeasePath)
			if ierr != nil {
				return nil, ierr
			}
			return &Owner{
				role:   RoleFollower,
				id:     rec.InstanceID,
				holder: rec,
				lost:   make(chan struct{}), // never fires for a follower
			}, nil
		}
		return nil, err
	}

	rctx, cancel := context.WithCancel(ctx)
	o := &Owner{
		role:   RoleOwner,
		id:     l.InstanceID(),
		holder: l.Record(),
		lease:  l,
		cancel: cancel,
		lost:   make(chan struct{}),
	}
	go o.renewLoop(rctx, interval)
	return o, nil
}

func (o *Owner) renewLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return // graceful stop (parent ctx or Release); not a loss
		case <-t.C:
			if err := o.lease.Renew(); err != nil {
				o.markLost(err)
				return
			}
		}
	}
}

func (o *Owner) markLost(err error) {
	o.lostOnce.Do(func() {
		o.lostErr = err
		close(o.lost)
	})
}

// Role reports whether this instance is the owner or a follower.
func (o *Owner) Role() Role { return o.role }

// IsOwner reports whether this instance holds the lease.
func (o *Owner) IsOwner() bool { return o.role == RoleOwner }

// InstanceID is this owner's id, or the current owner's id for a follower.
func (o *Owner) InstanceID() string { return o.id }

// Holder returns the current owner's lease record.
func (o *Owner) Holder() liblease.Record { return o.holder }

// Endpoint returns the owner's advertised endpoint, or "" if none.
func (o *Owner) Endpoint() string { return o.holder.Meta[EndpointMetaKey] }

// Backend returns the inference backend the owner serves, or "" if none.
func (o *Owner) Backend() string { return o.holder.Meta[BackendMetaKey] }

// Lost is closed when the owner can no longer renew the lease (taken over or an
// I/O failure). The caller must stop touching owned state when it fires. It
// never fires for a follower.
func (o *Owner) Lost() <-chan struct{} { return o.lost }

// LostErr returns why ownership was lost, after Lost fires.
func (o *Owner) LostErr() error { return o.lostErr }

// Release stops renewing and relinquishes the lease so a successor can take over
// immediately rather than waiting out the TTL. Safe to call multiple times; a
// no-op for a follower.
func (o *Owner) Release() error {
	o.relOnce.Do(func() {
		if o.cancel != nil {
			o.cancel()
		}
		if o.lease != nil {
			o.relErr = o.lease.Release()
		}
	})
	return o.relErr
}
