package localfileapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
)

// workspaceRoot is one entry of the /workspace/roots response: an allowlisted
// directory a client may choose as a workspace, with `default` marking the one
// that "/" and the empty root resolve to (see vfs.Factory.Default).
type workspaceRoot struct {
	Path    string `json:"path"`
	Default bool   `json:"default"`
}

// workspaceRootsResponse is the /workspace/roots response body — the shape GET,
// POST, and DELETE all return, so Beam consumes the current allowlist the same
// way whether it just read it or just changed it.
type workspaceRootsResponse struct {
	Roots []workspaceRoot `json:"roots"`
}

// addWorkspaceRootRequest is the POST /workspace/roots body: the directory to
// grant as a workspace root.
type addWorkspaceRootRequest struct {
	Path string `json:"path"`
}

// RootsMutators carries the write half of the workspace-roots surface: the
// grant/revoke operations POST and DELETE invoke. It is OPTIONAL — passed nil,
// AddWorkspaceRootsRoutes registers only the read-only GET (the legacy shape,
// and what the unit tests that predate grants still use).
//
// Each closure is expected to (1) persist the durable grant, (2) apply it to the
// live vfs.Factory so the very next GET reflects it without waiting for the bus,
// and (3) ring the cross-process reload doorbell — all of which live in serve's
// wiring (runtime/contenoxcli), not here, so this internal API stays free of the
// config store and the bus. A closure returning an error wrapping
// workspacegrants.ErrInvalidGrant is a client input fault (422); any other error
// is a storage/apply fault (500).
type RootsMutators struct {
	// Add persists and applies a grant for path. Returning
	// workspacegrants.ErrInvalidGrant means the path was bad input.
	Add func(ctx context.Context, path string) error
	// Remove revokes the grant for path (idempotent — revoking a path that was
	// never granted is not an error).
	Remove func(ctx context.Context, path string) error
}

// AddWorkspaceRootsRoutes registers GET /workspace/roots — which reports the
// serve-configured workspace-root allowlist — and, when mutators is non-nil,
// POST /workspace/roots and DELETE /workspace/roots, the authenticated grant
// verbs an operator uses to add or remove a root at runtime.
//
// The read route exists so a client — chiefly Beam, which may be LAN-served with
// no other hands on the host — can learn the allowlist and offer a folder picker
// (per-session cwd, dispatch cwd, workspace switcher) directly, instead of
// discovering the boundary by probing paths and reading the 422 that
// AddWorkspaceRoutes's per-request `root` check returns.
//
// # The write routes are new, and deliberately so (maintainer order, 2026-07-21)
//
// This route group once documented itself as read-only BY DESIGN, on the reasoning
// that growing the allowlist is a trust grant an operator makes through serve's
// launch configuration, not something a REST client should do. The maintainer
// reversed that after hitting the gap live: a LAN operator working only through
// the browser has no shell on the host to edit serve's launch config and no way
// to restart serve without dropping the session under review. Granting a
// workspace root is now an EXPLICIT, AUTHENTICATED verb — POST/DELETE are
// token-authed exactly like every other mutating route mounted here — carrying
// the same validation, the same durable config write, and the same reload
// doorbell as the `contenox workspace` CLI. The grant is auditable through the
// durable config it writes (and serve's log line on apply); it is not a silent
// widening. The `contenox workspace` CLI remains the shell-side equivalent for
// an operator who does have host access.
//
// Nil-gated like the other optional route groups: when serve has no
// workspace-root allowlist configured (factory nil), AddWorkspaceRootsRoutes
// registers nothing and the routes 404, matching AddWorkspaceRoutes's own
// requirement that factory be non-nil.
func AddWorkspaceRootsRoutes(mux *http.ServeMux, factory *vfs.Factory, mutators *RootsMutators) {
	if factory == nil {
		return
	}
	mux.HandleFunc("GET /workspace/roots", func(w http.ResponseWriter, r *http.Request) {
		writeRootsResponse(w, r, factory)
	})
	if mutators == nil {
		return
	}
	if mutators.Add != nil {
		mux.HandleFunc("POST /workspace/roots", func(w http.ResponseWriter, r *http.Request) {
			req, err := apiframework.Decode[addWorkspaceRootRequest](r)
			if err != nil {
				_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
				return
			}
			if strings.TrimSpace(req.Path) == "" {
				_ = apiframework.Error(w, r,
					apiframework.MissingParameter("path", "path is required: name the directory to grant as a workspace root"),
					apiframework.CreateOperation)
				return
			}
			if err := mutators.Add(r.Context(), req.Path); err != nil {
				_ = apiframework.Error(w, r, grantError(err), apiframework.CreateOperation)
				return
			}
			writeRootsResponse(w, r, factory)
		})
	}
	if mutators.Remove != nil {
		mux.HandleFunc("DELETE /workspace/roots", func(w http.ResponseWriter, r *http.Request) {
			path := apiframework.GetQueryParam(r, "path", "", "Workspace root directory to revoke.")
			if strings.TrimSpace(path) == "" {
				_ = apiframework.Error(w, r,
					apiframework.MissingParameter("path", "path is required: name the workspace root to revoke"),
					apiframework.DeleteOperation)
				return
			}
			if err := mutators.Remove(r.Context(), path); err != nil {
				_ = apiframework.Error(w, r, grantError(err), apiframework.DeleteOperation)
				return
			}
			writeRootsResponse(w, r, factory)
		})
	}
}

// writeRootsResponse encodes the live allowlist as the shared response shape.
// GET, POST, and DELETE all end here, so a mutating call returns the SAME body a
// fresh GET would — the client re-renders the picker from the response it already
// has, with no follow-up read.
func writeRootsResponse(w http.ResponseWriter, r *http.Request, factory *vfs.Factory) {
	def := factory.Default()
	roots := factory.Roots()
	resp := workspaceRootsResponse{Roots: make([]workspaceRoot, len(roots))}
	for i, root := range roots {
		resp.Roots[i] = workspaceRoot{Path: root, Default: root == def}
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response localfileapi.workspaceRootsResponse
}

// grantError maps a mutator failure onto the right API error: a bad grant path
// (wrapping workspacegrants.ErrInvalidGrant) is the client's fault and becomes a
// 422 carrying the teaching text — the same unprocessable-entity shape the browse
// API returns for a non-permitted root, so a client parses both uniformly.
// Anything else is a storage/apply failure and stays a 500.
func grantError(err error) error {
	if errors.Is(err, workspacegrants.ErrInvalidGrant) {
		return fmt.Errorf("%w: %s", apiframework.ErrUnprocessableEntity, err.Error())
	}
	return err
}
