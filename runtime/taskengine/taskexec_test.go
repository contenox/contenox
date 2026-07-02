package taskengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/internal/tools"
	"github.com/contenox/runtime/runtime/llmrepo"
	libmodelprovider "github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func TestUnit_TaskExec_ChatCompletionRejectsNilInput(t *testing.T) {
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, _ []libmodelprovider.Message, _ ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			return libmodelprovider.ChatResult{}, llmrepo.Meta{}, errors.New("provider should not be called")
		},
	}
	toolsRepo := tools.NewMockToolsRegistry().
		WithResponse("echo", tools.ToolsResponse{Output: "ok"})
	exec, err := taskengine.NewExec(context.Background(), repo, toolsRepo, libtracker.NoopTracker{})
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

func TestUnit_TaskExec_ChatCompletionAddsNoToolsGuardWhenRequestedToolsResolveEmpty(t *testing.T) {
	var seenMessages []libmodelprovider.Message
	var seenToolCount int
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, messages []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			cfg := &libmodelprovider.ChatConfig{}
			for _, opt := range opts {
				opt.Apply(cfg)
			}
			seenToolCount = len(cfg.Tools)
			seenMessages = append([]libmodelprovider.Message(nil), messages...)
			return libmodelprovider.ChatResult{
				Message: libmodelprovider.Message{Role: "assistant", Content: "no tools answer"},
			}, llmrepo.Meta{ModelName: "test-model", ProviderType: "llama"}, nil
		},
	}
	exec, err := taskengine.NewExec(context.Background(), repo, tools.NewMockToolsRegistry(), libtracker.NoopTracker{})
	require.NoError(t, err)

	task := &taskengine.TaskDefinition{
		ID:                "chat",
		Handler:           taskengine.HandleChatCompletion,
		SystemInstruction: "Use tools when they help.",
		ExecuteConfig: &taskengine.LLMExecutionConfig{
			Model: "test-model",
			Tools: []string{"*"},
		},
	}

	_, _, _, err = exec.TaskExec(
		context.Background(), time.Now().UTC(), 4000,
		&taskengine.ChainContext{}, task,
		taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "user", Content: "inspect this"}}},
		taskengine.DataTypeChatHistory,
	)
	require.NoError(t, err)
	require.Zero(t, seenToolCount)
	require.NotEmpty(t, seenMessages)
	require.Equal(t, "system", seenMessages[0].Role)
	require.Contains(t, seenMessages[0].Content, "No tools are available in this turn")
}

func TestUnit_TaskExec_ChatCompletionRetriesWithoutToolsWhenProviderRejectsToolCalls(t *testing.T) {
	var seenToolNames [][]string
	var secondMessages []libmodelprovider.Message
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, messages []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			cfg := &libmodelprovider.ChatConfig{}
			for _, opt := range opts {
				opt.Apply(cfg)
			}
			var names []string
			for _, tool := range cfg.Tools {
				if tool.Function != nil {
					names = append(names, tool.Function.Name)
				}
			}
			seenToolNames = append(seenToolNames, names)
			if len(cfg.Tools) > 0 {
				return libmodelprovider.ChatResult{}, llmrepo.Meta{}, errors.New("chat execution failed: llama: unsupported feature: tool calls (model declares no tool_calls.protocol)")
			}
			secondMessages = append([]libmodelprovider.Message(nil), messages...)
			return libmodelprovider.ChatResult{
				Message: libmodelprovider.Message{Role: "assistant", Content: "hello"},
			}, llmrepo.Meta{ModelName: "test-model", ProviderType: "llama"}, nil
		},
	}
	toolsRepo := tools.NewMockToolsRegistry().
		WithResponse("echo", tools.ToolsResponse{Output: "ok"})
	exec, err := taskengine.NewExec(context.Background(), repo, toolsRepo, libtracker.NoopTracker{})
	require.NoError(t, err)

	chainCtx := &taskengine.ChainContext{
		Tools: map[string]taskengine.ToolWithResolution{
			"echo.echo": {
				Tool:      taskengine.Tool{Type: "function", Function: taskengine.FunctionTool{Name: "echo.echo"}},
				ToolsName: "echo",
			},
		},
	}
	history := taskengine.ChatHistory{Messages: []taskengine.Message{
		{Role: "user", Content: "old request"},
		{Role: "assistant", CallTools: []taskengine.ToolCall{{
			ID: "call-1", Type: "function", Function: taskengine.FunctionCall{Name: "echo.echo", Arguments: `{}`},
		}}},
		{Role: "tool", ToolCallID: "call-1", Content: "ok"},
		{Role: "user", Content: "say hi"},
	}}
	task := &taskengine.TaskDefinition{
		ID:      "chat",
		Handler: taskengine.HandleChatCompletion,
		ExecuteConfig: &taskengine.LLMExecutionConfig{
			Model: "test-model",
			Tools: []string{"echo"},
		},
	}

	out, dt, transition, err := exec.TaskExec(
		context.Background(), time.Now().UTC(), 4000,
		chainCtx, task, history, taskengine.DataTypeChatHistory,
	)
	require.NoError(t, err)
	require.Equal(t, taskengine.DataTypeChatHistory, dt)
	require.Equal(t, taskengine.TransitionExecuted, transition)
	require.Len(t, seenToolNames, 2)
	require.Equal(t, []string{"echo.echo"}, seenToolNames[0])
	require.Empty(t, seenToolNames[1])
	require.NotEmpty(t, secondMessages)
	for _, msg := range secondMessages {
		require.NotEqual(t, "tool", msg.Role)
		require.Empty(t, msg.ToolCalls)
		require.Empty(t, msg.ToolCallID)
		if msg.Role == "assistant" {
			require.NotEmpty(t, msg.Content)
		}
	}
	hist, ok := out.(taskengine.ChatHistory)
	require.True(t, ok)
	require.Equal(t, "hello", hist.Messages[len(hist.Messages)-1].Content)
}

