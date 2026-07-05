package llama

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestE2E_WarmCacheSnapshotSurvivesModelSwapAndRuntimeRestart is the full-stack,
// deterministic proof for the modeld snapshot wiring: the runtime warm cache
// (runtime/modelrepo.WarmCache), talking over the real gRPC transport to modeld's
// noop MemoryService, captures a session's KV to an on-disk store when it evicts
// it to free the single modeld slot for another model, and restores it warm when
// the model is swapped back in. It also proves the disk store's role
// specifically: a snapshot written to disk is readable by an independent store
// instance rooted at the same directory, so the persisted state does not depend
// on any process-lifetime cache and survives a runtime restart.
//
// No CGO: the memory service models the warm-reuse contract exactly (see
// transport.MemoryService), so this isolates the runtime<->modeld snapshot
// wiring from real inference, mirroring TestE2E_RuntimeLlamaDialsModeld.
func TestE2E_WarmCacheSnapshotSurvivesModelSwapAndRuntimeRestart(t *testing.T) {
	setupModeldE2E(t)

	clientA := &client{modelName: "model-a", modelPath: "/models/a/model.gguf", modelDigest: "digest-a", cfg: Config{NumCtx: 100}}
	clientB := &client{modelName: "model-b", modelPath: "/models/b/model.gguf", modelDigest: "digest-b", cfg: Config{NumCtx: 100}}

	manifestA := ContextManifest{
		Backend:              "llama",
		ModelDigest:          "digest-a",
		PromptFormat:         "chatml",
		PromptTemplateDigest: "t1",
		RuntimeDigest:        "r1",
		StableByteHash:       contextasm.HashString("hello"),
	}
	ctx := context.Background()

	// 1. Open A, establish a stable prefix (5 "tokens": tokenProxy is one per
	// rune for the memory service), then release the turn so it is evictable.
	csA, err := clientA.acquire()
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	csA.Turn.Lock()
	statusA, err := csA.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA})
	csA.Turn.Unlock()
	if err != nil {
		t.Fatalf("EnsurePrefix A: %v", err)
	}
	if statusA.PrefilledTokens != 5 || statusA.ReusedTokens != 0 {
		t.Fatalf("first EnsurePrefix on A should be a cold prefill, got %+v", statusA)
	}

	// 2. Swap to B. WarmCacheMaxResident defaults to 1 (single modeld slot), so
	// this evicts A. Eviction must capture A's snapshot to disk before closing it.
	csB, err := clientB.acquire()
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	csB.Turn.Lock()
	if _, err := csB.Sess.EnsurePrefix(ctx, PrefixInput{Text: "other model prompt", Manifest: ContextManifest{
		Backend: "llama", ModelDigest: "digest-b", PromptFormat: "chatml", PromptTemplateDigest: "t1", RuntimeDigest: "r1",
		StableByteHash: contextasm.HashString("other model prompt"),
	}}); err != nil {
		csB.Turn.Unlock()
		t.Fatalf("EnsurePrefix B: %v", err)
	}
	csB.Turn.Unlock()

	// Verify persistence independently of the live warm cache: a fresh
	// DiskSnapshotStore instance, rooted at the same directory, must already see
	// A's captured blob. This is the "survives a runtime restart" property — the
	// bytes exist on disk, not just in an in-process cache.
	keyA := sessionCacheKey(clientA.ref(), normalizeConfig(clientA.cfg))
	independentStore := modelrepo.NewDiskSnapshotStore(func() string { return modeldconn.SnapshotDir("llama") }, 0, 0)
	if _, ok := independentStore.Load(keyA); !ok {
		t.Fatal("expected A's snapshot to be persisted to disk after eviction, independent of the live warm cache")
	}

	// 3. Swap back to A. This evicts B (capturing it in turn) and reopens A on a
	// brand-new, cold memory-service session — but the warm cache must restore
	// A's captured snapshot into it before returning.
	csA2, err := clientA.acquire()
	if err != nil {
		t.Fatalf("reacquire A: %v", err)
	}
	csA2.Turn.Lock()
	defer csA2.Turn.Unlock()

	// The decisive warm-vs-cold signal: EnsurePrefix with the exact same stable
	// text/manifest as before eviction. A genuinely cold session would prefill
	// all 5 tokens again; a restored session reuses them.
	statusA2, err := csA2.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA})
	if err != nil {
		t.Fatalf("EnsurePrefix A after restore: %v", err)
	}
	if statusA2.ReusedTokens != 5 || statusA2.PrefilledTokens != 0 {
		t.Fatalf("expected A to reopen warm via the restored snapshot (reused=5, prefilled=0), got %+v — snapshot restore did not take effect", statusA2)
	}
}

