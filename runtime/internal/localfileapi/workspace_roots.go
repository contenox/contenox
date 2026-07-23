package localfileapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/project"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
)

// workspaceRoot is one entry of the /workspace/roots response: an allowlisted
// project directory a client may choose as a workspace. `default` marks the one
// that "/" and the empty root resolve to (see vfs.Factory.Default); `name` is the
// project's EXPLICIT marker name — empty for an unmarked or unnamed root, so a
// client can tell a real named project from a structural root (it applies its own
// basename fallback for display); and `managed` marks a root the operator granted
// at runtime — the forgettable ones, as opposed to serve's launch roots.
type workspaceRoot struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Managed bool   `json:"managed"`
}

// workspaceRootsResponse is the /workspace/roots response body — the shape GET,
// POST, and DELETE all return, so Beam consumes the current allowlist the same
// way whether it just read it or just changed it.
type workspaceRootsResponse struct {
	Roots []workspaceRoot `json:"roots"`
}

// addWorkspaceRootRequest is the POST /workspace/roots body: the directory to
// register as a project workspace root, and an optional friendly name stamped
// into its marker (re-adding an already-registered path under a new name
// renames the project).
type addWorkspaceRootRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
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
	// Add registers path as a project workspace root under an optional friendly
	// name (stamped into the project's marker; an explicit name renames an
	// already-named project), then persists and applies the grant. Returning
	// workspacegrants.ErrInvalidGrant means the path or name was bad input.
	Add func(ctx context.Context, path, name string) error
	// Remove revokes the grant for path (idempotent — revoking a path that was
	// never granted is not an error).
	Remove func(ctx context.Context, path string) error
	// Grants returns the paths the operator granted at runtime, so the response can
	// mark them `managed` (forgettable). Optional: nil leaves every root unmanaged.
	Grants func(ctx context.Context) []string
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
		writeRootsResponse(w, r, factory, mutators) // @response localfileapi.workspaceRootsResponse
	})
	if mutators == nil {
		return
	}
	if mutators.Add != nil {
		mux.HandleFunc("POST /workspace/roots", func(w http.ResponseWriter, r *http.Request) {
			req, err := apiframework.Decode[addWorkspaceRootRequest](r) // @request localfileapi.addWorkspaceRootRequest
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
			if err := mutators.Add(r.Context(), req.Path, req.Name); err != nil {
				_ = apiframework.Error(w, r, grantError(err), apiframework.CreateOperation)
				return
			}
			writeRootsResponse(w, r, factory, mutators) // @response localfileapi.workspaceRootsResponse
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
			writeRootsResponse(w, r, factory, mutators) // @response localfileapi.workspaceRootsResponse
		})
	}
}

// writeRootsResponse encodes the live allowlist as the shared response shape.
// GET, POST, and DELETE all end here, so a mutating call returns the SAME body a
// fresh GET would — the client re-renders the picker from the response it already
// has, with no follow-up read.
func writeRootsResponse(w http.ResponseWriter, r *http.Request, factory *vfs.Factory, mutators *RootsMutators) {
	def := factory.Default()
	roots := factory.Roots()
	managed := managedRootSet(r.Context(), mutators)
	resp := workspaceRootsResponse{Roots: make([]workspaceRoot, len(roots))}
	for i, root := range roots {
		resp.Roots[i] = workspaceRoot{
			Path:    root,
			Name:    project.MarkerName(root),
			Default: root == def,
			Managed: managed[root],
		}
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response localfileapi.workspaceRootsResponse
}

// managedRootSet resolves the operator's runtime grants into the same
// symlink-resolved space factory.Roots() reports, so a grant marks its matching
// root `managed` even when the configured path was a symlink. Empty when no
// grants resolver was supplied (read-only mount) — every root is then unmanaged.
func managedRootSet(ctx context.Context, mutators *RootsMutators) map[string]bool {
	set := map[string]bool{}
	if mutators == nil || mutators.Grants == nil {
		return set
	}
	for _, g := range mutators.Grants(ctx) {
		if resolved, err := vfs.ResolveRoot(g); err == nil {
			set[resolved] = true
		}
	}
	return set
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
