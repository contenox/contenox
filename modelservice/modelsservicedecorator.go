package modelservice

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/runtimetypes"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Append(ctx context.Context, model *runtimetypes.Model) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"create",
		"model",
		"name", model.Model,
	)
	defer endFn()

	err := d.service.Append(ctx, model)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(model.Model, map[string]interface{}{
			"name": model.Model,
		})
	}

	return err
}

func (d *activityTrackerDecorator) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Model, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"list",
		"models",
		"cursor", fmt.Sprintf("%v", createdAtCursor),
		"limit", fmt.Sprintf("%d", limit),
	)
	defer endFn()

	models, err := d.service.List(ctx, createdAtCursor, limit)
	if err != nil {
		reportErrFn(err)
	}

	return models, err
}

func (d *activityTrackerDecorator) Delete(ctx context.Context, modelName string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"delete",
		"model",
		"name", modelName,
	)
	defer endFn()

	err := d.service.Delete(ctx, modelName)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(modelName, nil)
	}

	return err
}

func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}
