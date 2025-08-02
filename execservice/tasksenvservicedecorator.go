package execservice

import (
	"context"

	"github.com/contenox/activitytracker"
	"github.com/contenox/runtime/taskengine"
)

type activityTrackerTaskEnvDecorator struct {
	service TasksEnvService
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerTaskEnvDecorator) Execute(ctx context.Context, chain *taskengine.ChainDefinition, input string) (any, []taskengine.CapturedStateUnit, error) {
	// Extract useful metadata from the chain
	chainID := chain.ID

	reportErrFn, reportChangeFn, endFn := d.tracker.Start(
		ctx,
		"execute",
		"task-chain",
		"chainID", chainID,
		"inputLength", len(input),
	)
	defer endFn()

	result, stacktrace, err := d.service.Execute(ctx, chain, input)
	if err != nil {
		reportErrFn(err)
	} else {
		reportChangeFn(chainID, map[string]interface{}{
			"input":      input,
			"result":     result,
			"chainID":    chainID,
			"stacktrace": stacktrace,
		})
	}

	return result, stacktrace, err
}

func (d *activityTrackerTaskEnvDecorator) Supports(ctx context.Context) ([]string, error) {
	return d.service.Supports(ctx)
}

func EnvWithActivityTracker(service TasksEnvService, tracker activitytracker.ActivityTracker) TasksEnvService {
	return &activityTrackerTaskEnvDecorator{
		service: service,
		tracker: tracker,
	}
}
