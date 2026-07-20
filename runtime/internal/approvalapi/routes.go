// Package approvalapi exposes pending human-in-the-loop approvals
// (runtime/hitlservice) over REST: the durable ask slice C1
// (docs/development/blueprints/acp/fleet-consolidation.md) introduced now has
// a surface an operator can read and answer without attaching to the session
// that raised it — the inbox slice C2 closes with.
//
// The route/handler shape mirrors runtime/internal/missionapi and
// runtime/internal/fleetapi (thin handlers over apiframework.Encode/Decode/
// Error), and the `// @request` / `// @response` annotations are what the
// OpenAPI generator (tools/openapi-gen) scans for.
package approvalapi

import (
	"errors"
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/hitlservice"
)

// AnswerRequest is the POST /approvals/{id} body: the human's yes/no answer
// to a pending ask.
type AnswerRequest struct {
	Approved bool `json:"approved"`
}

// AddRoutes registers the approval-inbox routes on mux.
func AddRoutes(mux *http.ServeMux, svc hitlservice.Service) {
	h := &approvalHandler{svc: svc}

	mux.HandleFunc("GET /approvals", h.list)
	mux.HandleFunc("POST /approvals/{id}", h.answer)
}

type approvalHandler struct {
	svc hitlservice.Service
}

// list returns pending approvals, newest first — the inbox itself. A fleet
// with nothing pending renders an empty JSON array, not an error (see
// hitlservice.Service.ListPending's own non-nil guarantee).
func (h *approvalHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit, err := apiframework.LimitParam(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	items, err := h.svc.ListPending(ctx, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []*runtimetypes.HITLApproval
}

// answer resolves a pending approval. The three ways this can fail are
// mapped honestly rather than collapsed into one status: an unknown id is
// 404 (there is nothing to answer), while an already-resolved ask and an
// expired one are both 409 — the ask existed but can no longer be answered —
// distinguished from each other by message text, since hitlservice.Respond's
// own sentinel errors already say which.
func (h *approvalHandler) answer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the approval.")

	req, err := apiframework.Decode[AnswerRequest](r) // @request approvalapi.AnswerRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	if err := h.svc.Respond(ctx, id, req.Approved); err != nil {
		_ = apiframework.Error(w, r, mapRespondError(err), apiframework.UpdateOperation)
		return
	}

	result := "denied"
	if req.Approved {
		result = "approved"
	}
	_ = apiframework.Encode(w, r, http.StatusOK, result) // @response string
}

// mapRespondError maps hitlservice.Respond's typed errors onto HTTP status
// codes without collapsing the two 409 causes into one message: an
// already-resolved ask and an expired one both mean "this can no longer be
// answered", but an operator needs to know which happened, so the message
// (sourced from the distinct sentinel error) says so even though the status
// code is the same.
func mapRespondError(err error) error {
	switch {
	case errors.Is(err, hitlservice.ErrApprovalNotFound):
		return apiframework.NotFound(err.Error())
	case errors.Is(err, hitlservice.ErrApprovalAlreadyResolved):
		return apiframework.Conflict(err.Error())
	case errors.Is(err, hitlservice.ErrApprovalExpired):
		return apiframework.Conflict(err.Error())
	default:
		return err
	}
}
