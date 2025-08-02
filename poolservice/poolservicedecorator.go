package poolservice

import (
	"context"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/store"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Create(ctx context.Context, pool *store.Pool) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"create",
		"pool",
		"name", pool.Name,
		"purposeType", pool.PurposeType,
	)
	defer endFn()

	err := d.service.Create(ctx, pool)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(pool.ID, map[string]interface{}{
			"name":        pool.Name,
			"purposeType": pool.PurposeType,
		})
	}

	return err
}

func (d *activityTrackerDecorator) GetByID(ctx context.Context, id string) (*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pool",
		"poolID", id,
	)
	defer endFn()

	pool, err := d.service.GetByID(ctx, id)
	if err != nil {
		reportErrFn(err)
	}

	return pool, err
}

func (d *activityTrackerDecorator) GetByName(ctx context.Context, name string) (*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pool",
		"name", name,
	)
	defer endFn()

	pool, err := d.service.GetByName(ctx, name)
	if err != nil {
		reportErrFn(err)
	}

	return pool, err
}

func (d *activityTrackerDecorator) Update(ctx context.Context, pool *store.Pool) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"update",
		"pool",
		"poolID", pool.ID,
		"name", pool.Name,
	)
	defer endFn()

	err := d.service.Update(ctx, pool)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(pool.ID, map[string]interface{}{
			"name":        pool.Name,
			"purposeType": pool.PurposeType,
		})
	}

	return err
}

func (d *activityTrackerDecorator) Delete(ctx context.Context, id string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"delete",
		"pool",
		"poolID", id,
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

func (d *activityTrackerDecorator) ListAll(ctx context.Context) ([]*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(ctx, "list", "pools")
	defer endFn()

	pools, err := d.service.ListAll(ctx)
	if err != nil {
		reportErrFn(err)
	}

	return pools, err
}

func (d *activityTrackerDecorator) ListByPurpose(ctx context.Context, purpose string) ([]*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"list",
		"pools-by-purpose",
		"purpose", purpose,
	)
	defer endFn()

	pools, err := d.service.ListByPurpose(ctx, purpose)
	if err != nil {
		reportErrFn(err)
	}

	return pools, err
}

func (d *activityTrackerDecorator) AssignBackend(ctx context.Context, poolID, backendID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"assign",
		"backend-to-pool",
		"poolID", poolID,
		"backendID", backendID,
	)
	defer endFn()

	err := d.service.AssignBackend(ctx, poolID, backendID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(poolID, map[string]interface{}{
			"backendID": backendID,
		})
	}

	return err
}

func (d *activityTrackerDecorator) RemoveBackend(ctx context.Context, poolID, backendID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"remove",
		"backend-from-pool",
		"poolID", poolID,
		"backendID", backendID,
	)
	defer endFn()

	err := d.service.RemoveBackend(ctx, poolID, backendID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(poolID, map[string]interface{}{
			"backendID": backendID,
		})
	}

	return err
}

func (d *activityTrackerDecorator) ListBackends(ctx context.Context, poolID string) ([]*store.Backend, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pool-backends",
		"poolID", poolID,
	)
	defer endFn()

	backends, err := d.service.ListBackends(ctx, poolID)
	if err != nil {
		reportErrFn(err)
	}

	return backends, err
}

func (d *activityTrackerDecorator) ListPoolsForBackend(ctx context.Context, backendID string) ([]*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pools-for-backend",
		"backendID", backendID,
	)
	defer endFn()

	pools, err := d.service.ListPoolsForBackend(ctx, backendID)
	if err != nil {
		reportErrFn(err)
	}

	return pools, err
}

func (d *activityTrackerDecorator) AssignModel(ctx context.Context, poolID, modelID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"assign",
		"model-to-pool",
		"poolID", poolID,
		"modelID", modelID,
	)
	defer endFn()

	err := d.service.AssignModel(ctx, poolID, modelID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(poolID, map[string]interface{}{
			"modelID": modelID,
		})
	}

	return err
}

func (d *activityTrackerDecorator) RemoveModel(ctx context.Context, poolID, modelID string) error {
	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"remove",
		"model-from-pool",
		"poolID", poolID,
		"modelID", modelID,
	)
	defer endFn()

	err := d.service.RemoveModel(ctx, poolID, modelID)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(poolID, map[string]interface{}{
			"modelID": modelID,
		})
	}

	return err
}

func (d *activityTrackerDecorator) ListModels(ctx context.Context, poolID string) ([]*store.Model, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pool-models",
		"poolID", poolID,
	)
	defer endFn()

	models, err := d.service.ListModels(ctx, poolID)
	if err != nil {
		reportErrFn(err)
	}

	return models, err
}

func (d *activityTrackerDecorator) ListPoolsForModel(ctx context.Context, modelID string) ([]*store.Pool, error) {
	reportErrFn, _, endFn := d.tracker.Start(
		ctx,
		"read",
		"pools-for-model",
		"modelID", modelID,
	)
	defer endFn()

	pools, err := d.service.ListPoolsForModel(ctx, modelID)
	if err != nil {
		reportErrFn(err)
	}

	return pools, err
}

func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

var _ Service = (*activityTrackerDecorator)(nil)
