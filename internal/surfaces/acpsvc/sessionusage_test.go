package acpsvc

import (
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/stretchr/testify/require"
)

func testEvent(kind taskengine.TaskEventKind, handler string, ts time.Time, usage ...*taskengine.TokenUsage) taskengine.TaskEvent {
	var u *taskengine.TokenUsage
	if len(usage) > 0 {
		u = usage[0]
	}
	return taskengine.TaskEvent{
		Kind:         kind,
		Timestamp:    ts,
		TaskHandler:  handler,
		ModelName:    "gemini-test",
		ProviderType: "vertex-google",
		Usage:        u,
	}
}

func TestUnit_SessionUsage_ReducesOrderedEngineEvents(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	u := NewSessionUsage("sess-1")

	// Turn 1: one model call with a measured first chunk.
	events := []taskengine.TaskEvent{
		testEvent(taskengine.TaskEventChainStarted, "", base),
		testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(100*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepChunk, "chat_completion", base.Add(200*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepChunk, "chat_completion", base.Add(300*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepStreamEnd, "chat_completion", base.Add(2*time.Second+100*time.Millisecond), &taskengine.TokenUsage{
			Prompt: 1000, Completion: 200, Thinking: 50, Total: 1200, CacheRead: 800, CacheWrite: 10,
		}),
		testEvent(taskengine.TaskEventStepCompleted, "chat_completion", base.Add(2*time.Second+150*time.Millisecond), nil),
		// Turn 1 continues: a tool-execution step running two tools.
		testEvent(taskengine.TaskEventStepStarted, "execute_tool_calls", base.Add(2*time.Second+200*time.Millisecond), nil),
		testEvent(taskengine.TaskEventToolCallPending, "execute_tool_calls", base.Add(2*time.Second+300*time.Millisecond), nil),
		testEvent(taskengine.TaskEventToolCallPending, "execute_tool_calls", base.Add(2*time.Second+400*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepCompleted, "execute_tool_calls", base.Add(3*time.Second), nil),
		testEvent(taskengine.TaskEventChainCompleted, "", base.Add(3*time.Second+10*time.Millisecond), nil),
		// Turn 2: a model call whose stream ends before any chunk (no TTFT sample).
		testEvent(taskengine.TaskEventChainStarted, "", base.Add(3*time.Second+100*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(3*time.Second+200*time.Millisecond), nil),
		testEvent(taskengine.TaskEventStepStreamEnd, "chat_completion", base.Add(3*time.Second+400*time.Millisecond), &taskengine.TokenUsage{
			Prompt: 50, Completion: 5, Total: 55,
		}),
		testEvent(taskengine.TaskEventStepCompleted, "chat_completion", base.Add(3*time.Second+410*time.Millisecond), nil),
		testEvent(taskengine.TaskEventChainCompleted, "", base.Add(3*time.Second+420*time.Millisecond), nil),
	}
	for _, ev := range events {
		u.Observe(ev)
	}

	require.Equal(t, int64(2), u.Turns, "one chain run per user prompt")
	require.Equal(t, int64(2), u.Steps, "two tool executions")
	require.Equal(t, int64(2), u.ModelCalls)
	require.Equal(t, int64(2000+200), u.LlmMs, "wall time inside the two model calls")
	require.Equal(t, int64(800), u.ToolMs, "wall time inside the tool-execution step")
	require.Equal(t, int64(1), u.TtftSamples, "only the call with a first chunk yields a sample")
	require.Equal(t, int64(100), u.TtftMs, "first chunk 100ms after step start")
	require.Equal(t, int64(1900), u.StreamMs, "output-stream time excludes TTFT; the second call had no first chunk")

	require.Equal(t, int64(1050), u.InputTokens, "prompt totals include cached tokens, provider-reported")
	require.Equal(t, int64(205), u.OutputTokens)
	require.EqualValues(t, 50, u.ThinkingTokens)
	require.EqualValues(t, 50, u.Snapshot().ThinkingTokens)
	require.EqualValues(t, 50, u.Snapshot().ByModel[0].ThinkingTokens)
	require.Equal(t, int64(800), u.CacheReadTokens)
	require.Equal(t, int64(10), u.CacheWriteTokens)

	mu, ok := u.ByModel["gemini-test"]
	require.True(t, ok)
	require.Equal(t, "vertex-google", mu.Provider)
	require.Equal(t, int64(2), mu.Calls)
	require.Equal(t, int64(1050), mu.InputTokens)
	require.Equal(t, int64(800), mu.CacheReadTokens)
}

func TestUnit_SessionUsage_StreamFailureLeavesNoOpenState(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	u := NewSessionUsage("sess-2")

	u.Observe(testEvent(taskengine.TaskEventChainStarted, "", base))
	u.Observe(testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(100*time.Millisecond), nil))
	// A mid-stream failure emits no step_stream_end; the step failure must
	// not leave the reducer waiting on an open model call.
	u.Observe(testEvent(taskengine.TaskEventStepFailed, "chat_completion", base.Add(300*time.Millisecond), nil))

	require.Zero(t, u.ModelCalls, "a failed stream contributes no model call")
	require.Zero(t, u.TtftSamples)

	// A subsequent clean call is measured against its own step, not the dead one.
	u.Observe(testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(400*time.Millisecond), nil))
	u.Observe(testEvent(taskengine.TaskEventStepChunk, "chat_completion", base.Add(450*time.Millisecond), nil))
	u.Observe(testEvent(taskengine.TaskEventStepStreamEnd, "chat_completion", base.Add(700*time.Millisecond), &taskengine.TokenUsage{Prompt: 10, Completion: 1, Total: 11}))
	require.Equal(t, int64(1), u.ModelCalls)
	require.Equal(t, int64(50), u.TtftMs)
	require.Equal(t, int64(1), u.TtftSamples)
}

func TestUnit_SessionUsage_SnapshotSortsByModelAndCarriesTokens(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	u := NewSessionUsage("sess-3")
	ev := func(m string, prompt int) taskengine.TaskEvent {
		e := testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(100*time.Millisecond))
		e.ModelName = m
		return e
	}
	u.Observe(ev("b-model", 0))
	for _, m := range []string{"b-model", "a-model"} {
		u.Observe(testEvent(taskengine.TaskEventStepStarted, "chat_completion", base.Add(100*time.Millisecond), nil))
		s := testEvent(taskengine.TaskEventStepStreamEnd, "chat_completion", base.Add(200*time.Millisecond), &taskengine.TokenUsage{Prompt: 100, Completion: 10, Total: 110, CacheRead: 80})
		s.ModelName = m
		s.ProviderType = "vertex-google"
		u.Observe(s)
	}
	snap := u.Snapshot()
	require.Equal(t, "sess-3", snap.SessionID)
	require.Equal(t, int64(200), snap.InputTokens)
	require.Equal(t, int64(160), snap.CacheReadTokens)
	require.Len(t, snap.ByModel, 2)
	require.Equal(t, "a-model", snap.ByModel[0].Model, "snapshot must be sorted by model")
	require.Equal(t, "b-model", snap.ByModel[1].Model)
	require.Equal(t, int64(100), snap.ByModel[0].InputTokens)
	require.Equal(t, "vertex-google", snap.ByModel[0].Provider)
}
