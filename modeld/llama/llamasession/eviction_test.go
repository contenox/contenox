//go:build llamanode && llamacpp_direct

package llamasession

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/contenox/runtime/modeld/llama"
	"github.com/contenox/runtime/modeld/residency"
)

// TestSystem_LlamaSessionEvictRange_SlidingWindowStaysDecodable exercises the
// real StreamingLLM move on the live llama.cpp backend: prefill past a window,
// drop a middle token range (keeping the stable prefix as sinks and a recent
// tail), and confirm the KV is still coherent enough to keep decoding. It checks
// the mechanism — resident bookkeeping and that the shifted KV stays decodable —
// not exact text, which a tiny quantized model cannot guarantee.
func TestSystem_LlamaSessionEvictRange_SlidingWindowStaysDecodable(t *testing.T) {
	modelPath := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	requireTinyGGUF(t, modelPath)

	sess, err := New(modelPath, llama.Config{NumCtx: 256, NumBatch: 32, NumThreads: 1, DisableBOS: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stable := "system\nYou are a helpful assistant.\n"
	suffix := "user\nCount slowly: one two three four five six seven eight nine ten eleven twelve.\n"
	m := tinyManifest(stable, suffix)

	if _, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	prefixTokens := sess.ExplainContext().PrefixTokens
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: suffix, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	before := sess.ExplainContext().ResidentTokens

	exec, ok := sess.(residency.Executor)
	if !ok {
		t.Fatal("llama session does not implement residency.Executor")
	}
	if caps := exec.Capabilities(); !caps.RemoveMiddle || !caps.PositionShift {
		t.Fatalf("expected RemoveMiddle + PositionShift, got %+v", caps)
	}

	// Keep the stable prefix (sinks) plus the last 3 tokens (recent window); drop
	// the middle.
	evictStart := prefixTokens + 1
	evictEnd := before - 3
	if evictEnd <= evictStart {
		t.Skipf("not enough volatile tokens to exercise eviction: resident=%d prefix=%d", before, prefixTokens)
	}
	width := evictEnd - evictStart

	if err := exec.EvictRange(ctx, residency.Range{Start: evictStart, End: evictEnd}); err != nil {
		t.Fatalf("EvictRange: %v", err)
	}

	after := sess.ExplainContext().ResidentTokens
	if after != before-width {
		t.Fatalf("resident after eviction = %d, want %d (before %d - width %d)", after, before-width, before, width)
	}

	// The shifted KV must remain decodable: generation continues with no fatal
	// session and produces at least one new token.
	chunks, err := sess.Decode(ctx, llama.DecodeConfig{MaxTokens: 4})
	if err != nil {
		t.Fatalf("Decode after eviction: %v", err)
	}
	got := ""
	for c := range chunks {
		if c.Error != nil {
			t.Fatalf("decode stream error after eviction: %v", c.Error)
		}
		got += c.Text
	}
	if grown := sess.ExplainContext().ResidentTokens; grown <= after {
		t.Skipf("tiny model emitted no new token after eviction (resident stayed %d, text %q)", after, got)
	}
}

func TestSystem_LlamaSessionEvictAdmitTail_RestoresContinuation(t *testing.T) {
	modelPath := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	requireTinyGGUF(t, modelPath)

	cfg := llama.Config{
		NumCtx:                  256,
		PlannerEffectiveContext: 384,
		NumBatch:                32,
		NumThreads:              1,
		DisableBOS:              true,
	}
	sess, err := New(modelPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	stable := "system\nYou are a concise assistant.\n"
	suffix := "user\nRepeat the final word from this list: alpha beta gamma delta epsilon.\n"
	m := tinyManifest(stable, suffix)
	if _, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: suffix, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	snap, err := sess.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := decodeOne(ctx, sess)
	if err != nil {
		t.Fatal(err)
	}
	if want == "" {
		t.Skip("tiny model produced no visible token for the reference continuation")
	}

	cold, err := New(modelPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cold.Close()
	if err := cold.Restore(ctx, snap); err != nil {
		t.Fatal(err)
	}
	exec, ok := cold.(residency.Executor)
	if !ok {
		t.Fatal("llama session does not implement residency.Executor")
	}
	if caps := exec.Capabilities(); !caps.ColdStore {
		t.Fatalf("expected ColdStore capability with planner context > num_ctx, got %+v", caps)
	}

	before := cold.ExplainContext().ResidentTokens
	const width = 4
	if before <= width {
		t.Skipf("not enough resident tokens to evict tail: resident=%d width=%d", before, width)
	}
	r := residency.Range{Start: before - width, End: before}
	if err := exec.EvictRange(ctx, r); err != nil {
		t.Fatalf("EvictRange: %v", err)
	}
	if got := cold.ExplainContext().ResidentTokens; got != before-width {
		t.Fatalf("resident after evict = %d, want %d", got, before-width)
	}
	if err := exec.AdmitRange(ctx, r); err != nil {
		t.Fatalf("AdmitRange: %v", err)
	}
	if got := cold.ExplainContext().ResidentTokens; got != before {
		t.Fatalf("resident after admit = %d, want restored %d", got, before)
	}
	got, err := decodeOne(ctx, cold)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("cold-restored continuation %q != reference %q", got, want)
	}
}

// TestSystem_LlamaSessionDecode_SlidesPastNumCtx generates far more tokens than
// the context window holds. Before sliding, decode would overflow at num_ctx;
// with it, the window slides (prefix sinks + recent tail) so generation
// continues. It always asserts the safety invariants — no overflow, resident
// stays within num_ctx — and, when the tiny model generates enough to cross the
// window, confirms it produced more than the initial free room.
func TestSystem_LlamaSessionDecode_SlidesPastNumCtx(t *testing.T) {
	modelPath := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	requireTinyGGUF(t, modelPath)

	const numCtx = 48
	sess, err := New(modelPath, llama.Config{NumCtx: numCtx, NumBatch: 32, NumThreads: 1, DisableBOS: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stable := "system\nYou write long numbered lists and never stop early.\n"
	suffix := "user\nList numbers one per line starting at one.\n"
	m := tinyManifest(stable, suffix)
	if _, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: suffix, Manifest: m}); err != nil {
		t.Fatal(err)
	}
	roomAtStart := numCtx - sess.ExplainContext().ResidentTokens

	chunks, err := sess.Decode(ctx, llama.DecodeConfig{MaxTokens: numCtx * 4})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	produced := 0
	for c := range chunks {
		if c.Error != nil {
			t.Fatalf("decode stream error — sliding must prevent overflow past num_ctx: %v", c.Error)
		}
		if c.Text != "" {
			produced++
		}
	}

	if resident := sess.ExplainContext().ResidentTokens; resident > numCtx {
		t.Fatalf("resident %d exceeded num_ctx %d; sliding must keep it bounded", resident, numCtx)
	}
	if produced <= roomAtStart {
		t.Skipf("tiny model stopped before crossing the window (produced=%d, room=%d)", produced, roomAtStart)
	}
	t.Logf("generated ~%d visible tokens past a %d-token window (initial room %d)", produced, numCtx, roomAtStart)
}
