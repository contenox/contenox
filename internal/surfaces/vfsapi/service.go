// Package vfsapi serves the VFS over HTTP: files, folders and the access list
// that decides who may read and write them.
//
// Routes are registered without a prefix and the host mounts them where it
// wants them, which is what lets one surface serve an operator's admin API and
// a user-facing one without a second implementation:
//
//	sub := http.NewServeMux()
//	api.AddRoutes(sub)
//	mux.Handle("/v1/admin/vfs/", http.StripPrefix("/v1/admin/vfs", sub))
//
// Every request is resolved to a [vfs.Actor] by the host, and that actor is
// stamped onto the request context so the access list's callbacks see it: the
// surface does not invent an identity, and it never consults the access list
// itself for file reads — the VFS does, per resource.
package vfsapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/internal/services/vfs"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
)

// ErrNoActor is what an [Authenticator] returns when a request carries no
// identity. It is the only error answered 401; anything else is the
// authenticator's own failure and is answered 500.
var ErrNoActor = errors.New("vfsapi: request carries no actor")

// Authenticator resolves a request to the identity it acts as. The host owns
// this: a relay answers with the signed-in user and their platform role, a
// local install with the machine's own identity.
type Authenticator interface {
	Actor(r *http.Request) (vfs.Actor, error)
}

// Config is the surface's whole configuration.
type Config struct {
	// VFS is the store every route reads and writes. Required.
	VFS vfs.Service
	// ACL holds the grants and is what the access-control routes operate on.
	// Optional: without it, per-resource checks do not run and the
	// access-control routes are not mounted — a single-user install has no
	// grants to manage.
	ACL *vfs.ACL
	// Actor resolves the request's identity. Required.
	Actor Authenticator
	// Tracker instruments failures. Nil degrades to a no-op tracker.
	Tracker libtracker.ActivityTracker
}

// Service is the HTTP surface over one VFS.
type Service struct {
	vfs     vfs.Service
	acl     *vfs.ACL
	actor   Authenticator
	tracker libtracker.ActivityTracker
}

// New builds the surface.
func New(cfg Config) (*Service, error) {
	if cfg.VFS == nil {
		return nil, errors.New("vfsapi: a VFS is required")
	}
	if cfg.Actor == nil {
		return nil, errors.New("vfsapi: an actor resolver is required")
	}
	tracker := cfg.Tracker
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &Service{vfs: cfg.VFS, acl: cfg.ACL, actor: cfg.Actor, tracker: tracker}, nil
}

// AddRoutes mounts the surface on mux, without a prefix.
func (s *Service) AddRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /files", s.handleCreateFile)
	mux.HandleFunc("GET /files", s.handleListFiles)
	mux.HandleFunc("GET /files/{id}", s.handleGetFile)
	mux.HandleFunc("PUT /files/{id}", s.handleUpdateFile)
	mux.HandleFunc("DELETE /files/{id}", s.handleDeleteFile)
	mux.HandleFunc("GET /files/{id}/download", s.handleDownloadFile)
	mux.HandleFunc("PUT /files/{id}/name", s.handleRenameFile)
	mux.HandleFunc("PUT /files/{id}/move", s.handleMoveFile)

	mux.HandleFunc("POST /folders", s.handleCreateFolder)
	mux.HandleFunc("PUT /folders/{id}/name", s.handleRenameFolder)
	mux.HandleFunc("PUT /folders/{id}/move", s.handleMoveFolder)
	mux.HandleFunc("DELETE /folders/{id}", s.handleDeleteFolder)

	if s.acl != nil {
		mux.HandleFunc("GET /permissions", s.handleListPermissions)
		mux.HandleFunc("POST /access-control", s.handleCreateAccessEntry)
		mux.HandleFunc("GET /access-control", s.handleListAccessEntries)
		mux.HandleFunc("GET /access-control/{id}", s.handleGetAccessEntry)
		mux.HandleFunc("PUT /access-control/{id}", s.handleUpdateAccessEntry)
		mux.HandleFunc("DELETE /access-control/{id}", s.handleDeleteAccessEntry)
	}
}

