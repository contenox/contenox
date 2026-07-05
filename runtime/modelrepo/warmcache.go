package modelrepo

import (
	"context"
	"sync"
	"time"
)

// Tunables for the warm session cache (package vars so tests can override). They
// bound how many local backend sessions stay resident at once: each cached
// session keeps a handle to modeld's active slot, so local modeld uses a default
// cap of one resident slot. An unbounded cache prevents switching and can keep
// stale handles alive across modeld owner changes.
var (
	WarmCacheMaxResident = 1
	WarmCacheIdleTTL     = 5 * time.Minute
)

// WarmSession is the minimal contract the cache needs: closing a session releases
// the cached handle and lets modeld switch or unload the active slot.
type WarmSession interface{ Close() error }

// WarmEntry is one resident session kept warm across turns. Turn serializes a
// whole EnsurePrefix -> PrefillSuffix -> Decode sequence so concurrent requests
// on the same session do not corrupt its resident KV. Hold Turn for the duration
// of a turn; the cache will not evict an entry whose Turn is held.
type WarmEntry[S WarmSession] struct {
	Sess S
	Turn sync.Mutex

	key      string
	lastUsed time.Time
}

// snapshotHooks is the optional durable-warm-KV wiring for a WarmCache. When set,
// the cache captures a session's snapshot just before it evicts (closes) the
// handle, persists it to Store keyed by the cache key, and restores it into a
// freshly opened session on the next miss. This lets a stable prefix's KV survive
// a model swap on the single modeld slot (and, via a durable Store, a runtime
// restart): the reopened session is warm instead of cold. Restore is a pure
// optimization — capture and restore failures fall back to a cold prefill and can
// never corrupt resident state.
type snapshotHooks[S WarmSession] struct {
	// capture serializes the session's current snapshot to an opaque blob.
	capture func(context.Context, S) ([]byte, error)
	// restore applies a blob previously produced by capture to a fresh session.
	restore func(context.Context, S, []byte) error
	store   SnapshotStore
}

// WarmCache is a bounded, idle-reaped cache of warm backend sessions keyed by a
// model+config identity. It evicts by idle TTL and a max-resident cap (LRU),
// never evicting a session that is mid-turn, and closes evicted handles so the
// modeld slot can be switched or unloaded. Construct with NewWarmCache, or
// NewWarmCacheWithSnapshots to make evicted warm KV survive the swap.
type WarmCache[S WarmSession] struct {
	mu   sync.Mutex
	m    map[string]*WarmEntry[S]
	now  func() time.Time
	snap *snapshotHooks[S]
}

// NewWarmCache returns an empty cache with no snapshot survival.
func NewWarmCache[S WarmSession]() *WarmCache[S] {
	return &WarmCache[S]{m: map[string]*WarmEntry[S]{}, now: time.Now}
}

// NewWarmCacheWithSnapshots returns a cache that captures an evicted session's
// snapshot to store and restores it into the reopened session on the next
// acquire. capture and restore bridge the backend session's Snapshot/Restore to
// opaque blobs; store persists them by cache key. All three must be non-nil.
func NewWarmCacheWithSnapshots[S WarmSession](
	store SnapshotStore,
	capture func(context.Context, S) ([]byte, error),
	restore func(context.Context, S, []byte) error,
) *WarmCache[S] {
	c := NewWarmCache[S]()
	if store != nil && capture != nil && restore != nil {
		c.snap = &snapshotHooks[S]{capture: capture, restore: restore, store: store}
	}
	return c
}

// Acquire returns the warm entry for key, opening one via open on a miss. The
// caller must Turn.Lock() the returned entry for the duration of a turn.
//
// On a miss it first evicts (and closes) enough idle/over-cap sessions to leave
// room for the one about to open — BEFORE calling open — so a single-slot backend
// (modeld holds exactly one active model) frees the slot before the new session
// claims it. Opening first and trimming after would make the new open race a
// still-resident handle and fail with "slot busy". A post-admit reap stays as a
// backstop for entries that were mid-turn during the pre-open pass.
func (c *WarmCache[S]) Acquire(key string, open func() (S, error)) (*WarmEntry[S], error) {
	c.mu.Lock()
	if e, ok := c.m[key]; ok {
		e.lastUsed = c.now()
		c.mu.Unlock()
		return e, nil
	}
	c.mu.Unlock()

	c.mu.Lock()
	preVictims := c.selectVictimsLocked(nil, 1) // reserve a slot for the open below
	c.mu.Unlock()
	c.closeVictims(preVictims)

	s, err := open()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if e, ok := c.m[key]; ok { // lost the open race: reuse the winner, drop ours
		c.mu.Unlock()
		_ = s.Close()
		return e, nil
	}
	e := &WarmEntry[S]{Sess: s, key: key, lastUsed: c.now()}
	if c.snap != nil {
		// Hold Turn for the restore below so a concurrent caller that receives
		// this same entry from the map blocks on its own Turn.Lock() until the
		// restore finishes, instead of racing a partially-restored session.
		e.Turn.Lock()
	}
	c.m[key] = e
	victims := c.selectVictimsLocked(e, 0)
	c.mu.Unlock()

	c.closeVictims(victims)

	if c.snap != nil {
		c.tryRestore(e)
		e.Turn.Unlock()
	}
	return e, nil
}

