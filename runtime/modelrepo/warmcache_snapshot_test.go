package modelrepo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// snapWarmSession is a fake warm session whose "resident KV" is just a token
// count, snapshotted/restored as its decimal string. It lets the WarmCache
// snapshot-hook tests assert warm-vs-cold behavior without any real backend.
type snapWarmSession struct {
	mu       sync.Mutex
	closed   bool
	resident int
	// failRestore makes Restore reject any snapshot, modeling an incompatible
	// manifest — the cache must fall back to a cold session, never corrupt state.
	failRestore bool
}

func (f *snapWarmSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *snapWarmSession) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func mustAcquireSnap(t *testing.T, c *WarmCache[*snapWarmSession], key string, s *snapWarmSession) *WarmEntry[*snapWarmSession] {
	t.Helper()
	e, err := c.Acquire(key, func() (*snapWarmSession, error) { return s, nil })
	if err != nil {
		t.Fatalf("acquire %q: %v", key, err)
	}
	return e
}

func newSnapCache(t *testing.T, maxResident int) (*WarmCache[*snapWarmSession], *MemSnapshotStore) {
	t.Helper()
	withWarmCacheConfig(t, maxResident, 0)
	store := NewMemSnapshotStore()
	c := NewWarmCacheWithSnapshots[*snapWarmSession](
		store,
		func(_ context.Context, s *snapWarmSession) ([]byte, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			return []byte{byte(s.resident)}, nil
		},
		func(_ context.Context, s *snapWarmSession, blob []byte) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.failRestore {
				return errors.New("incompatible manifest")
			}
			if len(blob) != 1 {
				return errors.New("bad snapshot")
			}
			s.resident = int(blob[0])
			return nil
		},
	)
	return c, store
}

// TestUnit_WarmCache_EvictionCapturesAndReacquireRestores is the swap-survival
// proof: evicting session "a" (LRU, over the single-slot cap) captures its
// resident state to the store; reacquiring "a" later opens a fresh (cold, empty)
// session but the cache restores the captured state into it before returning —
// so the caller sees the old resident value, not a cold zero.
func TestUnit_WarmCache_EvictionCapturesAndReacquireRestores(t *testing.T) {
	c, _ := newSnapCache(t, 1) // cap 1: opening "b" evicts "a"

	a := &snapWarmSession{resident: 42}
	mustAcquireSnap(t, c, "a", a)

	b := &snapWarmSession{resident: 7}
	if _, err := c.Acquire("b", func() (*snapWarmSession, error) { return b, nil }); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	if !a.isClosed() {
		t.Fatal("expected 'a' to be evicted (over cap) when 'b' opened")
	}

	// Swap back to "a": the cache opens a brand-new cold session (resident=0)
	// but must restore the captured snapshot into it before returning.
	fresh := &snapWarmSession{resident: 0}
	ea2, err := c.Acquire("a", func() (*snapWarmSession, error) { return fresh, nil })
	if err != nil {
		t.Fatalf("reacquire a: %v", err)
	}
	if ea2.Sess.resident != 42 {
		t.Fatalf("expected restored resident=42, got %d (cold reopen, snapshot not applied)", ea2.Sess.resident)
	}
}

// TestUnit_WarmCache_NoAcquireWithoutEvictionMeansNoSnapshotNeeded proves a warm
// hit (no eviction ever happened) never touches the snapshot store at all — the
// original resident session is returned directly.
func TestUnit_WarmCache_NoAcquireWithoutEvictionMeansNoSnapshotNeeded(t *testing.T) {
	c, store := newSnapCache(t, 2)
	a := &snapWarmSession{resident: 9}
	mustAcquireSnap(t, c, "a", a)
	if _, ok := store.Load("a"); ok {
		t.Fatal("no eviction occurred yet; store must not have a snapshot for 'a'")
	}
}

// TestUnit_WarmCache_IncompatibleRestoreFallsBackToCold proves that when Restore
// rejects a captured snapshot (e.g. a manifest/runtime mismatch), the cache does
// not error and does not corrupt the fresh session: it just leaves the cold
// session as opened, and drops the bad snapshot from the store so it is not
// retried forever.
func TestUnit_WarmCache_IncompatibleRestoreFallsBackToCold(t *testing.T) {
	c, store := newSnapCache(t, 1)

	a := &snapWarmSession{resident: 42}
	mustAcquireSnap(t, c, "a", a)

	b := &snapWarmSession{resident: 7}
	if _, err := c.Acquire("b", func() (*snapWarmSession, error) { return b, nil }); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	if _, ok := store.Load("a"); !ok {
		t.Fatal("expected a captured snapshot for 'a' after eviction")
	}

	fresh := &snapWarmSession{resident: 0, failRestore: true}
	ea, err := c.Acquire("a", func() (*snapWarmSession, error) { return fresh, nil })
	if err != nil {
		t.Fatalf("reacquire a: %v", err)
	}
	if ea.Sess.resident != 0 {
		t.Fatalf("expected cold session left untouched (resident=0), got %d", ea.Sess.resident)
	}
	if _, ok := store.Load("a"); ok {
		t.Fatal("expected the incompatible snapshot to be dropped from the store")
	}
}

