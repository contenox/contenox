package hookproviderservice

import (
	"context"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/store"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Create(ctx context.Context, hook *store.RemoteHook) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"create",
		"remote_hook",
		"name", hook.Name,
		"endpoint_url", hook.EndpointURL,
	)
	defer endFn()

	err := d.service.Create(ctx, hook)
	if err != nil {
		reportErrFn(err)
	} else {
		// Mask sensitive information if needed
		hookData := struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			EndpointURL string `json:"endpointUrl"`
			Method      string `json:"method"`
			TimeoutMs   int    `json:"timeoutMs"`
		}{
			ID:          hook.ID,
			Name:        hook.Name,
			EndpointURL: hook.EndpointURL,
			Method:      hook.Method,
			TimeoutMs:   hook.TimeoutMs,
		}
		reportChangeFn(hook.ID, hookData)
	}

	return err
}

func (d *activityTrackerDecorator) Get(ctx context.Context, id string) (*store.RemoteHook, error) {
	// Minimal tracking for read operations
	_, _, endFn := d.tracker.Start(
		ctx,
		"get",
		"remote_hook",
		"id", id,
	)
	defer endFn()

	return d.service.Get(ctx, id)
}

func (d *activityTrackerDecorator) GetByName(ctx context.Context, name string) (*store.RemoteHook, error) {
	// Minimal tracking for read operations
	_, _, endFn := d.tracker.Start(
		ctx,
		"get_by_name",
		"remote_hook",
		"name", name,
	)
	defer endFn()

	return d.service.GetByName(ctx, name)
}

func (d *activityTrackerDecorator) Update(ctx context.Context, hook *store.RemoteHook) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"update",
		"remote_hook",
		"id", hook.ID,
		"name", hook.Name,
	)
	defer endFn()

	err := d.service.Update(ctx, hook)
	if err != nil {
		reportErrFn(err)
	} else {
		// Create a sanitized version for reporting
		hookData := struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			EndpointURL string `json:"endpointUrl"`
			Method      string `json:"method"`
			TimeoutMs   int    `json:"timeoutMs"`
		}{
			ID:          hook.ID,
			Name:        hook.Name,
			EndpointURL: hook.EndpointURL,
			Method:      hook.Method,
			TimeoutMs:   hook.TimeoutMs,
		}
		reportChangeFn(hook.ID, hookData)
	}

	return err
}

func (d *activityTrackerDecorator) Delete(ctx context.Context, id string) error {
	// Get hook details before deletion for reporting
	hook, err := d.service.Get(ctx, id)
	if err != nil {
		return err
	}

	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"delete",
		"remote_hook",
		"id", id,
		"name", hook.Name,
	)
	defer endFn()

	err = d.service.Delete(ctx, id)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(id, nil)
	}

	return err
}

func (d *activityTrackerDecorator) List(ctx context.Context) ([]*store.RemoteHook, error) {
	// Minimal tracking for list operations
	_, _, endFn := d.tracker.Start(
		ctx,
		"list",
		"remote_hooks",
	)
	defer endFn()

	return d.service.List(ctx)
}

// WithActivityTracker wraps the service with activity tracking
func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}
