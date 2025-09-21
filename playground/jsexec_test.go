package playground_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/contenox/runtime/eventstore"
	"github.com/contenox/runtime/functionstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/playground"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystem_GojaExecutor(t *testing.T) {
	ctx := context.Background()

	// Setup playground with all required services
	p := playground.New()
	p.WithPostgresTestContainer(ctx).
		WithNats(ctx).
		WithRuntimeState(ctx, false).
		WithMockTokenizer().
		WithLLMRepo().
		WithMockHookRegistry().
		WithEventSourceInit(ctx).
		WithFunctionService(ctx).
		WithEventDispatcher(ctx, func(err error) {
			if err != nil {
				t.Logf("Event dispatcher error: %v", err)
			}
		}, time.Millisecond*5).
		WithEventSourceService(ctx).
		WithActivityTracker(libtracker.NewLogActivityTracker(slog.Default())).
		WithGojaExecutor(ctx).
		StartGojaExecutorSync(ctx, 100*time.Millisecond)

	require.NoError(t, p.GetError())
	defer p.CleanUp()

	// Get the function service to create test functions
	functionService, err := p.GetFunctionService()
	require.NoError(t, err)

	// Get the executor
	exec, err := p.GetGojaExecutor()
	require.NoError(t, err)

	t.Run("ExecuteSimpleFunction", func(t *testing.T) {
		// Create a simple test function
		simpleFunction := &functionstore.Function{
			Name:       "testSimple",
			ScriptType: "goja",
			Script:     `function testSimple(event) { return { message: "Hello " + event.data.name }; }`,
		}

		err := functionService.CreateFunction(ctx, simpleFunction)
		require.NoError(t, err)

		exec.TriggerSync()
		time.Sleep(time.Millisecond * 100)
		// Create test event
		eventData := map[string]interface{}{"name": "World"}
		dataBytes, err := json.Marshal(eventData)
		require.NoError(t, err)

		event := &eventstore.Event{
			ID:            "test-event-1",
			EventType:     "test.event",
			AggregateType: "test",
			AggregateID:   "123",
			Version:       1,
			Data:          dataBytes,
			CreatedAt:     time.Now().UTC(),
		}

		// Execute the function
		result, err := exec.ExecuteFunction(ctx, simpleFunction.Script, "testSimple", event)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Hello World", result["message"])
	})

	t.Run("ExecuteFunctionWithEventSending", func(t *testing.T) {
		// Create a function that sends events
		eventSendingFunction := &functionstore.Function{
			Name: "testEventSending",
			Script: `
				function testEventSending(event) {
					// Send a new event
					var sendResult = sendEvent("response.event", {
						originalEventId: event.id,
						processedAt: new Date().toISOString()
					});

					return {
						eventSent: sendResult.success,
						newEventId: sendResult.event_id
					};
				}
			`,
			ScriptType: "goja",
		}

		err := functionService.CreateFunction(ctx, eventSendingFunction)
		require.NoError(t, err)

		// Wait a bit for sync to pick up the function
		time.Sleep(200 * time.Millisecond)

		// Create test event
		eventData := map[string]interface{}{"value": 42}
		dataBytes, err := json.Marshal(eventData)
		require.NoError(t, err)

		event := &eventstore.Event{
			ID:            "test-event-3",
			EventType:     "test.event",
			AggregateType: "test",
			AggregateID:   "789",
			Version:       1,
			Data:          dataBytes,
			CreatedAt:     time.Now().UTC(),
		}

		// Execute the function
		result, err := exec.ExecuteFunction(ctx, eventSendingFunction.Script, "testEventSending", event)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, true, result["eventSent"])
		assert.NotEmpty(t, result["newEventId"])
	})

	t.Run("FunctionErrorHandling", func(t *testing.T) {
		// Create a function that will cause an error
		errorFunction := &functionstore.Function{
			Name: "testError",
			Script: `
				function testError(event) {
					// This will cause a reference error
					return undefinedVariable;
				}
			`,
			ScriptType: "goja",
		}

		err := functionService.CreateFunction(ctx, errorFunction)
		require.NoError(t, err)
		exec.TriggerSync()
		// Wait a bit for sync to pick up the function
		time.Sleep(100 * time.Millisecond)

		// Create test event
		eventData := map[string]interface{}{"value": "test"}
		dataBytes, err := json.Marshal(eventData)
		require.NoError(t, err)

		event := &eventstore.Event{
			ID:            "test-event-4",
			EventType:     "test.event",
			AggregateType: "test",
			AggregateID:   "101",
			Version:       1,
			Data:          dataBytes,
			CreatedAt:     time.Now().UTC(),
		}

		// Execute the function - this should return an error
		result, err := exec.ExecuteFunction(ctx, errorFunction.Script, "testError", event)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "undefinedVariable")
	})
}