// TestUnit_WarmCache_DropDoesNotCapture proves the fatal-eviction path (Drop)
// never captures a snapshot: a session marked fatal by the backend has
// untrustworthy state, and persisting it would let a later restore resurrect
// corrupt KV instead of cold-prefilling.
func TestUnit_WarmCache_DropDoesNotCapture(t *testing.T) {
	c, store := newSnapCache(t, 2)
	a := &snapWarmSession{resident: 42}
	ea := mustAcquireSnap(t, c, "a", a)
	c.Drop(ea)
	if !a.isClosed() {
		t.Fatal("Drop should close the session")
	}
	if _, ok := store.Load("a"); ok {
		t.Fatal("Drop (fatal eviction) must never persist a snapshot")
	}
}

// TestUnit_WarmCache_IdleReapCapturesToo proves the idle-TTL reap path (not just
// the over-cap LRU path) also captures before closing, since it goes through the
// same closeVictims helper.
func TestUnit_WarmCache_IdleReapCapturesToo(t *testing.T) {
	withWarmCacheConfig(t, 10, 5*time.Minute)
	store := NewMemSnapshotStore()
	c := NewWarmCacheWithSnapshots[*snapWarmSession](
		store,
		func(_ context.Context, s *snapWarmSession) ([]byte, error) { return []byte{byte(s.resident)}, nil },
		func(_ context.Context, s *snapWarmSession, blob []byte) error { s.resident = int(blob[0]); return nil },
	)
	clock := time.Now()
	c.now = func() time.Time { return clock }

	a := &snapWarmSession{resident: 5}
	mustAcquireSnap(t, c, "a", a)

	clock = clock.Add(6 * time.Minute)
	c.Reap()
	if !a.isClosed() {
		t.Fatal("expected idle session to be reaped")
	}
	if _, ok := store.Load("a"); !ok {
		t.Fatal("expected idle reap to capture a snapshot before closing")
	}
}

// TestUnit_WarmCache_CaptureResidentPersistsWithoutEviction is the one-shot-exit
// proof: a session that is never evicted (no model swap, no idle reap) is still
// persisted by CaptureResident, so a fresh cache sharing the same store — the
// next `contenox chat` process — restores it warm. Without this, the durable
// store would be dead weight for the dominant one-shot CLI path.
func TestUnit_WarmCache_CaptureResidentPersistsWithoutEviction(t *testing.T) {
	withWarmCacheConfig(t, 2, 0)
	store := NewMemSnapshotStore()
	mk := func() *WarmCache[*snapWarmSession] {
		return NewWarmCacheWithSnapshots[*snapWarmSession](
			store,
			func(_ context.Context, s *snapWarmSession) ([]byte, error) { return []byte{byte(s.resident)}, nil },
			func(_ context.Context, s *snapWarmSession, blob []byte) error { s.resident = int(blob[0]); return nil },
		)
	}

	c1 := mk()
	a := &snapWarmSession{resident: 99}
	mustAcquireSnap(t, c1, "a", a) // resident, never evicted
	if _, ok := store.Load("a"); ok {
		t.Fatal("nothing should be persisted before the exit flush")
	}

	c1.CaptureResident() // graceful-exit flush
	if _, ok := store.Load("a"); !ok {
		t.Fatal("CaptureResident must persist the resident session")
	}

	// Next process: a fresh cache over the same store reopens A cold and restores.
	c2 := mk()
	fresh := &snapWarmSession{resident: 0}
	ea, err := c2.Acquire("a", func() (*snapWarmSession, error) { return fresh, nil })
	if err != nil {
		t.Fatalf("reacquire a in fresh cache: %v", err)
	}
	if ea.Sess.resident != 99 {
		t.Fatalf("expected restored resident=99 across a simulated restart, got %d", ea.Sess.resident)
	}
}

