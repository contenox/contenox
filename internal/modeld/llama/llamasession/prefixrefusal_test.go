//go:build llamanode && llamacpp_direct

package llamasession

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/llama"
)

// TestUnit_EnsurePrefix_RefusedStableRenderServesUncached pins the template
// refusal contract: a model template may legally reject the isolated stable
// render - Qwen 3.5 raises "No user query found in messages." for a message list
// with no user turn, which is exactly what a system-only stable prefix is.
// Refusal must cost prefix reuse, never the turn: PrefillSuffix renders the whole
// conversation and reconciles at the token level, so the answer does not depend
// on the isolated render.
func TestUnit_EnsurePrefix_RefusedStableRenderServesUncached(t *testing.T) {
	restore := renderStableTemplate
	renderStableTemplate = func(*session, []chatTemplateMessage, string) (string, error) {
		return "", errors.New("No user query found in messages.")
	}
	t.Cleanup(func() { renderStableTemplate = restore })

	s := bareSession()
	stable := "system\nYou are concise.\n"
	m := tinyManifest(stable, "user\nhello\n")

	status, err := s.EnsurePrefix(context.Background(), llama.PrefixInput{Text: stable, Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix on a refused stable render: %v", err)
	}
	if status.PrefixTokens != 0 || status.ReusedTokens != 0 {
		t.Fatalf("refused render must report no prefix tape, got %+v", status)
	}
	if s.closed {
		t.Fatal("a refused stable render must not poison the session")
	}
	if len(s.stableMsgs) != 1 || s.stableMsgs[0].Role != "system" {
		t.Fatalf("stable messages not adopted, got %+v", s.stableMsgs)
	}
	if s.stableText != stable {
		t.Fatalf("stable text not adopted, got %q", s.stableText)
	}
}

// qwen35ModelPath returns a Qwen 3.5 GGUF or skips. It is the generation whose
// template refuses an isolated system-only render (so EnsurePrefix has no prefix
// tape) and whose autoparser switches tool-call dialect on typed tool schemas.
// Precursor Qwen 3.0 templates carry the same multi-step scan without the raise.
func qwen35ModelPath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("CONTENOX_LLAMA_QWEN35_GGUF")
	if p == "" {
		t.Skip("set CONTENOX_LLAMA_QWEN35_GGUF to a Qwen 3.5 GGUF to run this test")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

// stableHistoryManifest marks the tail of the stable region as a user turn: the
// shape a request carries when the runtime asserts conversation history stable,
// and the only shape in which the whole-prefix render succeeds while a
// per-segment render does not.
func stableHistoryManifest(stable, history, suffix string) llama.ContextManifest {
	text := stable + history
	return llama.ContextManifest{
		ProfileID:            "guard-test",
		Backend:              "llamacpp",
		BackendVersion:       "test",
		ModelDigest:          "guard",
		PromptFormat:         "chatml",
		PromptTemplateDigest: "test-template",
		RuntimeDigest:        "test-runtime",
		StableBytes:          len(text),
		TotalBytes:           len(text) + len(suffix),
		StableByteHash:       shaHex(text),
		Segments: []llama.ManifestSegment{
			{Kind: "system", Stable: true, ByteStart: 0, ByteEnd: len(stable), ByteHash: shaHex(stable)},
			{Kind: "user", Stable: true, ByteStart: len(stable), ByteEnd: len(text), ByteHash: shaHex(history)},
			{Kind: "user", Stable: false, ByteStart: len(text), ByteEnd: len(text) + len(suffix), ByteHash: shaHex(suffix)},
		},
	}
}

// TestUnit_EnrichStableSegments_RefusedFragmentStopsRefinement pins the advisory
// side of the refusal contract: a template that renders the whole stable prefix
// but refuses its first fragment leaves the segment token ranges unset instead of
// failing the call. Residency reports the missing ranges; the turn does not
// depend on them.
func TestUnit_EnrichStableSegments_RefusedFragmentStopsRefinement(t *testing.T) {
	restore := renderStableTemplate
	renderStableTemplate = func(*session, []chatTemplateMessage, string) (string, error) {
		return "", errors.New("No user query found in messages.")
	}
	t.Cleanup(func() { renderStableTemplate = restore })

	s := bareSession()
	stable, history := "system\nBe concise.\n", "user\nMy name is Naro.\n"
	m := stableHistoryManifest(stable, history, "user\nWhat is my name?\n")
	stableMsgs := stableMessages(stable+history, m)
	if len(stableMsgs) != 2 {
		t.Fatalf("stable messages = %d, want 2", len(stableMsgs))
	}

	out := s.enrichStableSegments(m, stableMsgs, []int{1, 2, 3, 4, 5}, "")
	if out.StableTokenHash == "" {
		t.Fatal("the stable tape hash is known even when segment ranges are not")
	}
	if out.Segments[0].TokenStart != 0 || out.Segments[0].TokenEnd != 0 {
		t.Fatalf("a refused fragment must leave ranges unset, got %+v", out.Segments[0])
	}
}

// TestSystem_LlamaSession_GuardTemplateSystemOnlyPrefix is the live guard for
// the refusal contract: every ACP turn sends a system-first request, so
// EnsurePrefix renders a system-only list, and the turn must complete with reuse
// reported unavailable instead of failing. The second turn repeats it against a
// resident tape that has no matching prefix tape, which is what the wipe path
// exists for.
func TestSystem_LlamaSession_GuardTemplateSystemOnlyPrefix(t *testing.T) {
	modelPath := qwen35ModelPath(t)

	sess, err := New(modelPath, llama.Config{NumCtx: 2048, NumBatch: 64, NumThreads: 4, DisableBOS: true, ReasoningFormat: "deepseek"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	stable := "system\nYou are a helpful assistant.\n"
	thinkOff := false
	for i, turn := range []string{
		"user\nSay the word hello and nothing else.\n",
		"user\nNow say the word world.\n",
	} {
		m := tinyManifest(stable, turn)
		status, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m})
		if err != nil {
			t.Fatalf("turn %d EnsurePrefix: %v", i+1, err)
		}
		if status.PrefixTokens != 0 {
			t.Fatalf("turn %d: a refused stable render leaves no prefix tape, got %+v", i+1, status)
		}
		if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: turn, Manifest: m, EnableThinking: &thinkOff}); err != nil {
			t.Fatalf("turn %d PrefillSuffix: %v", i+1, err)
		}
		if got := drainDecode(t, sess, ctx, 16); strings.TrimSpace(got) == "" {
			t.Fatalf("turn %d produced no answer text: %q", i+1, got)
		} else {
			t.Logf("turn %d output: %q", i+1, got)
		}
	}
}

