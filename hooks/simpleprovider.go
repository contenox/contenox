package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/runtime/taskengine"
)

type SimpleRepo struct {
	hooks map[string]taskengine.HookRepo
}

func NewSimpleProvider(hooks map[string]taskengine.HookRepo) taskengine.HookRepo {
	return &SimpleRepo{
		hooks: hooks,
	}
}

func (m *SimpleRepo) Exec(ctx context.Context, startingTime time.Time, input any, dataType taskengine.DataType, transition string, args *taskengine.HookCall) (int, any, taskengine.DataType, string, error) {
	if hook, ok := m.hooks[args.Type]; ok {
		return hook.Exec(ctx, startingTime, input, dataType, transition, args)
	}
	return taskengine.StatusUnknownHookProvider, nil, taskengine.DataTypeAny, transition, fmt.Errorf("unknown hook type: %s", args.Type)
}

func (m *SimpleRepo) Supports(ctx context.Context) ([]string, error) {
	supported := make([]string, 0, len(m.hooks))
	for k := range m.hooks {
		supported = append(supported, k)
	}
	return supported, nil
}

var _ taskengine.HookRepo = (*SimpleRepo)(nil)
