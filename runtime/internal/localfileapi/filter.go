package localfileapi

import (
	"context"

	"github.com/contenox/runtime/runtime/agentview"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/localfileservice"
)

// Entry is a /files list element: the raw filesystem entry plus optional,
// filter-contributed annotations. localfileservice.Entry is embedded (anonymous,
// untagged) so its fields promote to the top level and the base JSON shape stays
// byte-identical to the unfiltered response; a filter only ever ADDS fields such
// as `access`, which stay absent (omitempty) when no filter runs.
type Entry struct {
	localfileservice.Entry
	// Access is the agent-view verdict for this path; present only under the
	// `agent` filter.
	Access *agentview.Verdict `json:"access,omitempty"`
}

// FileFilter transforms a listing into an annotated and/or narrowed listing. The
// `filter` query param on GET /files selects one by Name(). Filters are the
// extension point for future views (gitignore, filetype, modified-since): each
// registers as a named implementation and may add its own optional Entry fields.
// The evaluator is supplied so a filter that needs per-path agent verdicts can
// compute them without reimplementing the agent's gates.
type FileFilter interface {
	Name() string
	Apply(ctx context.Context, entries []Entry, ev *agentview.Evaluator) ([]Entry, error)
}

// PolicyEvaluatorFactory builds a HITL service bound to a specific policy name.
// An empty policyName selects the runtime's default policy resolution (the same
// resolution the live agent uses). Non-empty forces that exact named policy.
type PolicyEvaluatorFactory func(policyName string) hitlservice.Service

// defaultFilters is the registry of built-in filters. MVP registers only the
// agent view; adding a filter type is a single map entry.
func defaultFilters() map[string]FileFilter {
	return map[string]FileFilter{
		agentFilter{}.Name(): agentFilter{},
	}
}

// agentFilter annotates every entry with the agent's access verdict. It is an
// ANNOTATED filter: unreachable entries are returned marked reachable:false, not
// omitted, so the caller sees the sandbox boundary rather than just its inside.
type agentFilter struct{}

func (agentFilter) Name() string { return "agent" }

func (agentFilter) Apply(ctx context.Context, entries []Entry, ev *agentview.Evaluator) ([]Entry, error) {
	out := make([]Entry, len(entries))
	for i, e := range entries {
		v := ev.Verdict(ctx, e.Path, e.IsDirectory)
		e.Access = &v
		out[i] = e
	}
	return out, nil
}
