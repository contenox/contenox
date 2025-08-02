package taskengine

import "context"

// No-op implementations for dependencies
type noopTracker struct{}

func (t *noopTracker) Start(ctx context.Context, eventType string, taskID string, kvs ...any) (
	func(error),
	func(string, any),
	func(),
) {
	return func(err error) {}, func(s string, a any) {}, func() {}
}

type NoopAlertSink struct{}

func (a *NoopAlertSink) SendAlert(ctx context.Context, msg string, kvs ...string) error {
	return nil
}

func (s *NoopAlertSink) FetchAlerts(ctx context.Context, limit int) ([]*Alert, error) {
	return nil, nil
}

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
