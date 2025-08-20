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
	MockRawResponseSequence []string
	ErrorSequence           []error

	// Tracking
	CalledWithTask   *ChainTask
	CalledWithInput  any
	CalledWithPrompt string
	callCount        int
}

// TaskExec is the mock implementation of the TaskExec method.
func (m *MockTaskExecutor) TaskExec(ctx context.Context, startingTime time.Time, tokenLimit int, currentTask *ChainTask, input any, dataType DataType) (any, DataType, string, error) {
	m.callCount++
	m.CalledWithTask = currentTask
	m.CalledWithInput = input

	// Store prompt if input is string (for backward compatibility)
	if s, ok := input.(string); ok {
		m.CalledWithPrompt = s
	}

	// Determine which output to return
	var output any
	switch {
	case len(m.MockOutputSequence) > 0:
		output = m.MockOutputSequence[0]
		if len(m.MockOutputSequence) > 1 {
			m.MockOutputSequence = m.MockOutputSequence[1:]
		} else {
			m.MockOutputSequence = nil
		}
	default:
		output = m.MockOutput
	}

	// Determine which raw response to return
	var rawResponse string
	switch {
	case len(m.MockRawResponseSequence) > 0:
		rawResponse = m.MockRawResponseSequence[0]
		if len(m.MockRawResponseSequence) > 1 {
			m.MockRawResponseSequence = m.MockRawResponseSequence[1:]
		} else {
			m.MockRawResponseSequence = nil
		}
	default:
		rawResponse = m.MockRawResponse
	}

	// Determine which error to return
	var err error
	switch {
	case len(m.ErrorSequence) > 0:
		err = m.ErrorSequence[0]
		if len(m.ErrorSequence) > 1 {
			m.ErrorSequence = m.ErrorSequence[1:]
		} else {
			m.ErrorSequence = nil
		}
	default:
		err = m.MockError
	}

	return output, dataType, rawResponse, err
}

// Reset clears all mock state between tests
func (m *MockTaskExecutor) Reset() {
	m.MockOutput = nil
	m.MockRawResponse = ""
	m.MockError = nil
	m.MockOutputSequence = nil
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
