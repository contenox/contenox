package agentinstance

import (
	"context"
	"fmt"
	"sync"

	"github.com/contenox/runtime/libacp"
)

// Viewer is a consumer attached to one downstream session of a running
// instance. It is the ATTACHED-VIEWER analogue of go-process-manager's
// io.WriteCloser writers, but ACP-shaped: a viewer both RECEIVES the session's
// streamed updates (Deliver, the fan-out) and — when it is the session's
// controller — ANSWERS the downstream agent's permission requests
// (RequestPermission, the callback the byte-terminal reference has no equivalent
// of). It is defined HERE and typed only on libacp so an implementer (a future
// acpsvc bridge, beam's live view) needs no import from this package beyond the
// interface, and no import cycle can form.
type Viewer interface {
	// ID uniquely identifies this viewer WITHIN a session; it is the key the
	// hub registers under and the id Detach later names. Two viewers on the same
	// session must not share an ID.
	ID() string

	// Deliver receives one downstream session/update for the session this viewer
	// is attached to, both the REPLAYED journal backlog (on attach) and every
	// subsequent LIVE update, in order.
	//
	// It MUST NOT block. Deliver runs on the instance's fan-out path — the ACP
	// read loop for a live update, or the attaching caller's goroutine for the
	// replay — while the session lock is held, so a blocking Deliver stalls every
	// other viewer of the session AND the downstream read loop itself. Enqueue and
	// return; do the slow work (a WebSocket write, a render) elsewhere. The
	// returned error is advisory (logged by the caller at most); it never
	// disturbs the downstream turn.
	Deliver(ctx context.Context, n libacp.SessionNotification) error

	// RequestPermission answers the downstream agent's
	// session/request_permission. The instance invokes it ONLY on the session's
	// controller viewer; an observer implements it to satisfy the interface but
	// it is never called while that viewer is an observer (it WILL be called if
	// the viewer is later promoted to controller — see the hub's detach
	// promotion). Unlike Deliver it runs on its OWN goroutine (ACP dispatches
	// each inbound request separately), so it MAY block awaiting a human decision.
	RequestPermission(ctx context.Context, req libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error)
}

// TerminalServer is an OPTIONAL capability a Viewer MAY also implement to service a
// downstream agent's terminal/* client-callback family (create/output/wait/kill/release) for
// the session it controls. The instance's journaling harness routes an inbound terminal/*
// request to the session's CONTROLLER viewer iff that controller implements TerminalServer; a
// controller that does not — or a session with no controller — answers terminal/* with
// MethodNotFound, exactly as an agent that never advertised the terminal client capability
// expects. Like Deliver/RequestPermission the terminal callbacks run on their own dispatched
// goroutines, so WaitForTerminalExit MAY block until the command exits.
//
// It is the second inbound-callback surface (after RequestPermission) the kernel ROUTES to a
// controller: the byte-terminal reference fans bytes one way, but an ACP downstream also calls
// back. The kernel itself has NO shell dependency — it only routes terminal/* to whoever can
// serve it (an acpsvc bridge maps them onto the runtime's shell sessions). Whether the
// downstream is even TOLD terminals exist is governed separately, by SessionSpec.Terminal at
// OpenSession: the capability is advertised only when the consumer says a terminal server may
// attach.
//
// The method set mirrors the terminal subset of libacp.Client, so an implementer that already
// satisfies libacp.Client (e.g. an acpsvc bridge) satisfies this by construction.
type TerminalServer interface {
	CreateTerminal(ctx context.Context, req libacp.CreateTerminalRequest) (libacp.CreateTerminalResponse, error)
	TerminalOutput(ctx context.Context, req libacp.TerminalOutputRequest) (libacp.TerminalOutputResponse, error)
	WaitForTerminalExit(ctx context.Context, req libacp.WaitForTerminalExitRequest) (libacp.WaitForTerminalExitResponse, error)
	KillTerminal(ctx context.Context, req libacp.KillTerminalRequest) (libacp.KillTerminalResponse, error)
	ReleaseTerminal(ctx context.Context, req libacp.ReleaseTerminalRequest) (libacp.ReleaseTerminalResponse, error)
}

