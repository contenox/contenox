package liblease_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
)

func leasePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "owner.lease")
}

func TestUnit_Lease_AcquireFreeThenHeld(t *testing.T) {
	path := leasePath(t)

	l, err := liblease.Acquire(path, time.Minute)
	if err != nil {
		t.Fatalf("Acquire free: %v", err)
	}
	if l.InstanceID() == "" {
		t.Fatal("expected an instance id")
	}

	// A second acquire while the first is valid must be refused.
	if _, err := liblease.Acquire(path, time.Minute); !errors.Is(err, liblease.ErrHeld) {
		t.Fatalf("second Acquire: want ErrHeld, got %v", err)
	}
}

func TestUnit_Lease_TakeoverAfterExpiry(t *testing.T) {
	path := leasePath(t)

	first, err := liblease.Acquire(path, 40*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	time.Sleep(70 * time.Millisecond) // let it lapse

	second, err := liblease.Acquire(path, time.Minute)
	if err != nil {
		t.Fatalf("takeover after expiry: %v", err)
	}
	if second.InstanceID() == first.InstanceID() {
		t.Fatal("takeover should mint a new instance id")
	}

	// The original holder has lost it: Renew must report ErrLost.
	if err := first.Renew(); !errors.Is(err, liblease.ErrLost) {
		t.Fatalf("stale Renew: want ErrLost, got %v", err)
	}
}

func TestUnit_Lease_RenewKeepsOwnership(t *testing.T) {
	path := leasePath(t)

	l, err := liblease.Acquire(path, 60*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Renew past the original expiry window; ownership should hold.
	for range 3 {
		time.Sleep(30 * time.Millisecond)
		if err := l.Renew(); err != nil {
			t.Fatalf("Renew: %v", err)
		}
	}
	if l.Expired() {
		t.Fatal("lease should not be expired right after a renew")
	}
	// A challenger still cannot take a renewed, valid lease.
	if _, err := liblease.Acquire(path, time.Minute); !errors.Is(err, liblease.ErrHeld) {
		t.Fatalf("Acquire on renewed lease: want ErrHeld, got %v", err)
	}
}

func TestUnit_Lease_ReleaseFreesIt(t *testing.T) {
	path := leasePath(t)

	l, err := liblease.Acquire(path, time.Minute)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := liblease.Inspect(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("after Release: want ErrNotExist, got %v", err)
	}
	// Now freely re-acquirable.
	if _, err := liblease.Acquire(path, time.Minute); err != nil {
		t.Fatalf("re-Acquire after Release: %v", err)
	}
}

func TestUnit_Lease_ReleaseAfterTakeoverIsNoop(t *testing.T) {
	path := leasePath(t)

	first, err := liblease.Acquire(path, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	second, err := liblease.Acquire(path, time.Minute)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}

	// The original holder releasing must not remove the new holder's lease.
	if err := first.Release(); err != nil {
		t.Fatalf("stale Release: %v", err)
	}
	rec, err := liblease.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect after stale Release: %v", err)
	}
	if rec.InstanceID != second.InstanceID() {
		t.Fatal("stale Release wrongly disturbed the new holder's lease")
	}
}

// TestUnit_Lease_SingleWinnerRace is the core invariant: when many instances
// contend for a free lease at once, exactly one acquires it.
func TestUnit_Lease_SingleWinnerRace(t *testing.T) {
	path := leasePath(t)

	const racers = 25
	var wins int64
	var wg sync.WaitGroup
	bad := make(chan error, racers)

	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := liblease.Acquire(path, time.Minute)
			switch {
			case err == nil:
				atomic.AddInt64(&wins, 1)
			case errors.Is(err, liblease.ErrHeld):
				// expected for losers
			default:
				bad <- err
			}
		}()
	}
	wg.Wait()
	close(bad)
	for err := range bad {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", wins)
	}
}
