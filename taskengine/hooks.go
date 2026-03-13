package taskengine

import (
	"context"
	"strings"
)

// resolveHookNames returns the effective set of hook names for a task based on its allowlist.
//
// Semantics:
//   - nil allowlist  → all names from provider.Supports() (field was absent; backward compat)
//   - []             → empty set (field explicitly set to empty; no hooks)
//   - ["*"]          → all names from provider.Supports()
//   - ["a","b"]      → intersection of the named entries with Supports()
//   - ["*","!name"]  → all from Supports() minus the excluded names
//
// Entries starting with "!" are exclusions and may only be combined with "*".
// Unknown exact names (not returned by Supports) are silently ignored.
func resolveHookNames(ctx context.Context, allowlist []string, provider HookProvider) ([]string, error) {
	// nil means the field was absent — expose everything (backward compat).
	if allowlist == nil {
		return provider.Supports(ctx)
	}

	// Explicitly empty — no hooks.
	if len(allowlist) == 0 {
		return []string{}, nil
	}

	// Separate positives from exclusions.
	hasStar := false
	exact := make(map[string]struct{})
	excluded := make(map[string]struct{})

	for _, entry := range allowlist {
		if entry == "*" {
			hasStar = true
		} else if strings.HasPrefix(entry, "!") {
			excluded[strings.TrimPrefix(entry, "!")] = struct{}{}
		} else {
			exact[entry] = struct{}{}
		}
	}

	all, err := provider.Supports(ctx)
	if err != nil {
		return nil, err
	}

	// Build result set.
	result := make([]string, 0, len(all))
	for _, name := range all {
		if _, skip := excluded[name]; skip {
			continue
		}
		if hasStar {
			result = append(result, name)
			continue
		}
		if _, ok := exact[name]; ok {
			result = append(result, name)
		}
	}
	return result, nil
}

// ExportedResolveHookNames is a test-only export of resolveHookNames.
func ExportedResolveHookNames(ctx context.Context, allowlist []string, provider HookProvider) ([]string, error) {
	return resolveHookNames(ctx, allowlist, provider)
}

