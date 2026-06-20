package owner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
)

func TestUnit_JoinRetriesWhenHeldLeaseIsReleasedBeforeInspect(t *testing.T) {
	origAcquire := acquireLease
	origInspect := inspectLease
	t.Cleanup(func() {
		acquireLease = origAcquire
		inspectLease = origInspect
	})

	leasePath := filepath.Join(t.TempDir(), "modeld.lease")
	acquireCalls := 0
	inspectCalls := 0
	acquireLease = func(path string, ttl time.Duration, opts ...liblease.Option) (*liblease.Lease, error) {
		acquireCalls++
		if acquireCalls == 1 {
			return nil, fmt.Errorf("%w: synthetic holder", liblease.ErrHeld)
		}
		return origAcquire(path, ttl, opts...)
	}
	inspectLease = func(path string) (liblease.Record, error) {
		inspectCalls++
		if inspectCalls == 1 {
			return liblease.Record{}, os.ErrNotExist
		}
		return origInspect(path)
	}

	o, err := Join(context.Background(), Config{
		LeasePath: leasePath,
		TTL:       time.Minute,
		Endpoint:  "127.0.0.1:12345",
	})
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	t.Cleanup(func() { _ = o.Release() })
	if !o.IsOwner() {
		t.Fatalf("Join returned role %s, want owner", o.Role())
	}
	if acquireCalls != 2 {
		t.Fatalf("acquire calls = %d, want retry after missing inspected lease", acquireCalls)
	}
	if inspectCalls != 1 {
		t.Fatalf("inspect calls = %d, want one missing-lease inspection", inspectCalls)
	}
}