func TestUnit_TaskExec_ChatCompletionRetriesWithoutToolsWhenToolSchemaRejected(t *testing.T) {
	var seenToolCounts []int
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, _ []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			cfg := &libmodelprovider.ChatConfig{}
			for _, opt := range opts {
				opt.Apply(cfg)
			}
			seenToolCounts = append(seenToolCounts, len(cfg.Tools))
			if len(cfg.Tools) > 0 {
				return libmodelprovider.ChatResult{}, llmrepo.Meta{}, errors.New(`chat execution failed: rpc error: code = Internal desc = internal: llamasession: apply chat template: llamacppshim: common chat template: JSON schema conversion failed:
Unrecognized schema: {"description":"Request body"}`)
			}
			return libmodelprovider.ChatResult{
				Message: libmodelprovider.Message{Role: "assistant", Content: "hello"},
			}, llmrepo.Meta{ModelName: "test-model", ProviderType: "llama"}, nil
		},
	}
	toolsRepo := tools.NewMockToolsRegistry().
		WithResponse("echo", tools.ToolsResponse{Output: "ok"})
	exec, err := taskengine.NewExec(context.Background(), repo, toolsRepo, libtracker.NoopTracker{})
	require.NoError(t, err)

	chainCtx := &taskengine.ChainContext{
		Tools: map[string]taskengine.ToolWithResolution{
			"echo.web_post": {
				Tool: taskengine.Tool{Type: "function", Function: taskengine.FunctionTool{
					Name:       "echo.web_post",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{"body": map[string]any{"description": "Request body"}}},
				}},
				ToolsName: "echo",
			},
		},
	}
	task := &taskengine.TaskDefinition{
		ID:      "chat",
		Handler: taskengine.HandleChatCompletion,
		ExecuteConfig: &taskengine.LLMExecutionConfig{
			Model: "test-model",
			Tools: []string{"echo"},
		},
	}

	out, dt, transition, err := exec.TaskExec(
		context.Background(), time.Now().UTC(), 4000,
		chainCtx, task, taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "user", Content: "say hi"}}}, taskengine.DataTypeChatHistory,
	)
	require.NoError(t, err)
	require.Equal(t, taskengine.DataTypeChatHistory, dt)
	require.Equal(t, taskengine.TransitionExecuted, transition)
	require.Equal(t, []int{1, 0}, seenToolCounts)
	hist, ok := out.(taskengine.ChatHistory)
	require.True(t, ok)
	require.Equal(t, "hello", hist.Messages[len(hist.Messages)-1].Content)
}

func TestUnit_TaskExec_ChatCompletionRetriesWithoutToolsWhenToolsOverflowContext(t *testing.T) {
	var seenToolCounts []int
	repo := &mockModelRepo{
		chatFunc: func(_ context.Context, _ llmrepo.Request, _ []libmodelprovider.Message, opts ...libmodelprovider.ChatArgument) (libmodelprovider.ChatResult, llmrepo.Meta, error) {
			cfg := &libmodelprovider.ChatConfig{}
			for _, opt := range opts {
				opt.Apply(cfg)
			}
			seenToolCounts = append(seenToolCounts, len(cfg.Tools))
			if len(cfg.Tools) > 0 {
				return libmodelprovider.ChatResult{}, llmrepo.Meta{}, errors.New("chat execution failed: llama: context overflow: exceeded the session context window: exceeded the session context window during suffix: resident_tokens=1 additional_tokens=4871 num_ctx=4074")
			}
			return libmodelprovider.ChatResult{
				Message: libmodelprovider.Message{Role: "assistant", Content: "hello"},
			}, llmrepo.Meta{ModelName: "test-model", ProviderType: "llama"}, nil
		},
	}
	toolsRepo := tools.NewMockToolsRegistry().
		WithResponse("echo", tools.ToolsResponse{Output: "ok"})
	exec, err := taskengine.NewExec(context.Background(), repo, toolsRepo, libtracker.NoopTracker{})
	require.NoError(t, err)

	chainCtx := &taskengine.ChainContext{
		Tools: map[string]taskengine.ToolWithResolution{
			"echo.echo": {
				Tool:      taskengine.Tool{Type: "function", Function: taskengine.FunctionTool{Name: "echo.echo"}},
				ToolsName: "echo",
			},
		},
	}
	task := &taskengine.TaskDefinition{
		ID:      "chat",
		Handler: taskengine.HandleChatCompletion,
		ExecuteConfig: &taskengine.LLMExecutionConfig{
			Model: "test-model",
			Tools: []string{"echo"},
		},
	}

	out, dt, transition, err := exec.TaskExec(
		context.Background(), time.Now().UTC(), 4000,
		chainCtx, task, taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "user", Content: "say hi"}}}, taskengine.DataTypeChatHistory,
	)
	require.NoError(t, err)
	require.Equal(t, taskengine.DataTypeChatHistory, dt)
	require.Equal(t, taskengine.TransitionExecuted, transition)
	require.Equal(t, []int{1, 0}, seenToolCounts)
	hist, ok := out.(taskengine.ChatHistory)
	require.True(t, ok)
	require.Equal(t, "hello", hist.Messages[len(hist.Messages)-1].Content)
}
