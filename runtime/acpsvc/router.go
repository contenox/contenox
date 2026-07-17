package acpsvc

import (
	"context"
	"errors"
	"sync"

	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// ErrNoBoundSession reports that no live ACP transport owns the contenox
// session named in the request context. serve's shared AskApproval keys its
// fallback on this: when the router cannot route (a headless/API caller, or a
// session with no live WS connection), the engine's HITL request goes to the
// approval-API path instead of hanging on a permission prompt nobody answers.
var ErrNoBoundSession = errors.New("acpsvc: no ACP transport bound to contenox session")

// PermissionRouter maps a contenox session id to the ACP Transport that owns
// it. serve runs many ACP WebSocket connections (each its own Transport)
// behind a SINGLE shared engine, so the engine's one AskApproval callback
// cannot close over a single transport the way the stdio ACP path does
// (acp_cmd.go late-binds its lone transport directly). Instead each Transport
// registers its live (contenox session -> this transport) bindings here, and
// serve's AskApproval consults the router to dispatch session/request_permission
// to the exact WS connection whose client raised the gated tool call — the one
// beam's PermissionGate is waiting on.
//
// The stdio ACP path leaves Deps.PermissionRouter nil: it has exactly one
// transport and needs no routing.
type PermissionRouter struct {
	mu       sync.RWMutex
	bindings map[string]*Transport
}

// NewPermissionRouter returns an empty router ready to be shared across a
// serve process's ACP WebSocket transports.
func NewPermissionRouter() *PermissionRouter {
	return &PermissionRouter{bindings: make(map[string]*Transport)}
}

// bind records that t owns the given contenox session. A nil receiver (the
// stdio path, which passes no router) and empty inputs are no-ops so callers
// need no guard. Last writer wins: if the same session is loaded on a second
// connection, the newer transport becomes the routing target, matching the
// per-transport contenoxToACPID map's own last-writer-wins semantics.
func (r *PermissionRouter) bind(contenoxSessionID string, t *Transport) {
	if r == nil || contenoxSessionID == "" || t == nil {
		return
	}
	r.mu.Lock()
	r.bindings[contenoxSessionID] = t
	r.mu.Unlock()
}

// unbind drops the binding for contenoxSessionID, but only when it still points
// at t. Guarding on identity means a transport tearing down a session that a
// newer connection has since re-bound does not evict the live binding.
func (r *PermissionRouter) unbind(contenoxSessionID string, t *Transport) {
	if r == nil || contenoxSessionID == "" {
		return
	}
	r.mu.Lock()
	if r.bindings[contenoxSessionID] == t {
		delete(r.bindings, contenoxSessionID)
	}
	r.mu.Unlock()
}

func (r *PermissionRouter) transportFor(contenoxSessionID string) (*Transport, bool) {
	if r == nil || contenoxSessionID == "" {
		return nil, false
	}
	r.mu.RLock()
	t, ok := r.bindings[contenoxSessionID]
	r.mu.RUnlock()
	return t, ok
}

// AskApproval bridges an engine HITL request to the ACP transport that owns the
// contenox session named in ctx (runtimetypes.SessionIDContextKey), driving that
// connection's session/request_permission flow. It returns ErrNoBoundSession
// when no live transport owns the session so the caller can fall back to a
// non-ACP approval path; a genuine deny resolves as (false, nil) and a client
// cancellation as (false, context.Canceled) — neither is ErrNoBoundSession, so
// neither triggers a fallback.
func (r *PermissionRouter) AskApproval(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
	contenoxSessionID, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	t, ok := r.transportFor(contenoxSessionID)
	if !ok {
		return false, ErrNoBoundSession
	}
	return t.AskApproval(ctx, req)
}
