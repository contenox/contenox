//go:build llamanode && llamacpp_direct

package llama

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	modeldllama "github.com/contenox/runtime/modeld/llama"
	_ "github.com/contenox/runtime/modeld/llama/llamasession" // registers the CGO session factory
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestSystem_WarmCacheSnapshotSurvivesRealModelSwap is the capstone real-hardware
// proof for the modeld snapshot wiring: it drives the actual runtime WarmCache
// (runtime/modelrepo.WarmCache) against a real CGO llama.cpp modeld.Service on
// two distinct real GGUF models sharing the single-resident-slot cache (the
// production default). Swapping A out for B evicts A's session and — via the
// snapshot wiring under test — captures its real llama.cpp KV state to disk;
// swapping back re-opens A cold in modeld but the warm cache restores the real
// KV before returning. The proof is that EnsurePrefix reports full warm reuse
// (not a cold prefill) and greedy decoding continues byte-identically to a
// pre-eviction baseline.
//
// Run (from repo root, after make build-llamacpp-runtime):
//
//	CONTENOX_LLAMA_TINY_GGUF=~/.contenox/models/llama/qwen2.5-coder-0.5b/model.gguf \
//	CONTENOX_LLAMA_SWAP_GGUF=~/.contenox/models/llama/granite-3.2-2b/model.gguf \
//	go test -tags 'llamanode llamacpp_direct' -run TestSystem_WarmCacheSnapshotSurvivesRealModelSwap \
//	  -v -timeout 5m ./runtime/modelrepo/llama
func TestSystem_WarmCacheSnapshotSurvivesRealModelSwap(t *testing.T) {
	modelA := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	modelB := os.Getenv("CONTENOX_LLAMA_SWAP_GGUF")
	if modelA == "" || modelB == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF and CONTENOX_LLAMA_SWAP_GGUF to two distinct small GGUF models")
	}

	serveRealLlamaModeldSnapshot(t)

	stable, suffix := "system\n", "def add(a, b):\n"
	manifestA := swapTestManifest("swap-a", "swap-digest-a", stable, suffix)
	manifestB := swapTestManifest("swap-b", "swap-digest-b", stable, suffix)

	cfg := Config{NumCtx: 256, NumBatch: 32, NumThreads: 2, DisableBOS: true}
	clientA := &client{modelName: "swap-a", modelPath: modelA, modelDigest: "swap-digest-a", cfg: cfg}
	clientB := &client{modelName: "swap-b", modelPath: modelB, modelDigest: "swap-digest-b", cfg: cfg}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Baseline: open A cold, prefill, and decode a real greedy continuation.
	csA, err := clientA.acquire()
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	csA.Turn.Lock()
	statusA, err := csA.Sess.EnsurePrefix(ctx, PrefixInput{Text: stable, Manifest: manifestA})
	if err != nil {
		csA.Turn.Unlock()
		t.Fatalf("EnsurePrefix A: %v", err)
	}
	if statusA.ReusedTokens != 0 {
		csA.Turn.Unlock()
		t.Fatalf("expected a genuinely cold first open (reused=0), got %+v", statusA)
	}
	if _, err := csA.Sess.PrefillSuffix(ctx, SuffixInput{Text: suffix, Manifest: manifestA}); err != nil {
		csA.Turn.Unlock()
		t.Fatalf("PrefillSuffix A: %v", err)
	}
	baseline, err := decodeGreedy(ctx, csA.Sess, 12)
	csA.Turn.Unlock()
	if err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	t.Logf("baseline continuation: %q", baseline)
	if strings.TrimSpace(baseline) == "" {
		t.Fatal("baseline produced no tokens")
	}

	// 2. Swap to B. The single-resident-slot cache evicts A, which must capture
	// A's real KV snapshot to disk before closing it.
	csB, err := clientB.acquire()
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	csB.Turn.Lock()
	if _, err := csB.Sess.EnsurePrefix(ctx, PrefixInput{Text: stable, Manifest: manifestB}); err != nil {
		csB.Turn.Unlock()
		t.Fatalf("EnsurePrefix B: %v", err)
	}
	csB.Turn.Unlock()

	// Verify persistence on disk independent of the live cache: an unrelated
	// DiskSnapshotStore instance, rooted at the same directory, must already see
	// A's captured blob.
	keyA := sessionCacheKey(clientA.ref(), normalizeConfig(clientA.cfg))
	independentStore := modelrepo.NewDiskSnapshotStore(func() string { return modeldconn.SnapshotDir("llama") }, 0, 0)
	if _, ok := independentStore.Load(keyA); !ok {
		t.Fatal("expected A's real snapshot to be persisted to disk after eviction")
	}

	// 3. Swap back to A. This evicts B in turn and reopens A cold in modeld
	// (a brand-new llama.cpp session with empty KV) — but the warm cache must
	// restore A's captured snapshot into it before returning.
	csA2, err := clientA.acquire()
	if err != nil {
		t.Fatalf("reacquire A: %v", err)
	}
	csA2.Turn.Lock()
	defer csA2.Turn.Unlock()

	statusA2, err := csA2.Sess.EnsurePrefix(ctx, PrefixInput{Text: stable, Manifest: manifestA})
	if err != nil {
		t.Fatalf("EnsurePrefix A after restore: %v", err)
	}
	if statusA2.PrefilledTokens != 0 || statusA2.ReusedTokens != statusA.PrefilledTokens {
		t.Fatalf("expected A to reopen warm via the restored snapshot (reused=%d, prefilled=0), got %+v",
			statusA.PrefilledTokens, statusA2)
	}

	if _, err := csA2.Sess.PrefillSuffix(ctx, SuffixInput{Text: suffix, Manifest: manifestA}); err != nil {
		t.Fatalf("PrefillSuffix A after restore: %v", err)
	}
	continuation, err := decodeGreedy(ctx, csA2.Sess, 12)
	if err != nil {
		t.Fatalf("decode after restore: %v", err)
	}
	t.Logf("restored continuation: %q", continuation)
	if continuation != baseline {
		t.Fatalf("expected byte-identical continuation after snapshot restore\n baseline: %q\n restored: %q", baseline, continuation)
	}
}

