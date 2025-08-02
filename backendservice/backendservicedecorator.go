package backendservice

import (
	"context"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/store"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Create(ctx context.Context, backend *store.Backend) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"create",
		"backend",
		"name", backend.Name,
		"type", backend.Type,
		"baseURL", backend.BaseURL,
	)
	defer endFn()

	err := d.service.Create(ctx, backend)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(backend.ID, map[string]interface{}{
			"name":    backend.Name,
			"type":    backend.Type,
			"baseURL": backend.BaseURL,
		})
	}

	return err
}

func (d *activityTrackerDecorator) Get(ctx context.Context, id string) (*store.Backend, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"backend",
		"backendID", id,
	)
	defer endFn()

	backend, err := d.service.Get(ctx, id)
	if err != nil {
		reportErrFn(err)
	}

	return backend, err
}

func (d *activityTrackerDecorator) Update(ctx context.Context, backend *store.Backend) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"update",
		"backend",
		"backendID", backend.ID,
		"name", backend.Name,
	)
	defer endFn()

	err := d.service.Update(ctx, backend)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(backend.ID, map[string]interface{}{
			"name":    backend.Name,
			"type":    backend.Type,
			"baseURL": backend.BaseURL,
		})
	}

	return err
}

func (d *activityTrackerDecorator) Delete(ctx context.Context, id string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"delete",
		"backend",
		"backendID", id,
	)
	defer endFn()

	err := d.service.Delete(ctx, id)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(id, nil)
	}

	return err
}

func (d *activityTrackerDecorator) List(ctx context.Context) ([]*store.Backend, error) {
	reportErrFn, _, endFn := d.tracker.Start(ctx, "list", "backends")
	defer endFn()

	backends, err := d.service.List(ctx)
	if err != nil {
		reportErrFn(err)
	}

	return backends, err
}

func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

var _ Service = (*activityTrackerDecorator)(nil)
