package execservice

import (
	"context"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/taskengine"
)

type TasksEnvService interface {
	Execute(ctx context.Context, chain *taskengine.ChainDefinition, input string) (any, []taskengine.CapturedStateUnit, error)
	taskengine.HookRegistry
}

type tasksEnvService struct {
	environmentExec taskengine.EnvExecutor
	db              libdb.DBManager
	hookRegistry    taskengine.HookRegistry
}

func NewTasksEnv(ctx context.Context, environmentExec taskengine.EnvExecutor, dbInstance libdb.DBManager, hookRegistry taskengine.HookRegistry) TasksEnvService {
	return &tasksEnvService{
		environmentExec: environmentExec,
		db:              dbInstance,
		hookRegistry:    hookRegistry,
	}
}

func (s *tasksEnvService) Execute(ctx context.Context, chain *taskengine.ChainDefinition, input string) (any, []taskengine.CapturedStateUnit, error) {
	return s.environmentExec.ExecEnv(ctx, chain, input, taskengine.DataTypeAny)
}

func (s *tasksEnvService) Supports(ctx context.Context) ([]string, error) {
	return s.hookRegistry.Supports(ctx)
}
