package openvino

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

// These tests exercise the pure-Go pool eviction policy. They run in the default
// (untagged) build against the no-op ovsession stub, so they need neither the
// native OpenVINO libraries nor a model.

func resetGenAISessionPool(t *testing.T) {
	t.Helper()
	require.NoError(t, ShutdownGenAISessions())
	idleTTL := genAISessionIdleTTL
	maxResid := genAISessionMaxResid
	t.Cleanup(func() {
		_ = ShutdownGenAISessions()
		genAISessionIdleTTL = idleTTL
		genAISessionMaxResid = maxResid
	})
}

func putPoolEntry(key string, refs int, lastUsed time.Time) {
	genAISessions.Lock()
	genAISessions.entries[key] = &pooledGenAISession{
		session:  &ovsession.GenAISession{},
		refs:     refs,
		lastUsed: lastUsed,
	}
	genAISessions.Unlock()
}

func TestUnit_OpenVINOPool_IdleReapByTTL(t *testing.T) {
	resetGenAISessionPool(t)
	genAISessionIdleTTL = time.Minute
	genAISessionMaxResid = 0 // disable the cap so only TTL is under test

	now := time.Now()
	putPoolEntry("idle-stale", 0, now.Add(-2*time.Minute))   // past TTL -> reaped
	putPoolEntry("idle-fresh", 0, now)                       // within TTL -> kept
	putPoolEntry("in-use-stale", 1, now.Add(-2*time.Minute)) // refs>0 -> kept despite age

	sweepIdleGenAISessions()

	entries, refs := genAISessionPoolStatsForTest()
	require.Equal(t, 2, entries, "stale idle session should be reaped")
	require.Equal(t, 1, refs)
	genAISessions.Lock()
	_, staleKept := genAISessions.entries["idle-stale"]
	_, freshKept := genAISessions.entries["idle-fresh"]
	_, inUseKept := genAISessions.entries["in-use-stale"]
	genAISessions.Unlock()
	require.False(t, staleKept)
	require.True(t, freshKept)
	require.True(t, inUseKept)
}

func TestUnit_OpenVINOPool_CapEvictsLRUIdle(t *testing.T) {
	resetGenAISessionPool(t)
	genAISessionIdleTTL = 0 // disable TTL so only the cap is under test
	genAISessionMaxResid = 1

	now := time.Now()
	putPoolEntry("idle-old", 0, now.Add(-time.Hour)) // LRU idle -> evicted
	putPoolEntry("idle-new", 0, now)                 // most-recent idle -> kept

	sweepIdleGenAISessions()

	entries, _ := genAISessionPoolStatsForTest()
	require.Equal(t, 1, entries, "cap should evict down to the resident limit")
	genAISessions.Lock()
	_, oldKept := genAISessions.entries["idle-old"]
	_, newKept := genAISessions.entries["idle-new"]
	genAISessions.Unlock()
	require.False(t, oldKept, "LRU idle entry should be evicted first")
	require.True(t, newKept)
}

func TestUnit_OpenVINOPool_CapNeverEvictsInUse(t *testing.T) {
	resetGenAISessionPool(t)
	genAISessionIdleTTL = 0
	genAISessionMaxResid = 1

	now := time.Now()
	putPoolEntry("busy-a", 1, now.Add(-time.Hour))
	putPoolEntry("busy-b", 2, now)

	sweepIdleGenAISessions()

	entries, refs := genAISessionPoolStatsForTest()
	require.Equal(t, 2, entries, "in-use sessions must not be evicted even over the cap")
	require.Equal(t, 3, refs)
}

func TestUnit_OpenVINOPool_ShutdownEmptiesPool(t *testing.T) {
	resetGenAISessionPool(t)

	now := time.Now()
	putPoolEntry("a", 0, now)
	putPoolEntry("b", 1, now)

	require.NoError(t, ShutdownGenAISessions())

	entries, refs := genAISessionPoolStatsForTest()
	require.Equal(t, 0, entries)
	require.Equal(t, 0, refs)
}

func TestUnit_OpenVINOSessionKey_IncludesModelDigest(t *testing.T) {
	cfg := normalizedPoolConfig(ovsession.GenAIConfig{Device: "GPU"})

	a := genAISessionKey("/models/qwen", "digest-a", cfg)
	b := genAISessionKey("/models/qwen", "digest-b", cfg)
	aAgain := genAISessionKey("/models/qwen", "digest-a", cfg)

	require.Equal(t, a, aAgain)
	require.NotEqual(t, a, b, "replacing a model in place must not reuse the old pooled session")
}