// TestSystem_WarmCacheSnapshotSurvivesRealOneShotExit is the real-hardware proof
// for the one-shot `contenox chat` durability path: a single process opens a
// model, uses it (never switching, so never evicting), then flushes on graceful
// exit via the same modelrepo.Shutdown() hook the CLI drives. A simulated restart
// (drop the in-process cache; the daemon + on-disk snapshot survive) reopens the
// model cold in modeld but restores the real llama.cpp KV, and greedy decoding
// continues byte-identically to the pre-restart baseline.
//
// Run: CONTENOX_LLAMA_TINY_GGUF=... go test -tags 'llamanode llamacpp_direct' \
//
//	-run TestSystem_WarmCacheSnapshotSurvivesRealOneShotExit -v -timeout 5m ./runtime/modelrepo/llama
func TestSystem_WarmCacheSnapshotSurvivesRealOneShotExit(t *testing.T) {
	model := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if model == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}
	serveRealLlamaModeldSnapshot(t)

	stable, suffix := "system\n", "def add(a, b):\n"
	manifest := swapTestManifest("oneshot", "oneshot-digest", stable, suffix)
	cfg := Config{NumCtx: 256, NumBatch: 32, NumThreads: 2, DisableBOS: true}
	c := &client{modelName: "oneshot", modelPath: model, modelDigest: "oneshot-digest", cfg: cfg}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// "First process": one turn, no model switch -> nothing is ever evicted.
	cs, err := c.acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	cs.Turn.Lock()
	status, err := cs.Sess.EnsurePrefix(ctx, PrefixInput{Text: stable, Manifest: manifest})
	if err != nil {
		cs.Turn.Unlock()
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if status.ReusedTokens != 0 {
		cs.Turn.Unlock()
		t.Fatalf("first open should be cold, got %+v", status)
	}
	if _, err := cs.Sess.PrefillSuffix(ctx, SuffixInput{Text: suffix, Manifest: manifest}); err != nil {
		cs.Turn.Unlock()
		t.Fatalf("PrefillSuffix: %v", err)
	}
	baseline, err := decodeGreedy(ctx, cs.Sess, 12)
	cs.Turn.Unlock()
	if err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	t.Logf("baseline: %q", baseline)

	// Graceful exit flush (the CLI's modelrepo.Shutdown() path), then simulate the
	// process boundary by dropping the in-process cache.
	if err := modelrepo.Shutdown(); err != nil {
		t.Fatalf("shutdown flush: %v", err)
	}
	keyOneShot := sessionCacheKey(c.ref(), normalizeConfig(c.cfg))
	independentStore := modelrepo.NewDiskSnapshotStore(func() string { return modeldconn.SnapshotDir("llama") }, 0, 0)
	if _, ok := independentStore.Load(keyOneShot); !ok {
		t.Fatal("exit flush must persist the never-evicted resident session to disk")
	}
	closeCachedSessionsForTest()

	// "Second process": reopen cold in modeld, restore the real KV, continue.
	cs2, err := c.acquire()
	if err != nil {
		t.Fatalf("reacquire after restart: %v", err)
	}
	cs2.Turn.Lock()
	defer cs2.Turn.Unlock()
	status2, err := cs2.Sess.EnsurePrefix(ctx, PrefixInput{Text: stable, Manifest: manifest})
	if err != nil {
		t.Fatalf("EnsurePrefix after restart: %v", err)
	}
	if status2.PrefilledTokens != 0 || status2.ReusedTokens != status.PrefilledTokens {
		t.Fatalf("expected warm reopen from the exit-flushed snapshot (reused=%d, prefilled=0), got %+v", status.PrefilledTokens, status2)
	}
	if _, err := cs2.Sess.PrefillSuffix(ctx, SuffixInput{Text: suffix, Manifest: manifest}); err != nil {
		t.Fatalf("PrefillSuffix after restart: %v", err)
	}
	continuation, err := decodeGreedy(ctx, cs2.Sess, 12)
	if err != nil {
		t.Fatalf("decode after restart: %v", err)
	}
	t.Logf("restored: %q", continuation)
	if continuation != baseline {
		t.Fatalf("expected byte-identical continuation after one-shot-exit restore\n baseline: %q\n restored: %q", baseline, continuation)
	}
}

