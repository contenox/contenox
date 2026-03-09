package taskengine

import (
	"context"
	"fmt"
)

type templateVarsKey struct{}

// WithTemplateVars attaches a map of template variables to the context.
// MacroEnv expands {{var:name}} from this map. The engine never reads os.Getenv;
// callers (e.g. Contenox CLI, API) build the map and attach it here.
func WithTemplateVars(ctx context.Context, vars map[string]string) context.Context {
	if vars == nil {
		return ctx
	}
	return context.WithValue(ctx, templateVarsKey{}, vars)
}

// TemplateVarsFromContext returns the template variables map from the context.
// Returns nil if not set; a nil map is safe to read (key lookup returns false).
// MacroEnv will return an error for any {{var:key}} whose key is absent.
func TemplateVarsFromContext(ctx context.Context) (map[string]string, error) {
	v, ok := ctx.Value(templateVarsKey{}).(map[string]string)
	if !ok {
		return nil, fmt.Errorf("template vars not set in context")
	}
	return v, nil
}
