// Package liblease implements a cooperative, time-bounded file lease: a
// single-holder lock backed by an ordinary file, not an OS primitive. The file
// records who holds the lease and until when; a challenger may take over only
// once the lease has expired.
//
// It is intentionally OS-agnostic — just a JSON file with atomic writes — so it
// behaves identically on Linux, macOS, and Windows.
//
// A lease gives liveness, not safety, on its own. The holder MUST renew before
// expiry and MUST stop touching protected state if it cannot (see Expired): a
// process paused longer than the TTL can be taken over while it still believes
// it holds the lease. Protect critical operations by also checking the holder's
// InstanceID (a fencing token) so a revived stale holder cannot act.
package liblease

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ErrHeld is returned by Acquire when a live (unexpired) lease is held by a
// different instance.
var ErrHeld = errors.New("liblease: lease is held by another instance")

// ErrLost is returned by Renew when the lease has been taken over by another
// instance — the caller is no longer the holder.
var ErrLost = errors.New("liblease: lease was taken over by another instance")

// Record is the on-disk lease state. PID and Host are informational (the lease
// is enforced by TTL, not by process checks); InstanceID is the fencing token.
// Meta carries optional holder-supplied discovery data — e.g. the endpoint a
// serving owner advertises to followers — so "who and where" stays one atomic
// read.
type Record struct {
	InstanceID string            `json:"instance_id"`
	PID        int               `json:"pid"`
	Host       string            `json:"host"`
	AcquiredAt time.Time         `json:"acquired_at"`
	RenewedAt  time.Time         `json:"renewed_at"`
	TTL        time.Duration     `json:"ttl"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// Option customizes a lease at Acquire time.
type Option func(*Record)

// WithMeta attaches holder-supplied discovery data to the lease record. It is
// preserved across Renew.
func WithMeta(meta map[string]string) Option {
	return func(r *Record) { r.Meta = meta }
}

// ExpiresAt reports when the lease lapses.
func (r Record) ExpiresAt() time.Time { return r.RenewedAt.Add(r.TTL) }

func (r Record) expired(now time.Time) bool { return now.After(r.ExpiresAt()) }

// Lease is a held lease handle. It is not safe for concurrent use; call Renew
// from a single goroutine.
type Lease struct {
	path string
	rec  Record
}

// Acquire claims the lease at path for ttl. It succeeds when the lease is free,
// expired, or unreadable, and fails with ErrHeld when a different instance
// holds a still-valid lease. On success the caller must Renew before ttl
// elapses and Release on shutdown.
func Acquire(path string, ttl time.Duration, opts ...Option) (*Lease, error) {
	if ttl <= 0 {
		return nil, errors.New("liblease: ttl must be positive")
	}
	now := time.Now()
	host, _ := os.Hostname()
	rec := Record{
		InstanceID: uuid.NewString(),
		PID:        os.Getpid(),
		Host:       host,
		AcquiredAt: now,
		RenewedAt:  now,
		TTL:        ttl,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&rec)
		}
	}

	// Fast path: create the lease exclusively. os.Link fails if the file
	// already exists, so when the lease is free this is a race-free,
	// single-winner acquisition with no readback needed.
	won, err := tryCreate(path, rec)
	if err != nil {
		return nil, err
	}
	if won {
		return &Lease{path: path, rec: rec}, nil
	}

	// A lease file exists: refuse unless it has expired (a corrupt/unreadable
	// file is treated as stale).
	if cur, rerr := readRecord(path); rerr == nil && !cur.expired(now) {
		return nil, fmt.Errorf("%w: instance %s (pid %d) until %s",
			ErrHeld, cur.InstanceID, cur.PID, cur.ExpiresAt().Format(time.RFC3339))
	}

	// Expired: take over with an atomic overwrite, then verify we won. Two
	// challengers can race here; the last rename wins and the loser sees a
	// different id on readback. The InstanceID fencing token guards the rest.
	if err := writeRecord(path, rec); err != nil {
		return nil, err
	}
	after, err := readRecord(path)
	if err != nil {
		return nil, err
	}
	if after.InstanceID != rec.InstanceID {
		return nil, fmt.Errorf("%w: lost takeover race to instance %s", ErrHeld, after.InstanceID)
	}
	return &Lease{path: path, rec: rec}, nil
}

// tryCreate atomically creates the lease file only if it does not already
// exist, via a temp file + hardlink (link fails with EEXIST when the target is
// present). It returns (false, nil) when the file already exists.
func tryCreate(path string, rec Record) (bool, error) {
	tmpName, err := writeTemp(path, rec)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmpName)
	if err := os.Link(tmpName, path); err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Renew extends the lease by another TTL from now. It returns ErrLost if the
// lease has since been taken over by another instance, which the caller must
// treat as losing ownership.
func (l *Lease) Renew() error {
	cur, err := readRecord(l.path)
	if err != nil {
		return fmt.Errorf("liblease: renew: %w", err)
	}
	if cur.InstanceID != l.rec.InstanceID {
		return fmt.Errorf("%w: now held by %s", ErrLost, cur.InstanceID)
	}
	l.rec.RenewedAt = time.Now()
	return writeRecord(l.path, l.rec)
}

// Release relinquishes the lease by removing the file, but only if it is still
// ours. It is safe to call more than once.
func (l *Lease) Release() error {
	cur, err := readRecord(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if cur.InstanceID != l.rec.InstanceID {
		return nil // taken over already; not ours to remove
	}
	return os.Remove(l.path)
}

// Expired reports whether this holder's lease has lapsed based on its last
// successful Renew. A holder should use it to self-fence: if Expired is true,
// stop touching protected state — a takeover may have happened.
func (l *Lease) Expired() bool { return l.rec.expired(time.Now()) }

// InstanceID returns this holder's fencing token.
func (l *Lease) InstanceID() string { return l.rec.InstanceID }

// Record returns a snapshot of this holder's lease state.
func (l *Lease) Record() Record { return l.rec }

// Inspect reads the current lease at path without acquiring it. It returns
// os.ErrNotExist when no lease file is present.
func Inspect(path string) (Record, error) {
	return readRecord(path)
}

func readRecord(path string) (Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return Record{}, fmt.Errorf("liblease: corrupt lease %q: %w", path, err)
	}
	return rec, nil
}

// writeRecord writes rec atomically (temp file + rename) so a reader never sees
// a half-written lease and a crash mid-write cannot corrupt it.
func writeRecord(path string, rec Record) error {
	tmpName, err := writeTemp(path, rec)
	if err != nil {
		return err
	}
	defer os.Remove(tmpName)
	return os.Rename(tmpName, path)
}

// writeTemp marshals rec to a fresh temp file next to path and returns its
// name. The caller publishes it via rename (overwrite) or link (exclusive
// create), then removes the temp name.
func writeTemp(path string, rec Record) (string, error) {
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lease-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return tmpName, nil
}
