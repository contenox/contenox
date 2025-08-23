package taskengine

import (
	"context"
	"time"
)

// MockTaskExecutor is a mock implementation of taskengine.TaskExecutor.
type MockTaskExecutor struct {
	// Single value responses
	MockOutput      any
	MockRawResponse string
	MockError       error

	// Sequence responses
	MockOutputSequence      []any
	MockTaskTypeSequence    []DataType
	MockRawResponseSequence []string
	ErrorSequence           []error

	// Tracking
	CalledWithTask   *TaskDefinition
	CalledWithInput  any
	CalledWithPrompt string
	callCount        int
}

// TaskExec is the mock implementation of the TaskExec method.
func (m *MockTaskExecutor) TaskExec(ctx context.Context, startingTime time.Time, tokenLimit int, currentTask *TaskDefinition, input any, dataType DataType) (any, DataType, string, error) {
	m.callCount++
	m.CalledWithTask = currentTask
	m.CalledWithInput = input

	// Get output from sequence or single value
	var output any
	if len(m.MockOutputSequence) > 0 {
		output = m.MockOutputSequence[0]
		if len(m.MockOutputSequence) > 1 {
			m.MockOutputSequence = m.MockOutputSequence[1:]
		}
	} else {
		output = m.MockOutput
	}

	// Get error from sequence or single value
	var err error
	if len(m.ErrorSequence) > 0 {
		err = m.ErrorSequence[0]
		if len(m.ErrorSequence) > 1 {
			m.ErrorSequence = m.ErrorSequence[1:]
		}
	} else {
		err = m.MockError
	}

	// Get output data type from sequence or determine from output
	var outputDataType DataType
	if len(m.MockTaskTypeSequence) > 0 {
		outputDataType = m.MockTaskTypeSequence[0]
		if len(m.MockTaskTypeSequence) > 1 {
			m.MockTaskTypeSequence = m.MockTaskTypeSequence[1:]
		}
	} else {
		// Fallback: Determine data type from output value
		switch v := output.(type) {
		case string:
			outputDataType = DataTypeString
		case bool:
			outputDataType = DataTypeBool
		case int:
			outputDataType = DataTypeInt
		case float64:
			outputDataType = DataTypeFloat
		case ChatHistory:
			outputDataType = DataTypeChatHistory
		case OpenAIChatRequest:
			outputDataType = DataTypeOpenAIChat
		case OpenAIChatResponse:
			outputDataType = DataTypeOpenAIChatResponse
		case map[string]any:
			outputDataType = DataTypeJSON
		default:
			if v == nil {
				outputDataType = dataType // If output is nil, preserve input type
			} else {
				outputDataType = DataTypeAny
			}
		}
	}

	// Get raw response from sequence or single value
	var rawResponse string
	if len(m.MockRawResponseSequence) > 0 {
		rawResponse = m.MockRawResponseSequence[0]
		if len(m.MockRawResponseSequence) > 1 {
			m.MockRawResponseSequence = m.MockRawResponseSequence[1:]
		}
	} else {
		rawResponse = m.MockRawResponse
	}

	// Determine transition evaluation
	transitionEval := "mock_transition_ok"
	if s, ok := output.(string); ok {
		transitionEval = s // Handlers like raw_string use their output for transitions.
	} else if rawResponse != "" {
		transitionEval = rawResponse
	}

	return output, outputDataType, transitionEval, err
}

// Reset clears all mock state between tests
func (m *MockTaskExecutor) Reset() {
	m.MockOutput = nil
	m.MockRawResponse = ""
	m.MockError = nil
	m.MockOutputSequence = nil
	m.MockTaskTypeSequence = nil
	m.MockRawResponseSequence = nil
	m.ErrorSequence = nil
	m.CalledWithTask = nil
	m.CalledWithInput = nil
	m.CalledWithPrompt = ""
	m.callCount = 0
}

// CallCount returns how many times TaskExec was called
func (m *MockTaskExecutor) CallCount() int {
	return m.callCount
}
