// Package fleetapi exposes runtime/fleetservice — the fleet's operational
// surface — over REST: the config+runtime join of every declared agent
// annotated with its running instances, per-instance status lookup, dispatch
// (bring an instance up, open a session, and run the intent as the unit's
// first turn detached — every dispatch is a mission), and now stop/cancel as
// first-class operations.
//
// Every handler here is THIN: it decodes the request, calls straight into
// fleetservice.Service, and maps the result/error onto the wire. All
// lifecycle POLICY (the Enabled check, teardown-on-failure, cancel fan-out,
// ...) lives in fleetservice so it cannot drift between this REST path and
// the `contenox fleet` CLI (a follow-up slice) mounted on the same Service.
//
// The route/handler shape mirrors runtime/internal/agentregistryapi and
// runtime/internal/missionapi (the declared and durable halves of the same
// fleet), and the `// @request` / `// @response` annotations are what the
// OpenAPI generator (tools/openapi-gen) scans for.
package fleetapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/fleetservice"
)

// DispatchRequest is the POST /fleet/dispatch body. It is a type alias onto
// fleetservice.DispatchRequest — fleetservice is the single source of truth
// for the shape; this package never re-declares it.
type DispatchRequest = fleetservice.DispatchRequest

// DispatchResponse is the 202 body: the ids the dispatch created. It is a
// type alias onto fleetservice.DispatchResult.
type DispatchResponse = fleetservice.DispatchResult

// CancelRequest is the optional POST /fleet/{instanceID}/cancel body. An
// absent body or an empty/omitted sessionId cancels every session currently
// attached to the instance (see fleetservice.Service.Cancel).
type CancelRequest struct {
	SessionID string `json:"sessionId,omitempty"`
}

// AddRoutes registers the fleet routes on mux.
func AddRoutes(mux *http.ServeMux, fleet fleetservice.Service) {
	h := &fleetHandler{fleet: fleet}

	mux.HandleFunc("GET /fleet", h.list)
	mux.HandleFunc("POST /fleet/dispatch", h.dispatch)
	mux.HandleFunc("GET /fleet/{instanceID}", h.get)
	mux.HandleFunc("DELETE /fleet/{instanceID}", h.stop)
	mux.HandleFunc("POST /fleet/{instanceID}/cancel", h.cancel)
}

type fleetHandler struct {
	fleet fleetservice.Service
}

// list returns every fleet instance the kernel currently tracks.
func (h *fleetHandler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.fleet.List(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entries) // @response []agentinstance.FleetEntry
}

// get returns the live status of one fleet instance.
func (h *fleetHandler) get(w http.ResponseWriter, r *http.Request) {
	id := apiframework.GetPathParam(r, "instanceID", "The unique ID of the instance.")
	status, err := h.fleet.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, agentinstance.ErrNotFound) {
			err = apiframework.NotFound(err.Error())
		}
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, status) // @response agentinstance.InstanceStatus
}

// dispatch allocates a unit via fleetservice.Dispatch and returns 202 with the
// ids as soon as the session is open; the orchestration (Enabled check,
// teardown-on-failure, the mission record, the detached first turn) lives
// entirely in the service now.
func (h *fleetHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, err := apiframework.Decode[DispatchRequest](r) // @request fleetapi.DispatchRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	result, err := h.fleet.Dispatch(ctx, req)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusAccepted, result) // @response fleetapi.DispatchResponse
}

// stop tears an instance down via fleetservice.Stop, which is idempotent by
// kernel contract: stopping an unknown or already-stopped instance is a no-op
// 200, not a 404 — mirrors the delete-response idiom missionapi/mcpserverapi
// use (a plain "deleted" string body).
func (h *fleetHandler) stop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "instanceID", "The unique ID of the instance.")
	if err := h.fleet.Stop(ctx, id); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, "deleted") // @response string
}

// cancel cancels an in-flight prompt turn via fleetservice.Cancel. The body
// is OPTIONAL: an absent body, an empty body, or an omitted/empty sessionId
// all mean "cancel every session on this instance" (see CancelRequest and
// fleetservice.Service.Cancel) — read manually rather than through
// apiframework.Decode so a caller need not send `{}` for the common case.
func (h *fleetHandler) cancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "instanceID", "The unique ID of the instance.")

	var req CancelRequest // @request fleetapi.CancelRequest
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("read request body: %w", err), apiframework.UpdateOperation)
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			_ = apiframework.Error(w, r, fmt.Errorf("%w: %w", apiframework.ErrDecodeInvalidJSON, err), apiframework.UpdateOperation)
			return
		}
	}

	if err := h.fleet.Cancel(ctx, id, req.SessionID); err != nil {
		if errors.Is(err, agentinstance.ErrNotFound) {
			err = apiframework.NotFound(err.Error())
		}
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, "cancelled") // @response string
}