// tryRestore applies a previously captured snapshot to a freshly opened session,
// if one exists for e's key. A missing, corrupt, or incompatible snapshot is not
// an error here: it is deleted (if present) and the caller proceeds with the
// cold session exactly as if no snapshot wiring existed.
func (c *WarmCache[S]) tryRestore(e *WarmEntry[S]) {
	blob, ok := c.snap.store.Load(e.key)
	if !ok {
		return
	}
	ctx := context.Background()
	if SnapshotTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, SnapshotTimeout)
		defer cancel()
	}
	if err := c.snap.restore(ctx, e.Sess, blob); err != nil {
		c.snap.store.Delete(e.key)
	}
}

// capture persists a session's snapshot under its key so a later re-acquire can
// restore instead of cold-prefilling. Best-effort: a capture error, an empty
// blob (the capture bridge returns one when snapshot survival is disabled), or a
// blob over SnapshotMaxBlobBytes just means the next open is cold, same as
// today. The caller must exclusively hold e (its Turn), since capture reads the
// live session.
func (c *WarmCache[S]) capture(e *WarmEntry[S]) {
	ctx := context.Background()
	if SnapshotTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, SnapshotTimeout)
		defer cancel()
	}
	blob, err := c.snap.capture(ctx, e.Sess)
	if err != nil || len(blob) == 0 {
		return
	}
	if SnapshotMaxBlobBytes > 0 && int64(len(blob)) > SnapshotMaxBlobBytes {
		return
	}
	c.snap.store.Save(e.key, blob)
}

// CaptureResident snapshots every currently-resident session to the store without
// closing it. It is the graceful-shutdown/exit hook: a one-shot CLI process (and
// a long-running server on restart) never evicts a still-in-use model, so
// eviction-time capture alone would never persist the hot session — this flushes
// it so the next process restores warm. Mid-turn sessions are skipped (their KV
// is being mutated); their state is captured on a later exit or eviction. It is a
// no-op when snapshot survival is not configured.
func (c *WarmCache[S]) CaptureResident() {
	if c.snap == nil {
		return
	}
	c.mu.Lock()
	entries := make([]*WarmEntry[S], 0, len(c.m))
	for _, e := range c.m {
		entries = append(entries, e)
	}
	c.mu.Unlock()
	for _, e := range entries {
		if !e.Turn.TryLock() {
			continue // mid-turn: skip, capture it on a later exit/eviction
		}
		c.capture(e)
		e.Turn.Unlock()
	}
}

// Drop evicts an entry whose session became unusable (closed/stale/fatal) so the
// next call reopens. Safe to call with a stale entry already replaced in the map.
func (c *WarmCache[S]) Drop(e *WarmEntry[S]) {
	if e == nil {
		return
	}
	c.mu.Lock()
	if c.m[e.key] == e {
		delete(c.m, e.key)
	}
	c.mu.Unlock()
	_ = e.Sess.Close()
}

// Reap closes idle-past-TTL sessions and trims the cache down to the resident
// cap, never touching mid-turn sessions. Acquire reaps automatically; this is
// exported for callers (and tests) that want to force it.
func (c *WarmCache[S]) Reap() {
	c.mu.Lock()
	victims := c.selectVictimsLocked(nil, 0)
	c.mu.Unlock()
	c.closeVictims(victims)
}

// Clear evicts and closes every session (test cleanup / shutdown).
func (c *WarmCache[S]) Clear() {
	c.mu.Lock()
	m := c.m
	c.m = map[string]*WarmEntry[S]{}
	c.mu.Unlock()
	for _, e := range m {
		_ = e.Sess.Close()
	}
}

// selectVictimsLocked removes idle-past-TTL and over-cap (LRU) entries from the
// map and returns them for closing outside the lock. keep (may be nil) is never
// evicted — it is the just-acquired entry. reserve is how many not-yet-admitted
// sessions to make room for, so the cap is enforced against len(c.m)+reserve:
// pass reserve=1 before opening a new session so room is freed up front. A
// victim's Turn is left locked so a concurrent holder can never resurrect it; the
// entry is discarded afterward.
func (c *WarmCache[S]) selectVictimsLocked(keep *WarmEntry[S], reserve int) []*WarmEntry[S] {
	var victims []*WarmEntry[S]
	now := c.now()

	if WarmCacheIdleTTL > 0 {
		for k, e := range c.m {
			if e == keep {
				continue
			}
			if now.Sub(e.lastUsed) >= WarmCacheIdleTTL && e.Turn.TryLock() {
				delete(c.m, k)
				victims = append(victims, e)
			}
		}
	}

	// Trim to the resident cap by least-recently-used, skipping mid-turn entries.
	skip := map[*WarmEntry[S]]bool{}
	for WarmCacheMaxResident > 0 && len(c.m)+reserve > WarmCacheMaxResident {
		var lru *WarmEntry[S]
		var lruKey string
		for k, e := range c.m {
			if e == keep || skip[e] {
				continue
			}
			if lru == nil || e.lastUsed.Before(lru.lastUsed) {
				lru, lruKey = e, k
			}
		}
		if lru == nil {
			break // nothing evictable (only keep / busy entries remain)
		}
		if !lru.Turn.TryLock() {
			skip[lru] = true // busy: leave resident, reap when idle
			continue
		}
		delete(c.m, lruKey)
		victims = append(victims, lru)
	}
	return victims
}

// closeVictims closes each evicted entry, first capturing its snapshot (when
// snapshot survival is configured) so the key can be restored warm later. Each
// victim's Turn is already locked by selectVictimsLocked, so the capture and
// Close race no concurrent user of the session.
func (c *WarmCache[S]) closeVictims(victims []*WarmEntry[S]) {
	for _, e := range victims {
		if c.snap != nil {
			c.capture(e)
		}
		_ = e.Sess.Close()
	}
}
