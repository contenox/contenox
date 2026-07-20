// Package fleetapi exposes the live agent-instance fleet
// (runtime/agentinstance) over REST: the config+runtime join of every declared
// agent annotated with its running instances, per-instance status lookup, and
// dispatch (bring an instance up, open a session, and — for a supplied prompt —
// run the first turn detached).
//
// The read routes are deliberately the only lifecycle-free half; dispatch is the
// one write path (the "future scheduler (cron/bus → Start)" seam the kernel docs
// reserve), and Stop/restart stay with the Manager's owner, `contenox serve`.
//
// The route/handler shape mirrors runtime/internal/agentregistryapi and
// runtime/internal/missionapi (the declared and durable halves of the same
// fleet), and the `// @request` / `// @response` annotations are what the OpenAPI
// generator (tools/openapi-gen) scans for.
package fleetapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/vfs"
)

// DispatchRequest is the POST /fleet/dispatch body: the declared agent to bring
// up, an optional first prompt, an optional one-line mission intent, and an
// optional session working directory (validated against the workspace-root
// allowlist).
type DispatchRequest struct {
	AgentName     string `json:"agentName"`
	Prompt        string `json:"prompt,omitempty"`
	MissionIntent string `json:"missionIntent,omitempty"`
	Cwd           string `json:"cwd,omitempty"`
}

// DispatchResponse is the 202 body: the ids the dispatch created. MissionID is
// present only when a mission intent was supplied (explicit-only in v1).
type DispatchResponse struct {
	InstanceID string `json:"instanceId"`
	SessionID  string `json:"sessionId"`
	MissionID  string `json:"missionId,omitempty"`
}

// AddRoutes registers the fleet routes on mux. missions, workspaceRoots and
// projectRoot are dispatch-only and may be zero: dispatch still works with a nil
// mission registry as long as no missionIntent is supplied (validated per
// request). A nil tracker degrades to a Noop, so the async first-prompt outcome
// is simply not recorded rather than panicking.
func AddRoutes(
	mux *http.ServeMux,
	instances agentinstance.Manager,
	missions missionservice.Service,
	workspaceRoots *vfs.Factory,
	projectRoot string,
	tracker libtracker.ActivityTracker,
) {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	h := &fleetHandler{
		instances:      instances,
		missions:       missions,
		workspaceRoots: workspaceRoots,
		projectRoot:    projectRoot,
		tracker:        tracker,
	}

	mux.HandleFunc("GET /fleet", h.list)
	mux.HandleFunc("POST /fleet/dispatch", h.dispatch)
	mux.HandleFunc("GET /fleet/{instanceID}", h.get)
}

type fleetHandler struct {
	instances      agentinstance.Manager
	missions       missionservice.Service
	workspaceRoots *vfs.Factory
	projectRoot    string
	tracker        libtracker.ActivityTracker
}

func (h *fleetHandler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.instances.List(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entries) // @response []agentinstance.FleetEntry
}

func (h *fleetHandler) get(w http.ResponseWriter, r *http.Request) {
	id := apiframework.GetPathParam(r, "instanceID", "The unique ID of the instance.")
	status, err := h.instances.Get(id)
	if err != nil {
		if errors.Is(err, agentinstance.ErrNotFound) {
			err = apiframework.NotFound(err.Error())
		}
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, status) // @response agentinstance.InstanceStatus
}

