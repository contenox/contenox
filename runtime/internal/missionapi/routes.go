// Package missionapi exposes mission records (runtime/missionservice) over REST:
// the durable, agent-reportable half of the fleet manager's headless
// interaction model. Missions are the operator's one-line intent, envelope,
// and agent — created, listed, edited, and bound to the session/instance they
// spawn — plus the reports a unit files back while on the mission.
//
// The route/handler shape mirrors runtime/internal/agentregistryapi and
// runtime/internal/fleetapi so the fleet surfaces stay easy to compare, and the
// `// @request` / `// @response` / param-description annotations are what the
// OpenAPI generator (tools/openapi-gen) scans for.
package missionapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/missionservice"
)

// MissionPatch is the PATCH body: any subset of intent, status, and a session
// and/or instance id to bind. Omitted fields are left unchanged.
type MissionPatch struct {
	Intent     *string `json:"intent,omitempty"`
	Status     *string `json:"status,omitempty"`
	SessionID  string  `json:"sessionId,omitempty"`
	InstanceID string  `json:"instanceId,omitempty"`
}

// AddRoutes registers the mission CRUD routes and the mission report routes
// on mux.
func AddRoutes(mux *http.ServeMux, svc missionservice.Service) {
	h := &missionHandler{svc: svc}

	mux.HandleFunc("GET /missions", h.list)
	mux.HandleFunc("POST /missions", h.create)
	mux.HandleFunc("GET /missions/{id}", h.get)
	mux.HandleFunc("PATCH /missions/{id}", h.patch)
	mux.HandleFunc("DELETE /missions/{id}", h.delete)
	mux.HandleFunc("POST /missions/{id}/reports", h.addReport)
	mux.HandleFunc("GET /missions/{id}/reports", h.listReports)
}

type missionHandler struct {
	svc missionservice.Service
}

func (h *missionHandler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, err := apiframework.Decode[missionservice.Mission](r) // @request missionservice.Mission
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if err := h.svc.Create(ctx, &m); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, m) // @response missionservice.Mission
}

func (h *missionHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cursor, limit, err := apiframework.ListParams(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	items, err := h.svc.List(ctx, cursor, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []*missionservice.Mission
}

func (h *missionHandler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	m, err := h.svc.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, m) // @response missionservice.Mission
}

func (h *missionHandler) patch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	p, err := apiframework.Decode[MissionPatch](r) // @request missionapi.MissionPatch
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	m, err := h.svc.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	if p.Intent != nil || p.Status != nil {
		if p.Intent != nil {
			m.Intent = *p.Intent
		}
		if p.Status != nil {
			m.Status = missionservice.Status(*p.Status)
		}
		if err := h.svc.Update(ctx, m); err != nil {
			_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
			return
		}
	}

	if p.SessionID != "" || p.InstanceID != "" {
		m, err = h.svc.Bind(ctx, id, p.SessionID, p.InstanceID)
		if err != nil {
			_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
			return
		}
	}

	_ = apiframework.Encode(w, r, http.StatusOK, m) // @response missionservice.Mission
}

func (h *missionHandler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	if err := h.svc.Delete(ctx, id); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, "deleted") // @response string
}

// addReport files a report against mission id. The path id is authoritative:
// missionservice.Service.AddReport binds it onto the decoded report
// regardless of what the body's missionId field says. An unknown mission id
// surfaces as 404, not a silent insert; an invalid Kind or a multi-line
// Summary surfaces as 422 via the same validated-CRUD error mapping every
// other mutation in this package uses.
func (h *missionHandler) addReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")
	rep, err := apiframework.Decode[missionservice.Report](r) // @request missionservice.Report
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if err := h.svc.AddReport(ctx, id, &rep); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, rep) // @response missionservice.Report
}

// listReports returns mission id's reports newest-first.
func (h *missionHandler) listReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the mission.")

	limit, err := apiframework.LimitParam(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	items, err := h.svc.ListReports(ctx, id, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []*missionservice.Report
}
