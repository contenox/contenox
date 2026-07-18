// Package agentregistryapi exposes the declared-agents registry
// (runtime/agentregistryservice) over REST. It is deliberately read-only in
// this slice: listing and looking up registered agents is served here, while
// registration (create/update/delete) stays with the `contenox agent` CLI.
//
// The route/handler shape mirrors runtime/internal/mcpserverapi so the two
// declared-resource registries stay easy to compare, and the `// @request` /
// `// @response` / param-description annotations are what the OpenAPI generator
// (tools/openapi-gen) scans for.
package agentregistryapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/agentregistryservice"
)

// AddAgentRegistryRoutes registers the read-only agent registry routes on mux.
func AddAgentRegistryRoutes(mux *http.ServeMux, svc agentregistryservice.Service) {
	h := &agentHandler{svc: svc}

	mux.HandleFunc("GET /agents", h.list)
	mux.HandleFunc("GET /agents/by-name/{name}", h.getByName)
	mux.HandleFunc("GET /agents/{id}", h.get)
}

type agentHandler struct {
	svc agentregistryservice.Service
}

func (h *agentHandler) list(w http.ResponseWriter, r *http.Request) {
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
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []*runtimetypes.Agent
}

func (h *agentHandler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique ID of the agent.")
	agent, err := h.svc.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, agent) // @response runtimetypes.Agent
}

func (h *agentHandler) getByName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := apiframework.GetPathParam(r, "name", "The unique name of the agent.")
	agent, err := h.svc.GetByName(ctx, name)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, agent) // @response runtimetypes.Agent
}
