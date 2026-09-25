package vfsapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/internal/services/vfs"
	libdb "github.com/contenox/contenox/libdbexec"
)

// The access-control routes are the sharing surface: a grant names an identity,
// a resource and a permission. Every mutation requires manage on the resource
// the grant points at, which is why an owner can share a file they created and
// a stranger cannot.

func (s *Service) handleCreateAccessEntry(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.CreateOperation)
	if !ok {
		return
	}
	body, err := apiframework.Decode[accessEntryBody](r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("invalid json"), apiframework.CreateOperation)
		return
	}
	if body.Identity == "" || body.Resource == "" || body.ResourceType == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("identity, resource, and resourceType are required"), apiframework.CreateOperation)
		return
	}
	perm, err := vfs.PermissionFromString(body.Permission)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest(err.Error()), apiframework.CreateOperation)
		return
	}
	if !s.requireManage(ctx, w, r, actor, tenantID, body.ResourceType, body.Resource, apiframework.CreateOperation) {
		return
	}

	entry, err := s.acl.Grant(ctx, vfs.AccessEntry{
		TenantID:     tenantID,
		Identity:     body.Identity,
		Resource:     body.Resource,
		ResourceType: body.ResourceType,
		Permission:   perm,
	})
	if err != nil {
		if errors.Is(err, libdb.ErrUniqueViolation) {
			_ = apiframework.Error(w, r, apiframework.Conflict("that grant already exists"), apiframework.CreateOperation)
			return
		}
		_ = apiframework.Error(w, r, apiErr(err), apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, accessEntryToResp(entry))
}

func (s *Service) handleListAccessEntries(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.ListOperation)
	if !ok {
		return
	}

	var after *vfs.AccessCursor
	if tok := apiframework.GetQueryParam(r, "cursor", "", "Opaque next-page cursor from a prior response's X-Next-Cursor header."); tok != "" {
		createdAt, id, cerr := decodeCursor(tok)
		if cerr != nil {
			_ = apiframework.Error(w, r, apiframework.BadRequest("invalid cursor"), apiframework.ListOperation)
			return
		}
		after = &vfs.AccessCursor{CreatedAt: createdAt, ID: id}
	}
	limit := pageLimit(apiframework.GetQueryParam(r, "limit", "", "Maximum items per page; defaults to 100."))

	entries, next, err := s.visibleGrants(ctx, actor, tenantID, after, limit)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.ListOperation)
		return
	}
	if next != nil {
		w.Header().Set("X-Next-Cursor", encodeCursor(next.CreatedAt, next.ID))
	}
	out := make([]accessEntryResp, 0, len(entries))
	for _, e := range entries {
		out = append(out, accessEntryToResp(e))
	}
	_ = apiframework.Encode(w, r, http.StatusOK, accessEntriesListResp{Entries: out})
}

// visibleGrants returns the grants a caller may enumerate: an operator sees the
// tenant's, everyone else sees the ones naming them plus the ones on resources
// they hold manage over — which is what makes "who has access to this file"
// answerable by the person who shared it.
func (s *Service) visibleGrants(ctx context.Context, actor vfs.Actor, tenantID string, after *vfs.AccessCursor, limit int) ([]*vfs.AccessEntry, *vfs.AccessCursor, error) {
	entries, next, err := s.acl.Grants(ctx, tenantID, after, limit)
	if err != nil || actor.Admin {
		return entries, next, err
	}
	visible := make([]*vfs.AccessEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Identity == actor.UserID {
			visible = append(visible, entry)
			continue
		}
		if err := s.acl.Allow(ctx, actor, tenantID, entry.ResourceType, entry.Resource, vfs.PermissionManage); err == nil {
			visible = append(visible, entry)
		}
	}
	return visible, next, nil
}

func (s *Service) handleGetAccessEntry(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.GetOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing grant id"), apiframework.GetOperation)
		return
	}
	entry, err := s.acl.GetGrant(ctx, tenantID, id)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.GetOperation)
		return
	}
	// A grant is readable by whoever it names, by an operator, and by whoever
	// holds manage over what it points at — the authority that could have
	// created it.
	if !actor.Admin && entry.Identity != actor.UserID {
		if err := s.acl.Allow(ctx, actor, tenantID, entry.ResourceType, entry.Resource, vfs.PermissionManage); err != nil {
			_ = apiframework.Error(w, r, apiframework.NotFound("not found"), apiframework.GetOperation)
			return
		}
	}
	_ = apiframework.Encode(w, r, http.StatusOK, accessEntryToResp(entry))
}

func (s *Service) handleUpdateAccessEntry(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing grant id"), apiframework.UpdateOperation)
		return
	}
	body, err := apiframework.Decode[accessEntryBody](r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("invalid json"), apiframework.UpdateOperation)
		return
	}
	if body.Resource == "" || body.ResourceType == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("resource and resourceType are required"), apiframework.UpdateOperation)
		return
	}
	perm, err := vfs.PermissionFromString(body.Permission)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest(err.Error()), apiframework.UpdateOperation)
		return
	}

	// Manage is required on the grant's NEW target, which is the authority the
	// caller is exercising; the old target's authority is not re-read, so an
	// owner who has lost a resource cannot use this route to climb back onto it.
	if !s.requireManage(ctx, w, r, actor, tenantID, body.ResourceType, body.Resource, apiframework.UpdateOperation) {
		return
	}

	updated, err := s.acl.UpdateGrant(ctx, vfs.AccessEntry{
		ID:           id,
		TenantID:     tenantID,
		Resource:     body.Resource,
		ResourceType: body.ResourceType,
		Permission:   perm,
	})
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, accessEntryToResp(updated))
}

func (s *Service) handleDeleteAccessEntry(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.DeleteOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing grant id"), apiframework.DeleteOperation)
		return
	}

	// The grant is resolved first so the check can be made against the resource
	// it points at: revoking is exercising authority over that resource.
	existing, err := s.acl.GetGrant(ctx, tenantID, id)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.DeleteOperation)
		return
	}
	if !s.requireManage(ctx, w, r, actor, tenantID, existing.ResourceType, existing.Resource, apiframework.DeleteOperation) {
		return
	}
	if err := s.acl.Revoke(ctx, tenantID, id); err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.DeleteOperation)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := s.request(w, r, apiframework.GetOperation); !ok {
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, permissionsResp{Permissions: vfs.PermissionStrings()})
}

func accessEntryToResp(e *vfs.AccessEntry) accessEntryResp {
	return accessEntryResp{
		ID:           e.ID,
		Identity:     e.Identity,
		Resource:     e.Resource,
		ResourceType: e.ResourceType,
		Permission:   e.Permission.String(),
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}
