// Package accessapi mounts the workspace access-preview API: given a batch of
// workspace-root-relative path strings, it returns each path enriched with the
// HITL policy's read/write verdict — the batch, structured-reason HTTP surface
// over runtime/accessview (see that package for why it is a distinct thing from
// runtime/agentview's per-entry, UI-quiet verdicts).
//
// Bare paths in is deliberate: the client never asserts isDir or reachability —
// the server derives both from its own per-root vfs.View, so a malformed or
// dishonest client payload can at worst yield reachable:false for a path, never
// a corrupted verdict for one that IS reachable.
package accessapi

import (
	"fmt"
	"net/http"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/accessview"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/vfs"
)

// maxBatchPaths bounds one request's path list. It is a defensive cap (a
// pasted blob of thousands of paths must not be accepted as one request), not
// a tuning knob — the same register as localfileapi's find/search caps.
const maxBatchPaths = 2000

// EvaluateRequest is the POST /workspace/access request body: the paths to
// evaluate, workspace-root-relative. Paths is optional — an omitted or empty
// list is a valid, if uninteresting, batch (200 with an empty verdict list).
type EvaluateRequest struct {
	Paths []string `json:"paths"`
}

// EvaluateResponse is the POST /workspace/access response: the policy that was
// evaluated (its resolved name — see accessview.Evaluator.Evaluate) and the
// per-path verdicts, in the same order as the request's Paths.
type EvaluateResponse struct {
	PolicyName string                   `json:"policyName"`
	Verdicts   []accessview.PathVerdict `json:"verdicts"`
}

// AddRoutes registers POST /workspace/access on mux. It mirrors the root ->
// view and policy-factory resolution the /files, /workspace/search, and
// /workspace/find APIs use (see localfileapi.AddWorkspaceRoutes /
// AddWorkspaceFindRoutes): `root` is validated through the same *vfs.Factory
// allowlist, and hitlFor builds the HITL service bound to the requested (or
// default-resolved) policy.
//
// Nil-gated like the other optional workspace route groups: with no workspace
// allowlist configured (factory nil), nothing is registered. A nil hitlFor
// registers the route but every request answers 422 (evaluation unavailable on
// this deployment) — the same shape localfileapi's filter=agent uses when its
// own hitlFor is nil.
func AddRoutes(mux *http.ServeMux, factory *vfs.Factory, hitlFor localfileapi.PolicyEvaluatorFactory) {
	if factory == nil {
		return
	}
	h := &handler{factory: factory, hitlFor: hitlFor}
	mux.HandleFunc("POST /workspace/access", h.evaluate)
}

type handler struct {
	factory *vfs.Factory
	hitlFor localfileapi.PolicyEvaluatorFactory
}

// evaluate batch-evaluates the HITL policy's read/write verdict for every path
// in the request body against the `root` workspace (query param, same
// allowlist as /files) and `policy` (query param; omitted uses the runtime's
// default policy resolution, matching the live agent).
func (h *handler) evaluate(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[EvaluateRequest](r) // @request accessapi.EvaluateRequest
	if err != nil {
		// A malformed body is a client input error distinct from an
		// unprocessable-but-well-formed one (the batch-cap and root/policy
		// refusals below): surfaced as 400, not the 422 an empty Content-Type
		// default would otherwise fall through to.
		_ = apiframework.Error(w, r, fmt.Errorf("%w: %v", apiframework.ErrBadRequest, err), apiframework.CreateOperation)
		return
	}
	if len(req.Paths) > maxBatchPaths {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: batch of %d paths exceeds the %d-path limit", apiframework.ErrUnprocessableEntity, len(req.Paths), maxBatchPaths),
			apiframework.CreateOperation)
		return
	}

	root := apiframework.GetQueryParam(r, "root", "", "Workspace root the request operates in: a granted root (or a directory under one); empty or \"/\" resolves to the default (first-configured) root.")
	resolved, ok := h.factory.Allows(root)
	if !ok {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: workspace root %q is not under any configured workspace root; roots: %s",
				apiframework.ErrUnprocessableEntity, root, h.factory.DescribeRoots()),
			apiframework.CreateOperation)
		return
	}
	if h.hitlFor == nil {
		_ = apiframework.Error(w, r,
			fmt.Errorf("%w: workspace access evaluation is not available on this deployment", apiframework.ErrUnprocessableEntity),
			apiframework.CreateOperation)
		return
	}

	view, err := vfs.OpenView(resolved)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ServerOperation)
		return
	}

	// policy names the HITL policy to evaluate against; omitted -> the
	// runtime's default resolution (matching the live agent and the /files
	// filter=agent view).
	policyName := strings.TrimSpace(apiframework.GetQueryParam(r, "policy", "", "HITL policy name to evaluate against; omitted uses the runtime's default policy resolution (the same resolution the live agent uses)."))

	// The evaluator/policy binding is built ONCE here and reused for every path
	// in req.Paths (see accessview.Evaluator.Evaluate) — never rebuilt per path.
	ev := accessview.NewEvaluator(view, h.hitlFor(policyName))
	resolvedPolicy, verdicts := ev.Evaluate(r.Context(), req.Paths)

	_ = apiframework.Encode(w, r, http.StatusOK, EvaluateResponse{ // @response accessapi.EvaluateResponse
		PolicyName: resolvedPolicy,
		Verdicts:   verdicts,
	})
}
