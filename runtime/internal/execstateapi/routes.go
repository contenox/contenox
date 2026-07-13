// Package execstateapi exposes the captured per-run execution state
// (taskengine.CapturedStateUnit) over HTTP, keyed by request ID.
//
// The engine's KVInspector already persists the sanitized step stream for
// every run in the SQLite KV store; this endpoint makes that durable
// evidence reachable for clients that want to re-render a past turn's work
// log (the console scrollback). It is the HTTP twin of `contenox state show`.
package execstateapi

import (
	"net/http"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	libdb "github.com/contenox/runtime/libdbexec"
	libkvstore "github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/taskengine"
)

func AddRoutes(mux *http.ServeMux, db libdb.DBManager, tracker libtracker.ActivityTracker, auth middleware.AuthZReader) {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	kv := libkvstore.NewSQLiteManager(db)
	h := &handler{
		inspector: taskengine.NewKVInspector(taskengine.NewSimpleInspector(), kv, tracker),
		kv:        kv,
		auth:      auth,
	}
	mux.HandleFunc("GET /execution-state", h.get)
	mux.HandleFunc("GET /execution-events", h.getEvents)
}

type handler struct {
	inspector *taskengine.KVInspector
	kv        libkvstore.KVManager
	auth      middleware.AuthZReader
}

type executionStateResponse struct {
	RequestID string                         `json:"requestId"`
	State     []taskengine.CapturedStateUnit `json:"state" openapi_include_type:"taskengine.CapturedStateUnit"`
}

type executionEventsResponse struct {
	RequestID string                 `json:"requestId"`
	Events    []taskengine.TaskEvent `json:"events" openapi_include_type:"taskengine.TaskEvent"`
}

// get returns the captured execution state for a request ID. A run with no
// captured (or already-evicted) state yields an empty list, not an error —
// KV eviction makes absence non-exceptional.
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		if _, err := h.auth.GetIdentity(r.Context()); err != nil {
			_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
			return
		}
	}

	requestID := strings.TrimSpace(apiframework.GetQueryParam(r, "requestId", "", "Request ID of the run whose execution state to fetch.")) // @param requestId string
	if requestID == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("requestId is required"), apiframework.GetOperation)
		return
	}

	state, err := h.inspector.GetExecutionStateByRequestID(r.Context(), requestID)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	if state == nil {
		state = []taskengine.CapturedStateUnit{}
	}

	_ = apiframework.Encode(w, r, http.StatusOK, executionStateResponse{ // @response execstateapi.executionStateResponse
		RequestID: requestID,
		State:     state,
	})
}

// getEvents returns the durably journaled task events of a run — the full
// work log (tool calls, diffs, approvals) that outlives the live SSE stream.
// A run with no journal (or an evicted one) yields an empty list.
func (h *handler) getEvents(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		if _, err := h.auth.GetIdentity(r.Context()); err != nil {
			_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
			return
		}
	}

	requestID := strings.TrimSpace(apiframework.GetQueryParam(r, "requestId", "", "Request ID of the run whose journaled events to fetch.")) // @param requestId string
	if requestID == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("requestId is required"), apiframework.GetOperation)
		return
	}

	events, err := taskengine.GetJournaledEvents(r.Context(), h.kv, requestID)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	if events == nil {
		events = []taskengine.TaskEvent{}
	}

	_ = apiframework.Encode(w, r, http.StatusOK, executionEventsResponse{ // @response execstateapi.executionEventsResponse
		RequestID: requestID,
		Events:    events,
	})
}
