package taskengine_test

import (
	"context"
	"testing"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/taskengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_ComposeOverride(t *testing.T) {
	// Setup environment with proper mock sequence
	mockExec := &taskengine.MockTaskExecutor{
		MockOutputSequence: []any{
			map[string]any{"a": 1, "b": 2},
			map[string]any{"b": 3, "c": 4},
		},
	}
	env := setupTestEnv(mockExec)

	// Define chain with compose task
	chain := &taskengine.TaskChainDefinition{
		Tasks: []taskengine.TaskDefinition{
			{
				ID:      "task1",
				Handler: taskengine.HandleRawString,
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "task2"},
					},
				},
			},
			{
				ID:      "task2",
				Handler: taskengine.HandleRawString,
				Compose: &taskengine.ComposeTask{
					WithVar:  "task1",
					Strategy: "override",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}

	// Execute chain
	output, _, _, err := env.ExecEnv(context.Background(), chain, map[string]any{"a": 1, "b": 2}, taskengine.DataTypeJSON)
	require.NoError(t, err)

	// Verify composition
	expected := map[string]any{"a": 1, "b": 2, "c": 4}
	assert.Equal(t, expected, output)
}

func TestUnit_ComposeAppendStringToChatHistory(t *testing.T) {
	// Setup environment
	mockExec := &taskengine.MockTaskExecutor{
		MockOutputSequence: []any{
			"New system message", // Task1 output (string)
			taskengine.ChatHistory{ // Task2 output (ChatHistory)
				Messages: []taskengine.Message{
					{Role: "user", Content: "Hello"},
				},
			},
		},
	}
	env := setupTestEnv(mockExec)

	// Define chain
	chain := &taskengine.TaskChainDefinition{
		Tasks: []taskengine.TaskDefinition{
			{
				ID:      "task1",
				Handler: taskengine.HandleRawString,
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "task2"},
					},
				},
			},
			{
				ID:      "task2",
				Handler: taskengine.HandleRawString,
				Compose: &taskengine.ComposeTask{
					WithVar:  "task1",
					Strategy: "append_string_to_chat_history",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}

	// Execute chain
	output, _, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeAny)
	require.NoError(t, err)

	// Verify composition
	ch, ok := output.(taskengine.ChatHistory)
	require.True(t, ok, "output should be ChatHistory")
	require.Len(t, ch.Messages, 2)
	assert.Equal(t, "system", ch.Messages[0].Role)
	assert.Equal(t, "New system message", ch.Messages[0].Content)
	assert.Equal(t, "user", ch.Messages[1].Role)
	assert.Equal(t, "Hello", ch.Messages[1].Content)
}

func TestUnit_ComposeMergeChatHistories(t *testing.T) {
	// Setup environment
	mockExec := &taskengine.MockTaskExecutor{
		MockOutputSequence: []any{
			taskengine.ChatHistory{ // Task1 output
				Messages: []taskengine.Message{
					{Role: "user", Content: "Hello"},
				},
				InputTokens: 10,
			},
			taskengine.ChatHistory{ // Task2 output
				Messages: []taskengine.Message{
					{Role: "assistant", Content: "Hi there!"},
				},
				InputTokens: 20,
				Model:       "gpt-4",
			},
		},
		MockTaskTypeSequence: []taskengine.DataType{
			taskengine.DataTypeChatHistory,
			taskengine.DataTypeChatHistory,
		},
	}
	env := setupTestEnv(mockExec)

	// Define chain
	chain := &taskengine.TaskChainDefinition{
		Tasks: []taskengine.TaskDefinition{
			{
				ID:      "task1",
				Handler: taskengine.HandleNoop,
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "task2"},
					},
				},
			},
			{
				ID:      "task2",
				Handler: taskengine.HandleNoop,
				Compose: &taskengine.ComposeTask{
					WithVar:  "task1",
					Strategy: "merge_chat_histories",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}

	// Execute chain
	output, dt, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeChatHistory)
	require.NoError(t, err)
	require.Equal(t, taskengine.DataTypeChatHistory, dt)
	// Verify composition
	ch, ok := output.(taskengine.ChatHistory)
	require.True(t, ok, "output should be ChatHistory")
	require.Len(t, ch.Messages, 2)
	assert.Equal(t, "user", ch.Messages[0].Role)
	assert.Equal(t, "Hello", ch.Messages[0].Content)
	assert.Equal(t, "assistant", ch.Messages[1].Role)
	assert.Equal(t, "Hi there!", ch.Messages[1].Content)
	assert.Equal(t, 30, ch.InputTokens)
	assert.Empty(t, ch.Model) // Models differ so should be empty
}

