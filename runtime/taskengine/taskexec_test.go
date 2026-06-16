package taskengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/contenox/runtime/libtracker"
	libmodelprovider "github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/runtime/internal/tools"
	"github.com/contenox/runtime/runtime/llmrepo"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func TestUnit_TaskExec_ChatCompletionRejectsNilInput(t *testing.T) {
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, _ []libmodelprovider.Message, _ ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			return libmodelprovider.ChatResult{}, llmrepo.Meta{}, errors.New("provider should not be called")
		},
	}
	exec, err := taskengine.NewExec(context.Background(), repo, tools.NewMockToolsRegistry(), libtracker.NoopTracker{})
	require.NoError(t, err)

	task := &taskengine.TaskDefinition{
		ID:            "acp_chat",
		Handler:       taskengine.HandleChatCompletion,
		ExecuteConfig: &taskengine.LLMExecutionConfig{Model: "test-model"},
	}

	_, _, _, err = exec.TaskExec(context.Background(), time.Now().UTC(), 1000, &taskengine.ChainContext{}, task, nil, taskengine.DataTypeAny)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input is nil for task acp_chat")
}
