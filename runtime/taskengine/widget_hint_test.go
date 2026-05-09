package taskengine_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/contenox/contenox/runtime/taskengine"
)

func TestUnit_WidgetHintSink_AppendDrain(t *testing.T) {
	s := &taskengine.WidgetHintSink{}
	s.Append(taskengine.WidgetHint{Kind: "file_view", Payload: json.RawMessage(`{"a":1}`)})
	s.Append(taskengine.WidgetHint{Kind: "terminal_excerpt"})
	got := s.Drain()
	if len(got) != 2 {
		t.Fatalf("drain returned %d hints, want 2", len(got))
	}
	if got[0].Kind != "file_view" || got[1].Kind != "terminal_excerpt" {
		t.Fatalf("unexpected order: %+v", got)
	}
	// Drain clears.
	if again := s.Drain(); len(again) != 0 {
		t.Fatalf("second drain returned %d, want 0", len(again))
	}
}

func TestUnit_WidgetHintSink_NilSafe(t *testing.T) {
	var s *taskengine.WidgetHintSink
	s.Append(taskengine.WidgetHint{Kind: "x"})
	if got := s.Drain(); got != nil {
		t.Fatalf("nil sink Drain should return nil")
	}
	if got := s.Snapshot(); got != nil {
		t.Fatalf("nil sink Snapshot should return nil")
	}
}

func TestUnit_AppendWidgetHint_NoSinkIsNoOp(t *testing.T) {
	// No sink in context — must not panic, must not cost anything observable.
	taskengine.AppendWidgetHint(context.Background(), taskengine.WidgetHint{Kind: "x"})
	taskengine.AppendWidgetHintTyped(context.Background(), "x", map[string]any{"a": 1})
}

func TestUnit_AppendWidgetHint_WithSink(t *testing.T) {
	s := &taskengine.WidgetHintSink{}
	ctx := taskengine.WithWidgetHintSink(context.Background(), s)
	taskengine.AppendWidgetHint(ctx, taskengine.WidgetHint{Kind: "file_view"})
	taskengine.AppendWidgetHintTyped(ctx, "terminal_excerpt", map[string]any{"output": "hi"})
	got := s.Snapshot()
	if len(got) != 2 {
		t.Fatalf("got %d hints, want 2", len(got))
	}
	if got[1].Kind != "terminal_excerpt" {
		t.Fatalf("kind = %q", got[1].Kind)
	}
	var p struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(got[1].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Output != "hi" {
		t.Fatalf("decoded output = %q", p.Output)
	}
}

func TestUnit_AppendWidgetHint_ConcurrentAppend(t *testing.T) {
	s := &taskengine.WidgetHintSink{}
	ctx := taskengine.WithWidgetHintSink(context.Background(), s)
	const N = 100
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			taskengine.AppendWidgetHint(ctx, taskengine.WidgetHint{Kind: "x"})
		}()
	}
	wg.Wait()
	if got := len(s.Drain()); got != N {
		t.Fatalf("got %d hints, want %d", got, N)
	}
}

func TestUnit_WithWidgetHintSink_NilNoop(t *testing.T) {
	ctx := taskengine.WithWidgetHintSink(context.Background(), nil)
	// Should round-trip the original ctx, so AppendWidgetHint is a no-op.
	taskengine.AppendWidgetHint(ctx, taskengine.WidgetHint{Kind: "x"})
}
