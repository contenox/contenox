package modelrepo

import (
	"sync"
	"testing"
	"time"
)

type fakeWarmSession struct {
	mu     sync.Mutex
	closed bool
}

func (f *fakeWarmSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeWarmSession) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// withWarmCacheConfig sets the cache tunables for a test and restores them after.
func withWarmCacheConfig(t *testing.T, maxResident int, idleTTL time.Duration) {
	t.Helper()
	origMax, origTTL := WarmCacheMaxResident, WarmCacheIdleTTL
	WarmCacheMaxResident, WarmCacheIdleTTL = maxResident, idleTTL
	t.Cleanup(func() { WarmCacheMaxResident, WarmCacheIdleTTL = origMax, origTTL })
}

func mustAcquire(t *testing.T, c *WarmCache[*fakeWarmSession], key string, s *fakeWarmSession) *WarmEntry[*fakeWarmSession] {
	t.Helper()
	e, err := c.Acquire(key, func() (*fakeWarmSession, error) { return s, nil })
	if err != nil {
		t.Fatalf("acquire %q: %v", key, err)
	}
	return e
}

// TestUnit_WarmCache_CapEvictsLRUAndCloses is the model-switch leak proof: with a
// resident cap, opening more distinct sessions than the cap evicts and closes the
// least-recently-used one (releasing its model in modeld) instead of stacking.
func TestUnit_WarmCache_CapEvictsLRUAndCloses(t *testing.T) {
	withWarmCacheConfig(t, 2, 0) // cap 2, TTL disabled

	c := NewWarmCache[*fakeWarmSession]()
	clock := time.Now()
	c.now = func() time.Time { return clock }

	a, b, d := &fakeWarmSession{}, &fakeWarmSession{}, &fakeWarmSession{}
	mustAcquire(t, c, "a", a)
	clock = clock.Add(time.Second)
	mustAcquire(t, c, "b", b)
	clock = clock.Add(time.Second)
	mustAcquire(t, c, "c", d) // over cap -> evict LRU "a"

	if !a.isClosed() {
		t.Fatal("LRU session 'a' should have been evicted and closed")
	}
	if b.isClosed() || d.isClosed() {
		t.Fatal("'b' and 'c' must stay resident")
	}
}

// TestUnit_WarmCache_WarmHitReusesAndDoesNotClose proves a repeated acquire of the
// same key returns the same warm entry without opening or closing anything.
func TestUnit_WarmCache_WarmHitReusesAndDoesNotClose(t *testing.T) {
	withWarmCacheConfig(t, 2, 0)

	c := NewWarmCache[*fakeWarmSession]()
	a := &fakeWarmSession{}
	e1 := mustAcquire(t, c, "a", a)
	e2, err := c.Acquire("a", func() (*fakeWarmSession, error) {
		t.Fatal("warm hit must not open a new session")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("acquire warm: %v", err)
	}
	if e1 != e2 || a.isClosed() {
		t.Fatal("warm hit should return the same live entry")
	}
}

// TestUnit_WarmCache_DoesNotEvictMidTurn proves an entry whose Turn is held (a turn
// in flight) is never evicted, even when over the resident cap.
func TestUnit_WarmCache_DoesNotEvictMidTurn(t *testing.T) {
	withWarmCacheConfig(t, 1, 0)

	c := NewWarmCache[*fakeWarmSession]()
	clock := time.Now()
	c.now = func() time.Time { return clock }

	a, b := &fakeWarmSession{}, &fakeWarmSession{}
	ea := mustAcquire(t, c, "a", a)
	ea.Turn.Lock() // 'a' is mid-turn

	clock = clock.Add(time.Second)
	mustAcquire(t, c, "b", b) // over cap, but 'a' is busy -> must be skipped
	if a.isClosed() {
		t.Fatal("a busy (mid-turn) session must not be evicted")
	}

	ea.Turn.Unlock()
	c.Reap() // 'a' is now idle-but-over-cap and free to evict
	if !a.isClosed() {
		t.Fatal("after its turn released, the over-cap LRU should reap")
	}
}

// TestUnit_WarmCache_IdleTTLReaps proves sessions idle past the TTL are closed.
func TestUnit_WarmCache_IdleTTLReaps(t *testing.T) {
	withWarmCacheConfig(t, 10, 5*time.Minute)

	c := NewWarmCache[*fakeWarmSession]()
	clock := time.Now()
	c.now = func() time.Time { return clock }

	a := &fakeWarmSession{}
	mustAcquire(t, c, "a", a)

	clock = clock.Add(6 * time.Minute)
	c.Reap()
	if !a.isClosed() {
		t.Fatal("session idle past the TTL should be reaped and closed")
	}
}

// TestUnit_WarmCache_DropEvictsAndCloses proves the fatal-error path removes and
// closes the entry, and a later acquire reopens.
func TestUnit_WarmCache_DropEvictsAndCloses(t *testing.T) {
	withWarmCacheConfig(t, 2, 0)

	c := NewWarmCache[*fakeWarmSession]()
	a := &fakeWarmSession{}
	ea := mustAcquire(t, c, "a", a)
	c.Drop(ea)
	if !a.isClosed() {
		t.Fatal("Drop should close the session")
	}
	reopened := false
	if _, err := c.Acquire("a", func() (*fakeWarmSession, error) { reopened = true; return &fakeWarmSession{}, nil }); err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if !reopened {
		t.Fatal("after Drop, the next acquire must reopen")
	}
}
