// Package operatorinboxapi exposes the operator attention inbox
// (runtime/operatorinbox) over REST: mission reports that reached no live
// supervising session — an operator-fired mission's reports, and reports whose
// parent session was gone by the time they arrived. It is the read surface an
// operator (or the beam inbox slice) polls to see "what came back from the
// missions I fired directly," the sibling of the approval inbox (approvalapi)
// for notices that need eyes rather than a decision.
//
// The route/handler shape mirrors runtime/internal/missionapi so the fleet
// surfaces stay easy to compare; the `// @response` annotation is what the
// OpenAPI generator scans for.
package operatorinboxapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/operatorinbox"
)

// AddRoutes registers the operator-inbox read route on mux.
func AddRoutes(mux *http.ServeMux, svc operatorinbox.Service) {
	h := &inboxHandler{svc: svc}
	mux.HandleFunc("GET /operator-inbox", h.list)
}

type inboxHandler struct {
	svc operatorinbox.Service
}

// list returns the operator inbox newest-first: reports that reached no live
// supervisor. An empty inbox renders as [], never null.
func (h *inboxHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, err := apiframework.LimitParam(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	items, err := h.svc.List(ctx, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []*operatorinbox.Item
}
