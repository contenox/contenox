package stateservice

import (
	"context"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/runtimestate"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Get(ctx context.Context) (map[string]runtimestate.LLMState, error) {
	// Start tracking the operation
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",  // operation type
		"state", // resource type
	)
	defer endFn()

	// Execute the actual service call
	stateMap, err := d.service.Get(ctx)

	if err != nil {
		reportErrFn(err)
	}

	return stateMap, err
}

// WithActivityTracker wraps a StateService with activity tracking
func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

// Ensure the decorator implements the Service interface
var _ Service = (*activityTrackerDecorator)(nil)
