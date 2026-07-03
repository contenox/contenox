package llama

import (
	"errors"
	"strings"
	"testing"
)

// explainOverflow turns a typed context-overflow error into an actionable message
// while staying errors.Is(ErrContextOverflow)-true so existing handling works.
func TestUnit_ExplainOverflow_EnrichesContextOverflow(t *testing.T) {
	c := &client{
		modelName:  "qwen3-8b",
		cfg:        Config{NumCtx: 400},
		deviceKind: "cuda",
		freeBytes:  6 << 30, // 6 GiB
	}
	got := c.explainOverflow(NewContextOverflowError("suffix", 1, 461, 400))
	if !errors.Is(got, ErrContextOverflow) {
		t.Fatalf("wrapped error must stay ErrContextOverflow: %v", got)
	}
	msg := got.Error()
	for _, want := range []string{"context overflow", "qwen3-8b", "400 context tokens", "cuda", "6.0 GiB", "smaller model"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("overflow message missing %q:\n%s", want, msg)
		}
	}
}

func TestUnit_ExplainOverflow_PassThroughNonOverflow(t *testing.T) {
	c := &client{modelName: "qwen3-8b", cfg: Config{NumCtx: 400}}
	other := errors.New("some downstream failure")
	if got := c.explainOverflow(other); got != other {
		t.Fatalf("non-overflow error must pass through unchanged, got %v", got)
	}
	if c.explainOverflow(nil) != nil {
		t.Fatal("nil must pass through")
	}
}

func TestUnit_ExplainOverflow_FallsBackWithoutCapacityFacts(t *testing.T) {
	c := &client{modelName: "qwen3-8b", cfg: Config{NumCtx: 400}}
	got := c.explainOverflow(NewContextOverflowError("suffix", 1, 461, 400))
	if !errors.Is(got, ErrContextOverflow) {
		t.Fatalf("must stay ErrContextOverflow: %v", got)
	}
	if !strings.Contains(got.Error(), "this device") {
		t.Fatalf("expected generic device phrasing when modeld facts are absent:\n%s", got.Error())
	}
}
