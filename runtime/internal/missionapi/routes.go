// Package missionapi exposes mission records (runtime/missionservice) over REST:
// the durable manifest half of the fleet manager. Missions are the operator's
// one-line intent bound to work — created, listed, edited, and bound to the
// sessions/instances they spawn.
//
// The route/handler shape mirrors runtime/internal/agentregistryapi and
// runtime/internal/fleetapi so the fleet surfaces stay easy to compare, and the
// `// @request` / `// @response` / param-description annotations are what the
// OpenAPI generator (tools/openapi-gen) scans for.
package missionapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

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

// AddRoutes registers the mission CRUD routes on mux.
func AddRoutes(mux *http.ServeMux, svc missionservice.Service) {
	h := &missionHandler{svc: svc}

	mux.HandleFunc("GET /missions", h.list)
	mux.HandleFunc("POST /missions", h.create)
	mux.HandleFunc("GET /missions/{id}", h.get)
	mux.HandleFunc("PATCH /missions/{id}", h.patch)
	mux.HandleFunc("DELETE /missions/{id}", h.delete)
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

	limitStr := apiframework.GetQueryParam(r, "limit", "100", "Maximum number of items to return.")
	cursorStr := apiframework.GetQueryParam(r, "cursor", "", "RFC3339Nano timestamp for pagination cursor.")

	var cursor *time.Time
	if cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			_ = apiframework.Error(w, r, fmt.Errorf("invalid cursor: %w", err), apiframework.ListOperation)
			return
		}
		cursor = &t
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("invalid limit: %w", err), apiframework.ListOperation)
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