// dispatch allocates a unit: it brings up an instance for a declared agent, opens
// a session, optionally records a mission bound to both ids, and — for a supplied
// prompt — runs the first turn on a detached context, returning 202 with the ids
// immediately (async-after-OpenSession; the prompt's outcome is observable on the
// board). It is allocation, not operation: no restart policy, no adoption into a
// beam chat session (a documented v1 limitation).
func (h *fleetHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, err := apiframework.Decode[DispatchRequest](r) // @request fleetapi.DispatchRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if strings.TrimSpace(req.AgentName) == "" {
		_ = apiframework.Error(w, r, apiframework.MissingParameter("agentName", "agentName is required"), apiframework.CreateOperation)
		return
	}
	// Explicit-only missions in v1: an intent is meaningless without a wired
	// registry to record it, so reject the combination rather than silently
	// dropping the intent.
	if req.MissionIntent != "" && h.missions == nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("missionIntent given but the mission registry is not configured"), apiframework.CreateOperation)
		return
	}
	// cwd envelope discipline: a requested cwd must resolve within an allowlisted
	// workspace root; an absent one defaults to the same root the session path
	// uses. This mirrors acpsvc.Transport.resolveWorkspaceCwd.
	cwd, err := h.resolveCwd(req.Cwd)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	// 1. Bring up an instance for the declared agent. Start wraps libdb.ErrNotFound
	// for an unknown agent, which the error mapper renders as 404 — the same idiom
	// the read path's Get relies on.
	instanceID, err := h.instances.Start(ctx, req.AgentName)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	// 2. Open a session on the instance. On failure tear the fresh instance down so
	// a failed dispatch never leaks a running subprocess (the acpsvc contract).
	sessionID, err := h.instances.OpenSession(ctx, instanceID, agentinstance.SessionSpec{Cwd: cwd})
	if err != nil {
		_ = h.instances.Stop(instanceID)
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	resp := DispatchResponse{InstanceID: instanceID, SessionID: string(sessionID)}

	// 3. Explicit mission: create the record, then bind both ids to it.
	if req.MissionIntent != "" {
		m := &missionservice.Mission{Intent: req.MissionIntent, AgentName: req.AgentName}
		if err := h.missions.Create(ctx, m); err != nil {
			_ = h.instances.Stop(instanceID)
			_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
			return
		}
		if _, err := h.missions.Bind(ctx, m.ID, string(sessionID), instanceID); err != nil {
			_ = h.instances.Stop(instanceID)
			_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
			return
		}
		resp.MissionID = m.ID
	}

	// 4. First prompt runs detached: dispatch ACCEPTS and returns ids; the turn's
	// outcome is observable on the board and recorded through the tracker (never
	// swallowed). context.WithoutCancel keeps request-scoped values (request id)
	// while surviving the handler's return. Payload discipline: ids and stop
	// reason only, never prompt content.
	if strings.TrimSpace(req.Prompt) != "" {
		detached := context.WithoutCancel(ctx)
		blocks := []libacp.ContentBlock{libacp.NewTextContent(req.Prompt)}
		go func() {
			reportErr, reportChange, end := h.tracker.Start(detached, "prompt", "fleet_dispatch",
				"instance_id", instanceID, "session_id", string(sessionID), "agent_name", req.AgentName)
			defer end()
			stop, err := h.instances.Prompt(detached, instanceID, sessionID, blocks)
			if err != nil {
				reportErr(err)
				return
			}
			reportChange(string(sessionID), string(stop))
		}()
	}

	_ = apiframework.Encode(w, r, http.StatusAccepted, resp) // @response fleetapi.DispatchResponse
}

// resolveCwd validates a requested session cwd against the workspace-root
// allowlist and returns the concrete root the session will use, mirroring the
// minimal shape of acpsvc.Transport.resolveWorkspaceCwd. When an allowlist is
// configured (serve), the sentinel "/" and an empty cwd resolve to the default
// root, and any other value must be an allowlisted root (else a 400). When none
// is configured, an explicit cwd is kept as-is and an absent one defaults to the
// fixed project root — the same default the session path uses.
func (h *fleetHandler) resolveCwd(cwd string) (string, error) {
	if h.workspaceRoots != nil {
		resolved, err := h.workspaceRoots.Resolve(cwd)
		if err != nil {
			return "", apiframework.InvalidParameterValue("cwd",
				fmt.Sprintf("workspace directory %q is not permitted; choose one of the configured workspace roots", cwd))
		}
		return resolved, nil
	}
	if strings.TrimSpace(cwd) == "" {
		return h.projectRoot, nil
	}
	return cwd, nil
}
