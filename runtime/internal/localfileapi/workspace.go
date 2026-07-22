package localfileapi

import (
	"fmt"
	"net/http"
	"sync"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/vfs"
)

// AddWorkspaceRoutes registers the /files browse API in per-root mode: every
// request names its workspace via the `root` query parameter (or the request
// body's `root` field for writes), which is validated against the allowlist
// before a rooted localfileservice serves it. The empty value and "/" resolve
// to the default root, matching the session cwd compat rule. Requests for a
// non-allowlisted root are rejected — a browser can browse any allowlisted root
// but nothing outside the allowlist.
//
// hitlFor, when non-nil, enables the `agent` view filter on GET /files: it builds
// a HITL service bound to the requested (or default) policy so per-entry access
// verdicts are computed by the agent's real policy engine. When nil, requesting
// filter=agent returns an unprocessable-entity error (the filter is not available
// on that deployment) while the unfiltered tree is unchanged.
//
// This is the PRIMARY mount serve uses (serverapi registers it whenever a
// workspace allowlist is configured) and the one the OpenAPI spec documents;
// the single-root AddRoutes mount is the ProjectRoot fallback and is the one
// carrying the exclude directive.
func AddWorkspaceRoutes(mux *http.ServeMux, factory *vfs.Factory, hitlFor PolicyEvaluatorFactory) error {
	if factory == nil {
		return fmt.Errorf("localfileapi: workspace factory is nil")
	}
	wh := &workspaceHandler{
		factory:  factory,
		services: map[string]localfileservice.Service{},
		filters:  defaultFilters(),
		hitlFor:  hitlFor,
	}
	// Warm the cache and fail fast if any allowlisted root cannot be served.
	for _, root := range factory.Roots() {
		if _, err := wh.serviceFor(root); err != nil {
			return fmt.Errorf("localfileapi: workspace root %q: %w", root, err)
		}
	}

	// Method expressions, not closures: `wrap`'s parameter type IS the method
	// expression signature, so closures would be redundant. The OpenAPI
	// generator resolves `wh.wrap((*handler).list)` to the named handler method
	// (annotations, operationId, summary) and additionally derives the `root`
	// query parameter from wrap's own body — so this mount documents the full
	// per-root surface. AddRoutes registers the same method+path pairs for the
	// ProjectRoot fallback and is openapi:exclude'd (a duplicate registration
	// of the same METHOD+path is otherwise a generation error).
	mux.HandleFunc("GET /files", wh.wrap((*handler).list))
	mux.HandleFunc("GET /files/stat", wh.wrap((*handler).stat))
	mux.HandleFunc("GET /files/content", wh.wrap((*handler).content))
	mux.HandleFunc("GET /files/download", wh.wrap((*handler).download))
	mux.HandleFunc("POST /files", wh.wrap((*handler).createFile))
	mux.HandleFunc("PUT /files", wh.wrap((*handler).updateFile))
	mux.HandleFunc("DELETE /files", wh.wrap((*handler).deleteFile))
	mux.HandleFunc("PUT /files/move", wh.wrap((*handler).movePath))
	mux.HandleFunc("POST /folders", wh.wrap((*handler).createFolder))
	mux.HandleFunc("DELETE /folders", wh.wrap((*handler).deleteFolder))
	return nil
}

type workspaceHandler struct {
	factory *vfs.Factory

	// filters is the shared filter registry; hitlFor builds the policy engine a
	// filter needs. Both are shared across roots; the per-root view is bound in
	// wrap.
	filters map[string]FileFilter
	hitlFor PolicyEvaluatorFactory

	mu       sync.Mutex
	services map[string]localfileservice.Service
}

// serviceFor returns a cached localfileservice rooted at the resolved
// (allowlisted) root.
func (wh *workspaceHandler) serviceFor(resolvedRoot string) (localfileservice.Service, error) {
	wh.mu.Lock()
	defer wh.mu.Unlock()
	if svc, ok := wh.services[resolvedRoot]; ok {
		return svc, nil
	}
	svc, err := localfileservice.New(resolvedRoot)
	if err != nil {
		return nil, err
	}
	wh.services[resolvedRoot] = svc
	return svc, nil
}

// wrap resolves the request's workspace root, rejects a disallowed one, and
// dispatches to the per-root handler.
func (wh *workspaceHandler) wrap(fn func(*handler, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root := apiframework.GetQueryParam(r, "root", "", "Workspace root the request operates in: a granted root (or a directory under one); empty or \"/\" resolves to the default (first-configured) root.")
		resolved, ok := wh.factory.Allows(root)
		if !ok {
			_ = apiframework.Error(w, r,
				fmt.Errorf("%w: workspace root %q is not under any configured workspace root; roots: %s",
					apiframework.ErrUnprocessableEntity, root, wh.factory.DescribeRoots()),
				apiframework.ListOperation)
			return
		}
		svc, err := wh.serviceFor(resolved)
		if err != nil {
			_ = apiframework.Error(w, r, err, apiframework.ListOperation)
			return
		}
		// Bind a root-scoped view for the agent-view filter (reachability gate).
		// A resolution failure leaves view nil; filter=agent then reports itself
		// unavailable rather than serving wrong verdicts.
		view, _ := vfs.OpenView(svc.Root())
		fn(&handler{service: svc, view: view, filters: wh.filters, hitlFor: wh.hitlFor}, w, r)
	}
}
