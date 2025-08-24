package taskengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func TestUnit_TaskExec_PromptToString(t *testing.T) {
	mockExec := &taskengine.MockTaskExecutor{
		MockOutput:          "mock-result",
		MockTransitionValue: "mock-response",
		MockError:           nil,
	}

	task := &taskengine.TaskDefinition{
		Handler: taskengine.HandleRawString,
	}

	output, _, _, err := mockExec.TaskExec(context.Background(), time.Now(), 100, task, "What is 2+2?", taskengine.DataTypeString)
	require.NoError(t, err)
	require.Equal(t, "mock-result", output)
}
