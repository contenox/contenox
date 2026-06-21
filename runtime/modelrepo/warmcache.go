package modelrepo

import (
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

// WarmCache is a bounded, idle-reaped cache of warm backend sessions keyed by a
// model+config identity. It evicts by idle TTL and a max-resident cap (LRU),
// never evicting a session that is mid-turn, and closes evicted handles so the
// modeld slot can be switched or unloaded. Construct with NewWarmCache.
type WarmCache[S WarmSession] struct {
	mu  sync.Mutex
	m   map[string]*WarmEntry[S]
	now func() time.Time
}

// NewWarmCache returns an empty cache.
func NewWarmCache[S WarmSession]() *WarmCache[S] {
	return &WarmCache[S]{m: map[string]*WarmEntry[S]{}, now: time.Now}
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
	closeAll(preVictims)

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
	c.m[key] = e
	victims := c.selectVictimsLocked(e, 0)
	c.mu.Unlock()

	closeAll(victims)
	return e, nil
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
	closeAll(victims)
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

func closeAll[S WarmSession](victims []*WarmEntry[S]) {
	for _, e := range victims {
		_ = e.Sess.Close()
	}
}
