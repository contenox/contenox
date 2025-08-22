package taskengine

import "context"

type NoopInspector struct{}

func (i *NoopInspector) Start(ctx context.Context) StackTrace {
	return &NoopExecutionStack{}
}

type NoopExecutionStack struct{}

// ClearBreakpoints implements StackTrace.
func (s *NoopExecutionStack) ClearBreakpoints() {
	panic("unimplemented")
}

// GetCurrentState implements StackTrace.
func (s *NoopExecutionStack) GetCurrentState() ExecutionState {
	panic("unimplemented")
}

// SetBreakpoint implements StackTrace.
func (s *NoopExecutionStack) SetBreakpoint(taskID string) {
	panic("unimplemented")
}

func (s *NoopExecutionStack) RecordStep(step CapturedStateUnit) {}
func (s *NoopExecutionStack) GetExecutionHistory() []CapturedStateUnit {
	return nil
}
func (s *NoopExecutionStack) HasBreakpoint(taskID string) bool { return false }