// TestE2E_WarmCacheSnapshotSurvivesOneShotExit is the proof for the dominant
// `contenox chat` path: a single-turn process never swaps models and never idle-
// reaps, so eviction-time capture alone would persist nothing. The graceful-exit
// flush (modelrepo.Shutdown -> WarmCache.CaptureResident, wired from the llama
// package init and invoked by contenoxcli.Main) captures the resident session at
// process exit; the next process restores it warm. This drives the real runtime
// warm cache and the real gRPC transport, simulating the process boundary with a
// flush + closeCachedSessionsForTest (which drops the in-process cache exactly as
// a process exit would) while the on-disk snapshot survives.
func TestE2E_WarmCacheSnapshotSurvivesOneShotExit(t *testing.T) {
	setupModeldE2E(t)

	clientA := &client{modelName: "model-a", modelPath: "/models/a/model.gguf", modelDigest: "digest-a", cfg: Config{NumCtx: 100}}
	manifestA := ContextManifest{
		Backend: "llama", ModelDigest: "digest-a", PromptFormat: "chatml", PromptTemplateDigest: "t1", RuntimeDigest: "r1",
		StableByteHash: contextasm.HashString("hello"),
	}
	ctx := context.Background()

	// "First process": one turn, model never switched -> nothing is evicted.
	csA, err := clientA.acquire()
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	csA.Turn.Lock()
	statusA, err := csA.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA})
	csA.Turn.Unlock()
	if err != nil {
		t.Fatalf("EnsurePrefix A: %v", err)
	}
	if statusA.PrefilledTokens != 5 || statusA.ReusedTokens != 0 {
		t.Fatalf("first turn should be a cold prefill, got %+v", statusA)
	}

	// Graceful exit: the shutdown hook flushes resident sessions to disk. Invoke
	// the same registry the CLI drives so this exercises the real wiring.
	if err := modelrepo.Shutdown(); err != nil {
		t.Fatalf("shutdown flush: %v", err)
	}
	keyA := sessionCacheKey(clientA.ref(), normalizeConfig(clientA.cfg))
	independentStore := modelrepo.NewDiskSnapshotStore(func() string { return modeldconn.SnapshotDir("llama") }, 0, 0)
	if _, ok := independentStore.Load(keyA); !ok {
		t.Fatal("exit flush must persist the resident (never-evicted) session to disk")
	}

	// "Process restart": drop the in-process warm cache; the daemon + disk survive.
	closeCachedSessionsForTest()

	csA2, err := clientA.acquire()
	if err != nil {
		t.Fatalf("reacquire A after restart: %v", err)
	}
	csA2.Turn.Lock()
	defer csA2.Turn.Unlock()
	statusA2, err := csA2.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA})
	if err != nil {
		t.Fatalf("EnsurePrefix A after restart: %v", err)
	}
	if statusA2.ReusedTokens != 5 || statusA2.PrefilledTokens != 0 {
		t.Fatalf("expected warm reopen from the exit-flushed snapshot (reused=5, prefilled=0), got %+v", statusA2)
	}
}

// TestE2E_WarmCacheSnapshotDisabledStaysCold is the control for the test above:
// with snapshot survival explicitly disabled (mirrors an operator setting
// CONTENOX_WARM_SNAPSHOT_DISABLE), the exact same swap-out/swap-back sequence
// must reopen cold. This proves the warm restore in the sibling test is actually
// caused by the snapshot wiring, not an artifact of the memory service or the
// warm cache's own bookkeeping.
func TestE2E_WarmCacheSnapshotDisabledStaysCold(t *testing.T) {
	setupModeldE2E(t)
	t.Setenv("CONTENOX_WARM_SNAPSHOT_DISABLE", "1")

	clientA := &client{modelName: "model-a", modelPath: "/models/a/model.gguf", modelDigest: "digest-a", cfg: Config{NumCtx: 100}}
	clientB := &client{modelName: "model-b", modelPath: "/models/b/model.gguf", modelDigest: "digest-b", cfg: Config{NumCtx: 100}}
	manifestA := ContextManifest{
		Backend: "llama", ModelDigest: "digest-a", PromptFormat: "chatml", PromptTemplateDigest: "t1", RuntimeDigest: "r1",
		StableByteHash: contextasm.HashString("hello"),
	}
	ctx := context.Background()

	csA, err := clientA.acquire()
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	csA.Turn.Lock()
	if _, err := csA.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA}); err != nil {
		csA.Turn.Unlock()
		t.Fatalf("EnsurePrefix A: %v", err)
	}
	csA.Turn.Unlock()

	csB, err := clientB.acquire() // evicts A; snapshot survival is disabled, so no capture happens
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	csB.Turn.Lock()
	csB.Turn.Unlock()

	csA2, err := clientA.acquire()
	if err != nil {
		t.Fatalf("reacquire A: %v", err)
	}
	csA2.Turn.Lock()
	defer csA2.Turn.Unlock()

	statusA2, err := csA2.Sess.EnsurePrefix(ctx, PrefixInput{Text: "hello", Manifest: manifestA})
	if err != nil {
		t.Fatalf("EnsurePrefix A after cold reopen: %v", err)
	}
	if statusA2.PrefilledTokens != 5 || statusA2.ReusedTokens != 0 {
		t.Fatalf("expected a cold reopen with snapshot survival disabled (prefilled=5, reused=0), got %+v", statusA2)
	}
}

// setupModeldE2E stands up an in-process modeld MemoryService over a real gRPC
// listener, points modeldconn at a fresh temp data root (so both the lease probe
// and the snapshot directory are test-isolated), and clears the package's global
// warm cache before and after the test so no state leaks across tests sharing the
// same test binary. Returns the temp data root.
func setupModeldE2E(t *testing.T) string {
	t.Helper()
	withWarmCacheSingleSlot(t)
	closeCachedSessionsForTest()
	t.Cleanup(closeCachedSessionsForTest)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()

	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 30*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint, "backend": "llama"}))
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		t.Fatalf("inspect lease: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, transport.NewMemoryService(), rec.InstanceID, "llama") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
	return dataRoot
}

// withWarmCacheSingleSlot pins the warm cache to the single-resident-slot
// default (matching a real modeld daemon) for the duration of the test.
func withWarmCacheSingleSlot(t *testing.T) {
	t.Helper()
	origMax, origTTL := modelrepo.WarmCacheMaxResident, modelrepo.WarmCacheIdleTTL
	modelrepo.WarmCacheMaxResident, modelrepo.WarmCacheIdleTTL = 1, 0
	t.Cleanup(func() { modelrepo.WarmCacheMaxResident, modelrepo.WarmCacheIdleTTL = origMax, origTTL })
}