// sessionState is the per-session viewer set, controller, and replay journal —
// the ACP counterpart of a single ProcessPty's writers + cacheBytesBuf, but
// scoped per downstream session rather than per process (one instance multiplexes
// many sessions over one connection).
type sessionState struct {
	journal      *journal
	viewers      map[string]Viewer
	order        []string // attach order, for deterministic controller promotion
	controllerID string   // "" means no controller (permission falls back to deny)
}

// viewerHub is the instance's per-session registry: journal + fan-out + controller
// routing. All access is serialized by mu, so the fan-out (deliver) and a viewer
// attach can never interleave — that mutual exclusion is what makes the replay
// exactly-once and correctly ordered (a viewer either sees an update in its
// replayed backlog OR live, never both, never neither, never out of order).
//
// This tightens the reference, which replayed its byte cache and joined the live
// fan-out without a lock spanning both (an accepted small race for a terminal);
// for structured events we hold the invariant strictly.
type viewerHub struct {
	instanceID  string
	journalSize int

	// onAttach/onDetach are the instance's lifecycle hooks, fired OUTSIDE mu so a
	// sink that calls back into the Manager cannot deadlock the fan-out.
	onAttach func(sessionID libacp.SessionID, viewerID string, controller bool)
	onDetach func(sessionID libacp.SessionID, viewerID string)

	mu       sync.Mutex
	sessions map[libacp.SessionID]*sessionState
}

func newViewerHub(instanceID string, journalSize int) *viewerHub {
	return &viewerHub{
		instanceID:  instanceID,
		journalSize: journalSize,
		sessions:    make(map[libacp.SessionID]*sessionState),
	}
}

// session returns the state for id, creating it on first use. Callers hold mu.
func (h *viewerHub) session(id libacp.SessionID) *sessionState {
	s := h.sessions[id]
	if s == nil {
		s = &sessionState{
			journal: newJournal(h.journalSize),
			viewers: make(map[string]Viewer),
		}
		h.sessions[id] = s
	}
	return s
}

// deliver journals n and fans it out to every viewer of n.SessionID, in arrival
// order — the structured-event form of ProcessPty.readInit's write-to-all-writers
// loop. It runs on the ACP read loop (inline, per libacp's notification
// dispatch), so it relies on the non-blocking Deliver contract.
func (h *viewerHub) deliver(ctx context.Context, n libacp.SessionNotification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.session(n.SessionID)
	s.journal.append(n)
	for _, id := range s.order {
		_ = s.viewers[id].Deliver(ctx, n)
	}
}

// attach registers viewer against sessionID, REPLAYS that session's journal to it
// (ProcessPty.ReadCache), then leaves it in the live fan-out — all under mu so no
// update can slip between replay and join. The first viewer of a session with no
// controller becomes the controller (controllerGranted true); later viewers are
// observers. A viewer id already attached to the session is rejected (mirrors the
// reference's "connection already exists").
func (h *viewerHub) attach(ctx context.Context, sessionID libacp.SessionID, viewer Viewer) (controllerGranted bool, err error) {
	vid := viewer.ID()
	if vid == "" {
		return false, fmt.Errorf("agentinstance: viewer ID is required")
	}

	h.mu.Lock()
	s := h.session(sessionID)
	if _, dup := s.viewers[vid]; dup {
		h.mu.Unlock()
		return false, fmt.Errorf("agentinstance: viewer %q already attached to session %q", vid, sessionID)
	}
	s.viewers[vid] = viewer
	s.order = append(s.order, vid)
	if s.controllerID == "" {
		s.controllerID = vid
		controllerGranted = true
	}
	// Replay the backlog under the lock, so a concurrent live update (which also
	// needs mu) is forced to wait and therefore lands strictly AFTER the replay.
	for _, n := range s.journal.snapshot() {
		_ = viewer.Deliver(ctx, n)
	}
	h.mu.Unlock()

	if h.onAttach != nil {
		h.onAttach(sessionID, vid, controllerGranted)
	}
	return controllerGranted, nil
}