// TestSystem_LlamaSession_GuardTemplateStableHistorySegmentRanges is the live
// guard for the second refusal site: with conversation history marked stable, the
// whole-prefix render succeeds, so the turn keeps its prefix tape and its reuse,
// while the per-segment render is refused and only the segment ranges are lost.
// This is the shape that killed a chain's on_failure handler after the primary
// task had already failed.
func TestSystem_LlamaSession_GuardTemplateStableHistorySegmentRanges(t *testing.T) {
	modelPath := qwen35ModelPath(t)

	sess, err := New(modelPath, llama.Config{NumCtx: 2048, NumBatch: 64, NumThreads: 4, DisableBOS: true, ReasoningFormat: "deepseek"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	thinkOff := false
	stable := "system\nYou are a helpful assistant.\n"
	history := "user\nMy name is Naro.\n"
	turn := "user\nWhat is my name?\n"
	m := stableHistoryManifest(stable, history, turn)

	status, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable + history, Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix with a renderable stable history: %v", err)
	}
	if status.PrefixTokens == 0 {
		t.Fatalf("a renderable stable region keeps its prefix tape, got %+v", status)
	}
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: turn, Manifest: m, EnableThinking: &thinkOff}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	if got := drainDecode(t, sess, ctx, 16); strings.TrimSpace(got) == "" {
		t.Fatalf("turn produced no answer text: %q", got)
	} else {
		t.Logf("output: %q", got)
	}
	if report := sess.ExplainContext(); report.Residency == nil || report.Residency.Error == "" {
		t.Fatalf("unrefined segment ranges must surface as a residency diagnostic, got %+v", report.Residency)
	}
}