func decodeGreedy(ctx context.Context, sess Session, maxTokens int) (string, error) {
	temp := 0.0
	seed := 7
	chunks, err := sess.Decode(ctx, DecodeConfig{MaxTokens: maxTokens, Temperature: &temp, Seed: &seed})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			return "", chunk.Error
		}
		b.WriteString(chunk.Text)
	}
	return b.String(), nil
}

func swapTestManifest(profileID, modelDigest, stable, suffix string) ContextManifest {
	return ContextManifest{
		ProfileID:            profileID,
		Backend:              "llamacpp",
		BackendVersion:       "test",
		ModelDigest:          modelDigest,
		PromptFormat:         "chatml",
		PromptTemplateDigest: "test-template",
		RuntimeDigest:        "test-runtime",
		AddBOS:               false,
		StableBytes:          len(stable),
		TotalBytes:           len(stable) + len(suffix),
		StableByteHash:       hashString(stable),
		Segments: []ManifestSegment{
			{Kind: "system", Stable: true, ByteStart: 0, ByteEnd: len(stable), ByteHash: hashString(stable)},
			{Kind: "user", Stable: false, ByteStart: len(stable), ByteEnd: len(stable) + len(suffix), ByteHash: hashString(suffix)},
		},
	}
}

// serveRealLlamaModeldSnapshot serves a real CGO llama.cpp modeld.Service over a
// real gRPC listener, points modeldconn at a fresh temp data root (so the lease
// probe and the on-disk snapshot directory are test-isolated), and pins the warm
// cache to the single-resident-slot production default so a second model
// genuinely evicts the first.
func serveRealLlamaModeldSnapshot(t *testing.T) {
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
	lease, err := liblease.Acquire(leasePath, 60*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint}))
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
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldllama.NewService(), rec.InstanceID, "llama") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
}