func TestUnit_ComposeAutoStrategy(t *testing.T) {
	t.Run("NonChatHistoryOverride", func(t *testing.T) {
		// Setup environment
		mockExec := &taskengine.MockTaskExecutor{
			MockOutputSequence: []any{
				map[string]any{"a": 1},
				map[string]any{"b": 2},
			},
		}
		env := setupTestEnv(mockExec)

		// Define chain with automatic strategy
		chain := &taskengine.TaskChainDefinition{
			Tasks: []taskengine.TaskDefinition{
				{
					ID:      "task1",
					Handler: taskengine.HandleRawString,
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: "task2"},
						},
					},
				},
				{
					ID:      "task2",
					Handler: taskengine.HandleRawString,
					Compose: &taskengine.ComposeTask{
						WithVar: "task1", // Strategy omitted for auto
					},
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
						},
					},
				},
			},
		}

		// Execute chain
		output, _, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeAny)
		require.NoError(t, err)

		// Verify auto-selected override strategy
		result, ok := output.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 1, result["a"])
		assert.Equal(t, 2, result["b"])
	})
}

func TestUnit_ComposeMergeChatHistories_MessageOrder(t *testing.T) {
	// Setup environment
	mockExec := &taskengine.MockTaskExecutor{
		MockOutputSequence: []any{
			taskengine.ChatHistory{ // Task1 output (user message)
				Messages: []taskengine.Message{
					{Role: "user", Content: "User message"},
				},
			},
			taskengine.ChatHistory{ // Task2 output (system message)
				Messages: []taskengine.Message{
					{Role: "system", Content: "System message"},
				},
			},
		},
		MockTaskTypeSequence: []taskengine.DataType{
			taskengine.DataTypeChatHistory,
			taskengine.DataTypeChatHistory,
		},
	}
	env := setupTestEnv(mockExec)

	// Define chain
	chain := &taskengine.TaskChainDefinition{
		Tasks: []taskengine.TaskDefinition{
			{
				ID:      "task1",
				Handler: taskengine.HandleNoop,
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: "task2"},
					},
				},
			},
			{
				ID:      "task2",
				Handler: taskengine.HandleNoop,
				Compose: &taskengine.ComposeTask{
					WithVar:  "task1", // Compose with task1 output
					Strategy: "merge_chat_histories",
				},
				Transition: taskengine.TaskTransition{
					Branches: []taskengine.TransitionBranch{
						{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
					},
				},
			},
		},
	}

	// Execute chain
	output, dt, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeChatHistory)
	require.NoError(t, err)
	require.Equal(t, taskengine.DataTypeChatHistory, dt)

	// Verify composition and message order
	ch, ok := output.(taskengine.ChatHistory)
	require.True(t, ok, "output should be ChatHistory")
	require.Len(t, ch.Messages, 2)

	// The right messages (task1 output) should come first
	assert.Equal(t, "user", ch.Messages[0].Role)
	assert.Equal(t, "User message", ch.Messages[0].Content)

	// The left messages (task2 output) should come after
	assert.Equal(t, "system", ch.Messages[1].Role)
	assert.Equal(t, "System message", ch.Messages[1].Content)
}

