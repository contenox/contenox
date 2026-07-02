package llama

import (
	"errors"
	"strings"
	"testing"
)

func TestUnit_ExplainOverflowPrefersLiveNumCtxFromError(t *testing.T) {
	c := &client{
		modelName:                 "qwen3-4b",
		deviceKind:                "gpu",
		freeBytes:                 2 << 30,
		describedEffectiveContext: 433, // stale describe answer (encumbered snapshot)
	}
	live := NewContextOverflowError("suffix", 123, 7042, 3854)
	wrapped := c.explainOverflow(live)
	if !errors.Is(wrapped, ErrContextOverflow) {
		t.Fatalf("wrapped error lost ErrContextOverflow identity: %v", wrapped)
	}
	msg := wrapped.Error()
	if !strings.Contains(msg, "serves only 3854 context tokens") {
		t.Fatalf("message should quote the live session window, got: %s", msg)
	}
	if strings.Contains(msg, "serves only 433") {
		t.Fatalf("message quoted the stale describe answer: %s", msg)
	}
}

func TestUnit_ExplainOverflowFallsBackToDescribeAnswer(t *testing.T) {
	c := &client{modelName: "m", describedEffectiveContext: 2048}
	wrapped := c.explainOverflow(errorsJoinOverflowNoCtx())
	if !strings.Contains(wrapped.Error(), "serves only 2048 context tokens") {
		t.Fatalf("expected describe fallback in message, got: %s", wrapped.Error())
	}
}

// an overflow error with no num_ctx= token in its text
func errorsJoinOverflowNoCtx() error {
	return ErrContextOverflow
}
