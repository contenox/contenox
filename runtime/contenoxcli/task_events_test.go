package contenoxcli

import (
	"bytes"
	"context"
	"testing"
	"time"

	libbus "github.com/contenox/contenox/libbus"
	"github.com/contenox/contenox/runtime/hitlservice"
	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_EngineWatchTaskEvents_RequestScoped(t *testing.T) {
	bus := libbus.NewInMem()
	engine := &Engine{Bus: bus}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan taskengine.TaskEvent, 4)
	_, err := engine.WatchTaskEvents(ctx, "req-1", events)
	require.NoError(t, err)

	sink := taskengine.NewBusTaskEventSink(bus)
	require.NoError(t, sink.PublishTaskEvent(context.Background(), taskengine.TaskEvent{
		Kind:      taskengine.TaskEventStepChunk,
		RequestID: "req-2",
		Content:   "ignored",
	}))
	require.NoError(t, sink.PublishTaskEvent(context.Background(), taskengine.TaskEvent{
		Kind:      taskengine.TaskEventStepChunk,
		RequestID: "req-1",
		Content:   "hello",
	}))

	select {
	case event := <-events:
		require.Equal(t, "req-1", event.RequestID)
		require.Equal(t, "hello", event.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task event")
	}

	select {
	case event := <-events:
		t.Fatalf("unexpected extra event: %+v", event)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestUnit_PrintIdleHintIfStale_SuppressedWhileApprovalPending(t *testing.T) {
	if localtools.IsApprovalPending() {
		t.Fatalf("approvalPending leaked from a prior test")
	}

	gate := make(chan struct{})
	released := make(chan struct{})

	wrapper := localtools.NewHITLWrapper(
		fakeInnerRepo{},
		func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			<-gate
			return true, nil
		},
		fakePolicy{action: hitlservice.ActionApprove},
		nil,
	)

	go func() {
		_, _, _ = wrapper.Exec(context.Background(), time.Now(), map[string]any{}, false, &taskengine.ToolsCall{Name: "fake", ToolName: "fake"})
		close(released)
	}()

	deadline := time.Now().Add(time.Second)
	for !localtools.IsApprovalPending() {
		if time.Now().After(deadline) {
			t.Fatalf("wrapper never set approvalPending")
		}
		time.Sleep(2 * time.Millisecond)
	}

	var buf bytes.Buffer
	r := &cliTaskEventRenderer{
		w:            &buf,
		lastStreamAt: time.Now().Add(-30 * time.Second),
	}
	r.printIdleHintIfStale()
	assert.Empty(t, buf.String(), "no hint must be printed while approval is pending")

	close(gate)
	<-released
	assert.False(t, localtools.IsApprovalPending(), "wrapper must clear approvalPending after ask returns")
}

func TestUnit_PrintIdleHintIfStale_PrintsWhenIdle(t *testing.T) {
	if localtools.IsApprovalPending() {
		t.Fatalf("approvalPending leaked from a prior test")
	}
	var buf bytes.Buffer
	r := &cliTaskEventRenderer{
		w:            &buf,
		lastStreamAt: time.Now().Add(-30 * time.Second),
	}
	r.printIdleHintIfStale()
	assert.Contains(t, buf.String(), "still working")
}

type fakePolicy struct {
	action hitlservice.Action
}

func (f fakePolicy) Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (hitlservice.EvaluationResult, error) {
	return hitlservice.EvaluationResult{Action: f.action}, nil
}

type fakeInnerRepo struct{}

func (fakeInnerRepo) Exec(ctx context.Context, startTime time.Time, input any, debug bool, tools *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	return "ok", taskengine.DataTypeString, nil
}
func (fakeInnerRepo) Supports(ctx context.Context) ([]string, error) { return []string{"fake"}, nil }
func (fakeInnerRepo) GetSchemasForSupportedTools(ctx context.Context) (map[string]*openapi3.T, error) {
	return map[string]*openapi3.T{}, nil
}
func (fakeInnerRepo) GetToolsForToolsByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	return nil, nil
}
