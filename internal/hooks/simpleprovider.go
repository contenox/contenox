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

// Exec now implements the new, simplified HookRepo interface.
func (m *SimpleRepo) Exec(
	ctx context.Context,
	startingTime time.Time,
	input any,
	args *taskengine.HookCall,
) (any, error) {
	if hook, ok := m.hooks[args.Name]; ok {
		// Delegate the call using the new signature.
		return hook.Exec(ctx, startingTime, input, args)
	}
	return nil, fmt.Errorf("unknown hook type: %s", args.Name)
}

func (m *SimpleRepo) Supports(ctx context.Context) ([]string, error) {
	supported := make([]string, 0, len(m.hooks))
	for k := range m.hooks {
		supported = append(supported, k)
	}
	return supported, nil
}

var _ taskengine.HookRepo = (*SimpleRepo)(nil)