// TestUnit_WarmCache_CaptureResidentSkipsMidTurn proves the exit flush never
// snapshots a session whose turn is in flight (its KV is mid-mutation).
func TestUnit_WarmCache_CaptureResidentSkipsMidTurn(t *testing.T) {
	withWarmCacheConfig(t, 2, 0)
	store := NewMemSnapshotStore()
	c := NewWarmCacheWithSnapshots[*snapWarmSession](
		store,
		func(_ context.Context, s *snapWarmSession) ([]byte, error) { return []byte{byte(s.resident)}, nil },
		func(_ context.Context, s *snapWarmSession, blob []byte) error { s.resident = int(blob[0]); return nil },
	)
	a := &snapWarmSession{resident: 5}
	ea := mustAcquireSnap(t, c, "a", a)
	ea.Turn.Lock() // mid-turn
	defer ea.Turn.Unlock()

	c.CaptureResident()
	if _, ok := store.Load("a"); ok {
		t.Fatal("a mid-turn session must not be captured")
	}
}

// TestUnit_WarmCache_DisabledCaptureBridgeSkipsSnapshotCost proves that when the
// capture bridge reports "disabled" by returning an empty blob (the kill-switch
// contract), eviction persists nothing and the store is never written.
func TestUnit_WarmCache_DisabledCaptureBridgeSkipsSnapshotCost(t *testing.T) {
	withWarmCacheConfig(t, 1, 0)
	store := NewMemSnapshotStore()
	captureCalls := 0
	c := NewWarmCacheWithSnapshots[*snapWarmSession](
		store,
		func(_ context.Context, s *snapWarmSession) ([]byte, error) { captureCalls++; return nil, nil }, // disabled: empty blob
		func(_ context.Context, s *snapWarmSession, blob []byte) error { s.resident = int(blob[0]); return nil },
	)
	a := &snapWarmSession{resident: 42}
	mustAcquireSnap(t, c, "a", a)
	b := &snapWarmSession{resident: 7}
	if _, err := c.Acquire("b", func() (*snapWarmSession, error) { return b, nil }); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	if captureCalls == 0 {
		t.Fatal("capture bridge should be consulted on eviction")
	}
	if _, ok := store.Load("a"); ok {
		t.Fatal("a disabled capture bridge (empty blob) must persist nothing")
	}
}

// TestUnit_WarmCache_SnapshotMaxBlobBytesSkipsLargeSnapshots proves an oversized
// snapshot blob is not persisted, bounding capture latency/bandwidth.
func TestUnit_WarmCache_SnapshotMaxBlobBytesSkipsLargeSnapshots(t *testing.T) {
	withWarmCacheConfig(t, 1, 0)
	orig := SnapshotMaxBlobBytes
	SnapshotMaxBlobBytes = 4
	t.Cleanup(func() { SnapshotMaxBlobBytes = orig })

	store := NewMemSnapshotStore()
	c := NewWarmCacheWithSnapshots[*snapWarmSession](
		store,
		func(_ context.Context, s *snapWarmSession) ([]byte, error) { return make([]byte, 16), nil }, // 16 > 4
		func(_ context.Context, s *snapWarmSession, blob []byte) error { return nil },
	)
	a := &snapWarmSession{resident: 1}
	mustAcquireSnap(t, c, "a", a)
	b := &snapWarmSession{resident: 2}
	if _, err := c.Acquire("b", func() (*snapWarmSession, error) { return b, nil }); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	if _, ok := store.Load("a"); ok {
		t.Fatal("an over-limit snapshot blob must be skipped")
	}
}

// TestUnit_WarmCache_WithoutSnapshotHooksBehavesLikePlainCache proves
// NewWarmCache (no snapshot wiring) is unaffected: eviction never touches any
// store and reacquire always opens genuinely cold, matching pre-snapshot
// behavior exactly.
func TestUnit_WarmCache_WithoutSnapshotHooksBehavesLikePlainCache(t *testing.T) {
	withWarmCacheConfig(t, 1, 0)
	c := NewWarmCache[*snapWarmSession]()

	a := &snapWarmSession{resident: 42}
	mustAcquireSnap(t, c, "a", a)

	b := &snapWarmSession{resident: 7}
	if _, err := c.Acquire("b", func() (*snapWarmSession, error) { return b, nil }); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	if !a.isClosed() {
		t.Fatal("expected 'a' evicted over cap")
	}

	fresh := &snapWarmSession{resident: 0}
	ea, err := c.Acquire("a", func() (*snapWarmSession, error) { return fresh, nil })
	if err != nil {
		t.Fatalf("reacquire a: %v", err)
	}
	if ea.Sess.resident != 0 {
		t.Fatalf("plain WarmCache must never restore state; got resident=%d", ea.Sess.resident)
	}
}
