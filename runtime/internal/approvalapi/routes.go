package approvalapi

import (
	"context"
	"net/http"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/runtime/hitlservice"
)

func AddRoutes(mux *http.ServeMux, svc hitlservice.Service, auth middleware.AuthZReader) {
	h := &handler{svc: svc, auth: auth}
	mux.HandleFunc("POST /approvals/{approvalId}", h.respond)
}

type handler struct {
	svc  hitlservice.Service
	auth middleware.AuthZReader
}

type respondBody struct {
	Approved bool `json:"approved"`
}

func (h *handler) authorize(ctx context.Context) error {
	if h.auth == nil {
		return nil
	}
	_, err := h.auth.GetIdentity(ctx)
	return err
}

func (h *handler) respond(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r.Context()); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.svc == nil {
		_ = apiframework.Error(w, r, apiframework.NotFound("approval service is not configured"), apiframework.CreateOperation)
		return
	}

	approvalID := apiframework.GetPathParam(r, "approvalId", "The UUID of the pending HITL approval request.")
	if approvalID == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("approvalId is required"), apiframework.CreateOperation)
		return
	}

	body, err := apiframework.Decode[respondBody](r) // @request approvalapi.respondBody
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	if !h.svc.Respond(approvalID, body.Approved) {
		_ = apiframework.Error(w, r, apiframework.NotFound("approval not found or already resolved"), apiframework.CreateOperation)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