func TestUnit_ComposeErrors(t *testing.T) {
	t.Run("UnsupportedStrategy", func(t *testing.T) {
		// Setup environment
		mockExec := &taskengine.MockTaskExecutor{MockOutput: "test"}
		env := setupTestEnv(mockExec)

		// Define chain with invalid strategy
		chain := &taskengine.TaskChainDefinition{
			Tasks: []taskengine.TaskDefinition{
				{
					ID:      "task1",
					Handler: taskengine.HandleRawString,
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: "task2"},
						},
					},
				},
				{
					ID:      "task2",
					Handler: taskengine.HandleRawString,
					Compose: &taskengine.ComposeTask{
						WithVar:  "task1",
						Strategy: "invalid_strategy",
					},
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
						},
					},
				},
			},
		}

		// Execute chain
		_, _, _, err := env.ExecEnv(context.Background(), chain, "input", taskengine.DataTypeString)

		// Verify error
		assert.ErrorContains(t, err, "unsupported compose strategy")
	})

	t.Run("MissingRightVar", func(t *testing.T) {
		// Setup environment
		mockExec := &taskengine.MockTaskExecutor{MockOutput: "test"}
		env := setupTestEnv(mockExec)

		// Define chain with missing right variable
		chain := &taskengine.TaskChainDefinition{
			Tasks: []taskengine.TaskDefinition{
				{
					ID:      "task1",
					Handler: taskengine.HandleRawString,
					Compose: &taskengine.ComposeTask{
						WithVar: "nonexistent",
					},
				},
			},
		}

		// Execute chain
		_, _, _, err := env.ExecEnv(context.Background(), chain, "input", taskengine.DataTypeString)

		// Verify error
		assert.ErrorContains(t, err, "compose right_var \"nonexistent\" not found")
	})

	t.Run("InvalidAppendStringTypes", func(t *testing.T) {
		// Setup environment
		mockExec := &taskengine.MockTaskExecutor{
			MockOutputSequence: []any{
				[]string{}, // Invalid type
				taskengine.ChatHistory{},
			},
		}
		env := setupTestEnv(mockExec)

		// Define chain
		chain := &taskengine.TaskChainDefinition{
			Tasks: []taskengine.TaskDefinition{
				{
					ID:      "task1",
					Handler: taskengine.HandleRawString,
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: "task2"},
						},
					},
				},
				{
					ID:      "task2",
					Handler: taskengine.HandleRawString,
					Compose: &taskengine.ComposeTask{
						WithVar:  "task1",
						Strategy: "append_string_to_chat_history",
					},
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: taskengine.TermEnd},
						},
					},
				},
			},
		}

		// Execute chain
		_, _, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeAny)

		// Verify error
		assert.Error(t, err, "invalid types for append_string_to_chat_history")
	})

	t.Run("InvalidMergeChatHistoryTypes", func(t *testing.T) {
		// Setup environment
		mockExec := &taskengine.MockTaskExecutor{
			MockOutputSequence: []any{
				"not a chat history",
				taskengine.ChatHistory{},
			},
		}
		env := setupTestEnv(mockExec)

		// Define chain
		chain := &taskengine.TaskChainDefinition{
			Tasks: []taskengine.TaskDefinition{
				{
					ID:      "task1",
					Handler: taskengine.HandleRawString,
					Transition: taskengine.TaskTransition{
						Branches: []taskengine.TransitionBranch{
							{Operator: taskengine.OpDefault, Goto: "task2"},
						},
					},
				},
				{
					ID:      "task2",
					Handler: taskengine.HandleRawString,
					Compose: &taskengine.ComposeTask{
						WithVar:  "task1",
						Strategy: "merge_chat_histories",
					},
				},
			},
		}

		// Execute chain
		_, _, _, err := env.ExecEnv(context.Background(), chain, nil, taskengine.DataTypeAny)
		assert.Error(t, err, "compose strategy 'merge_chat_histories' requires both left")
	})
}

// Helper to create test environment
func setupTestEnv(exec taskengine.TaskExecutor) taskengine.EnvExecutor {
	// Create no-op dependencies
	tracker := &libtracker.NoopTracker{}
	inspector := taskengine.NewSimpleInspector()

	env, _ := taskengine.NewEnv(
		context.Background(),
		tracker,
		exec,
		inspector,
	)
	return env
}
