package vfsapi

import (
	"net/http"

	"github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/internal/services/vfs"
)

func (s *Service) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.CreateOperation)
	if !ok {
		return
	}
	body, err := apiframework.Decode[folderCreateBody](r)
	if err != nil || body.Name == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("name is required"), apiframework.CreateOperation)
		return
	}
	folder, err := s.vfs.CreateFolder(ctx, tenantID, body.ParentID, body.Name)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, folderToResp(folder))
}

func (s *Service) handleRenameFolder(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing folder id"), apiframework.UpdateOperation)
		return
	}
	body, err := apiframework.Decode[nameBody](r)
	if err != nil || body.Name == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("name is required"), apiframework.UpdateOperation)
		return
	}
	folder, err := s.vfs.RenameFolder(ctx, tenantID, id, body.Name)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, folderToResp(folder))
}

func (s *Service) handleMoveFolder(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing folder id"), apiframework.UpdateOperation)
		return
	}
	body, err := apiframework.Decode[moveBody](r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("invalid json"), apiframework.UpdateOperation)
		return
	}
	folder, err := s.vfs.MoveFolder(ctx, tenantID, id, body.NewParentID)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, folderToResp(folder))
}

func (s *Service) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.DeleteOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing folder id"), apiframework.DeleteOperation)
		return
	}
	if !s.requireManage(ctx, w, r, actor, tenantID, vfs.ResourceTypeFolders, id, apiframework.DeleteOperation) {
		return
	}
	if err := s.vfs.DeleteFolder(ctx, tenantID, id); err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, messageResp{Message: "folder removed"})
}
