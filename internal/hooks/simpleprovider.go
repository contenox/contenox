package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/contenox/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

// SimpleRepo holds a map of locally registered hooks.
type SimpleRepo struct {
	hooks map[string]taskengine.HookRepo
}

func NewSimpleProvider(hooks map[string]taskengine.HookRepo) taskengine.HookRepo {
	return &SimpleRepo{
		hooks: hooks,
	}
}

func (m *SimpleRepo) Exec(
	ctx context.Context,
	startingTime time.Time,
	input any,
	args *taskengine.HookCall,
) (any, error) {
	if hook, ok := m.hooks[args.Name]; ok {
		return hook.Exec(ctx, startingTime, input, args)
	}
	return nil, fmt.Errorf("unknown hook type: %s", args.Name)
}

// Supports returns a list of all hook names registered in the internal map.
func (m *SimpleRepo) Supports(ctx context.Context) ([]string, error) {
	supported := make([]string, 0, len(m.hooks))
	for k := range m.hooks {
		supported = append(supported, k)
	}
	return supported, nil
}

// GetSchemasForSupportedHooks aggregates the schemas from all registered hooks.
func (m *SimpleRepo) GetSchemasForSupportedHooks(ctx context.Context) (map[string]*openapi3.T, error) {
	return nil, nil
	// allSchemas := make(map[string]*openapi3.T)

	// // Iterate through each registered hook implementation.
	// for hookName, hookImpl := range m.hooks {
	// 	// Get the schemas provided by this specific hook's implementation.
	// 	implSchemas, err := hookImpl.GetSchemasForSupportedHooks(ctx)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("error getting schema for hook '%s': %w", hookName, err)
	// 	}

	// 	// Merge the returned schemas into our main map.
	// 	for k, v := range implSchemas {
	// 		allSchemas[k] = v // cannot use v (variable of type map[string]interface{}) as *openapi3.T value in assignment (compiler IncompatibleAssign)
	// 	}
	// }
	// return allSchemas, nil
}

func (m *SimpleRepo) GetToolsForHookByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	if hook, ok := m.hooks[name]; ok {
		return hook.GetToolsForHookByName(ctx, name)
	}
	return nil, fmt.Errorf("unknown hook type: %s", name)
}

var _ taskengine.HookRepo = (*SimpleRepo)(nil)