// detach removes viewer vid from sessionID's fan-out. If it was the controller
// and other viewers remain, the earliest-attached survivor is promoted (so a
// session keeps a controller across a controller's departure); if none remain
// the session is dropped entirely. Detaching an unknown viewer/session is an
// error the caller may ignore.
func (h *viewerHub) detach(sessionID libacp.SessionID, viewerID string) error {
	h.mu.Lock()
	s := h.sessions[sessionID]
	if s == nil {
		h.mu.Unlock()
		return fmt.Errorf("agentinstance: session %q has no attached viewers", sessionID)
	}
	if _, ok := s.viewers[viewerID]; !ok {
		h.mu.Unlock()
		return fmt.Errorf("agentinstance: viewer %q not attached to session %q", viewerID, sessionID)
	}
	delete(s.viewers, viewerID)
	for i, id := range s.order {
		if id == viewerID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	// Promote on controller departure: the next-oldest viewer takes control, so a
	// permission request still has an answerer. Control is bound to attachment,
	// not a wall-clock lease — see the package doc's divergence note.
	if s.controllerID == viewerID {
		if len(s.order) > 0 {
			s.controllerID = s.order[0]
		} else {
			s.controllerID = ""
		}
	}
	if len(s.viewers) == 0 {
		delete(h.sessions, sessionID)
	}
	h.mu.Unlock()

	if h.onDetach != nil {
		h.onDetach(sessionID, viewerID)
	}
	return nil
}

// requestPermission routes a downstream session/request_permission to the
// session's controller. It reads the controller under mu then RELEASES the lock
// before calling into it, because the controller's answer may block awaiting a
// human — holding mu there would stall the whole session's fan-out.
//
// Fallback (no controller attached): an UNSUPERVISED DENY, returned as a
// spec-graceful "cancelled" outcome. Denying is the safe default for an instance
// running headless — nothing gets to perform a permission-gated action with no one
// watching — and cancelled (rather than a JSON-RPC error) lets the downstream turn
// end cleanly instead of faulting.
func (h *viewerHub) requestPermission(ctx context.Context, req libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error) {
	h.mu.Lock()
	var controller Viewer
	if s := h.sessions[req.SessionID]; s != nil && s.controllerID != "" {
		controller = s.viewers[s.controllerID]
	}
	h.mu.Unlock()

	if controller == nil {
		return libacp.RequestPermissionResponse{
			Outcome: libacp.RequestPermissionOutcome{Outcome: libacp.PermissionOutcomeCancelled},
		}, nil
	}
	return controller.RequestPermission(ctx, req)
}

// terminalServer returns sessionID's controller viewer cast to TerminalServer, or nil when
// there is no controller or the controller does not implement it. It reads the controller
// under mu then RELEASES the lock before the caller invokes it — a terminal callback
// (WaitForTerminalExit especially) may block, and holding mu there would stall the session's
// fan-out. Mirrors requestPermission's controller lookup, for the terminal/* callback family.
func (h *viewerHub) terminalServer(sessionID libacp.SessionID) TerminalServer {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[sessionID]
	if s == nil || s.controllerID == "" {
		return nil
	}
	if ts, ok := s.viewers[s.controllerID].(TerminalServer); ok {
		return ts
	}
	return nil
}

// closeSession removes sessionID's ENTIRE state — its journal, its viewer registry, and its
// controller — as one wholesale teardown (the kernel's CloseSession), unlike detach which
// removes one viewer at a time. onDetach fires for each viewer that was still attached,
// OUTSIDE the lock, so an event sink sees a departure for every viewer the closed session
// held. A no-op for an unknown session.
func (h *viewerHub) closeSession(sessionID libacp.SessionID) {
	h.mu.Lock()
	s := h.sessions[sessionID]
	if s == nil {
		h.mu.Unlock()
		return
	}
	ids := append([]string(nil), s.order...)
	delete(h.sessions, sessionID)
	h.mu.Unlock()

	if h.onDetach != nil {
		for _, id := range ids {
			h.onDetach(sessionID, id)
		}
	}
}

// counts reports the number of sessions with at least one attached viewer and the
// total attached viewers across them — the viewer/session counts List surfaces.
func (h *viewerHub) counts() (sessions, viewers int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.sessions {
		sessions++
		viewers += len(s.viewers)
	}
	return sessions, viewers
}
