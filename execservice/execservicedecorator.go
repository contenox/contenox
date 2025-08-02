package execservice

import (
	"context"

	"github.com/contenox/activitytracker"
)

type activityTrackerDecorator struct {
	service ExecService
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Execute(ctx context.Context, request *TaskRequest) (*TaskResponse, error) {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"execute",
		"prompt",
		"promptLength", len(request.Prompt),
	)
	defer endFn()

	response, err := d.service.Execute(ctx, request)
	if err != nil {
		reportErrFn(err)
	} else if response != nil {
		reportChangeFn(response.ID, map[string]interface{}{
			"prompt":   request.Prompt,
			"response": response.Response,
		})
	}

	return response, err
}

func WithActivityTracker(service ExecService, tracker activitytracker.ActivityTracker) ExecService {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

var _ ExecService = (*activityTrackerDecorator)(nil)
