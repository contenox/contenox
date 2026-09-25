package taskengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/kernel/taskengine/llmretry"
	"github.com/contenox/contenox/internal/kernel/tools"
	"github.com/contenox/contenox/internal/models/llmrepo"
	libmodelprovider "github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

func errorParcelStream(err error) func(context.Context, llmrepo.Request, []libmodelprovider.Message, ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
	return func(context.Context, llmrepo.Request, []libmodelprovider.Message, ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
		ch := make(chan *libmodelprovider.StreamParcel, 1)
		ch <- &libmodelprovider.StreamParcel{Error: err}
		close(ch)
		return ch, llmrepo.Meta{ModelName: "test-model", ProviderType: "vertex-google"}, nil
	}
}

func recoveredStream(text string) func(context.Context, llmrepo.Request, []libmodelprovider.Message, ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
	return func(context.Context, llmrepo.Request, []libmodelprovider.Message, ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
		ch := make(chan *libmodelprovider.StreamParcel, 2)
		ch <- &libmodelprovider.StreamParcel{Data: text}
		ch <- &libmodelprovider.StreamParcel{Terminal: &libmodelprovider.StreamTerminal{FinishReason: "stop"}}
		close(ch)
		return ch, llmrepo.Meta{ModelName: "test-model", ProviderType: "vertex-google"}, nil
	}
}

func chatTask(policy *llmretry.RetryPolicy) *taskengine.TaskDefinition {
	return &taskengine.TaskDefinition{
		ID:      "chat",
		Handler: taskengine.HandleChatCompletion,
		ExecuteConfig: &taskengine.LLMExecutionConfig{
			Model:       "test-model",
			RetryPolicy: policy,
		},
	}
}

func fastRetry() *llmretry.RetryPolicy {
	return &llmretry.RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: llmretry.Duration(time.Millisecond),
	}
}

func chatInput() taskengine.ChatHistory {
	return taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "user", Content: "hello"}}}
}

func TestUnit_ChatCompletionRetriesRateLimitedStreamError(t *testing.T) {
	var attempts int
	repo := &mockModelRepo{}
	repo.streamFunc = func(ctx context.Context, req llmrepo.Request, messages []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
		attempts++
		if attempts < 3 {
			return errorParcelStream(errors.New("vertex API returned non-200 status for stream: 429, body: RESOURCE_EXHAUSTED"))(ctx, req, messages, opts...)
		}
		return recoveredStream("recovered")(ctx, req, messages, opts...)
	}
	exec, err := taskengine.NewExec(context.Background(), repo, tools.NewMockToolsRegistry(), libtracker.NoopTracker{})
	require.NoError(t, err)

	out, outType, _, err := exec.TaskExec(
		context.Background(), time.Now().UTC(), 100000,
		&taskengine.ChainContext{}, chatTask(fastRetry()), chatInput(), taskengine.DataTypeChatHistory,
	)
	require.NoError(t, err, "a transient provider error carried by the stream must not fail the task")
	require.Equal(t, 3, attempts, "the rate-limited stream is retried up to max_attempts")
	require.Equal(t, taskengine.DataTypeChatHistory, outType)

	hist, ok := out.(taskengine.ChatHistory)
	require.True(t, ok)
	require.Contains(t, hist.Messages[len(hist.Messages)-1].Content, "recovered")
}

func TestUnit_ChatCompletionDoesNotRetryStreamAfterContentWasProduced(t *testing.T) {
	var attempts int
	repo := &mockModelRepo{
		streamFunc: func(context.Context, llmrepo.Request, []libmodelprovider.Message, ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
			attempts++
			ch := make(chan *libmodelprovider.StreamParcel, 2)
			ch <- &libmodelprovider.StreamParcel{Data: "half an answer"}
			ch <- &libmodelprovider.StreamParcel{Error: errors.New("500 internal server error")}
			close(ch)
			return ch, llmrepo.Meta{ModelName: "test-model", ProviderType: "vertex-google"}, nil
		},
	}
	exec, err := taskengine.NewExec(context.Background(), repo, tools.NewMockToolsRegistry(), libtracker.NoopTracker{})
	require.NoError(t, err)

	_, _, _, err = exec.TaskExec(
		context.Background(), time.Now().UTC(), 100000,
		&taskengine.ChainContext{}, chatTask(fastRetry()), chatInput(), taskengine.DataTypeChatHistory,
	)
	require.Error(t, err, "a stream that already produced content fails rather than replaying it")
	require.Equal(t, 1, attempts, "a mid-stream failure is not retried: replaying it would duplicate content the client already saw")
}

func TestUnit_ChatCompletionDoesNotRetryCapacityStreamError(t *testing.T) {
	var attempts int
	repo := &mockModelRepo{
		streamFunc: func(ctx context.Context, req llmrepo.Request, messages []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (<-chan *libmodelprovider.StreamParcel, llmrepo.Meta, error) {
			attempts++
			return errorParcelStream(errors.New("exceeds context length: total token count 131133 > 128000"))(ctx, req, messages, opts...)
		},
	}
	exec, err := taskengine.NewExec(context.Background(), repo, tools.NewMockToolsRegistry(), libtracker.NoopTracker{})
	require.NoError(t, err)

	_, _, _, err = exec.TaskExec(
		context.Background(), time.Now().UTC(), 100000,
		&taskengine.ChainContext{}, chatTask(fastRetry()), chatInput(), taskengine.DataTypeChatHistory,
	)
	require.Error(t, err)
	require.Equal(t, 1, attempts, "a non-retryable class is not retried even with a retry policy set")
}