// request resolves the caller and the tenant it is acting in, answering the
// error itself when either is missing. The returned context carries the actor
// for the VFS callbacks, which is the only thing that makes the access list
// enforce anything.
func (s *Service) request(w http.ResponseWriter, r *http.Request, op apiframework.Operation) (context.Context, vfs.Actor, string, bool) {
	actor, err := s.actor.Actor(r)
	if err != nil {
		if errors.Is(err, ErrNoActor) {
			_ = apiframework.Error(w, r, apiframework.Unauthorized("unauthenticated"), op)
			return nil, vfs.Actor{}, "", false
		}
		s.fail(w, r, "actor", err, op)
		return nil, vfs.Actor{}, "", false
	}

	tenantID := strings.TrimSpace(actor.TenantID)
	// An operator acts across tenants and has none of its own, so it names the
	// one it is working in. Anyone else acts in their own tenant, and naming
	// another is not a thing they can do.
	if actor.Admin {
		if named := strings.TrimSpace(apiframework.GetQueryParam(r, "tenant_id", "",
			"Tenant to act in. Honoured for an operator, ignored otherwise.")); named != "" {
			tenantID = named
		}
	}
	if tenantID == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("no tenant in scope: an operator must name one with tenant_id"), op)
		return nil, vfs.Actor{}, "", false
	}
	actor.TenantID = tenantID
	return vfs.WithActor(r.Context(), actor), actor, tenantID, true
}

// requireManage refuses a mutation the actor may not perform. A delete is the
// case the VFS boundary cannot see: its callbacks gate a delete as a write, so
// edit reaches them, and manage is the authority that means "may destroy this".
func (s *Service) requireManage(ctx context.Context, w http.ResponseWriter, r *http.Request, actor vfs.Actor, tenantID, resourceType, resource string, op apiframework.Operation) bool {
	if s.acl == nil {
		return true
	}
	if err := s.acl.Allow(ctx, actor, tenantID, resourceType, resource, vfs.PermissionManage); err != nil {
		if errors.Is(err, vfs.ErrAccessDenied) {
			_ = apiframework.Error(w, r, apiframework.Forbidden("manage permission required on this resource"), op)
			return false
		}
		s.fail(w, r, "authorize", err, op)
		return false
	}
	return true
}

func (s *Service) fail(w http.ResponseWriter, r *http.Request, operation string, err error, op apiframework.Operation) {
	reportErr, _, end := s.tracker.Start(r.Context(), operation, "vfsapi")
	defer end()
	reportErr(err)
	_ = apiframework.Error(w, r, apiframework.InternalServerError("vfs: request failed"), op)
}

// apiErr maps the VFS's own refusals onto HTTP, so every route answers through
// apiframework with one status per cause. Sentinels apiframework already
// understands pass through untouched.
func apiErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, vfs.ErrUnknownPath):
		return apiframework.NotFound("not found")
	case errors.Is(err, vfs.ErrFolderNotEmpty):
		return apiframework.Conflict(err.Error())
	case errors.Is(err, vfs.ErrInvalidMove):
		return apiframework.BadRequest(err.Error())
	case errors.Is(err, libdb.ErrNotFound):
		// The access list refuses as "not found" on purpose: a caller who may not
		// see a resource should not learn from the status that it exists.
		return apiframework.NotFound("not found")
	case errors.Is(err, vfs.ErrAccessDenied):
		return apiframework.Forbidden("access denied")
	case errors.Is(err, libdb.ErrUniqueViolation):
		return apiframework.Conflict("already exists")
	default:
		return err
	}
}

// encodeCursor serialises a keyset position into the opaque token returned in
// X-Next-Cursor. The id rides along because two rows can share a timestamp, and
// a timestamp-only cursor then drops or repeats rows.
func encodeCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.Format(time.RFC3339Nano) + "\x00" + id))
}

func decodeCursor(tok string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return time.Time{}, "", err
	}
	ts, id, ok := strings.Cut(string(raw), "\x00")
	if !ok {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, id, nil
}

// pageLimit reads ?limit=, clamped to something a store can answer.
func pageLimit(raw string) int {
	if raw == "" {
		return 100
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 1000 {
		return 100
	}
	return n
}
