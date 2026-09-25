package vfsapi

import (
	"bytes"
	"net/http"
	"net/url"
	"strconv"

	"github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/internal/services/vfs"
)

func (s *Service) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.CreateOperation)
	if !ok {
		return
	}
	up, err := readUpload(w, r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest(err.Error()), apiframework.CreateOperation)
		return
	}

	file, err := s.vfs.CreateFile(ctx, tenantID, &vfs.File{
		Name:        up.name,
		ParentID:    up.parentID,
		ContentType: up.contentType,
		Data:        up.data,
		Size:        up.size,
		Metadata:    up.metadata,
	})
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, fileToResp(file))
}

func (s *Service) handleGetFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.GetOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.GetOperation)
		return
	}
	file, err := s.vfs.GetFileByID(ctx, tenantID, id)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, fileToResp(file))
}

func (s *Service) handleListFiles(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.ListOperation)
	if !ok {
		return
	}
	// Unescaped because a slash is a separator inside the path, not in the
	// encoding of it, so a client may send either form.
	rawPath := apiframework.GetQueryParam(r, "path", "", "Folder path to list; empty lists the tenant root.")
	decoded, err := url.QueryUnescape(rawPath)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("invalid path"), apiframework.ListOperation)
		return
	}

	var after *vfs.Cursor
	if tok := apiframework.GetQueryParam(r, "cursor", "", "Opaque next-page cursor from a prior response's X-Next-Cursor header."); tok != "" {
		createdAt, id, cerr := decodeCursor(tok)
		if cerr != nil {
			_ = apiframework.Error(w, r, apiframework.BadRequest("invalid cursor"), apiframework.ListOperation)
			return
		}
		after = &vfs.Cursor{CreatedAt: createdAt, ID: id}
	}

	list, next, err := s.vfs.GetFilesByPath(ctx, tenantID, decoded, vfs.Page{
		Desc:  true,
		Limit: pageLimit(apiframework.GetQueryParam(r, "limit", "", "Maximum items per page; defaults to 100.")),
		After: after,
	})
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.ListOperation)
		return
	}
	// The store returns a cursor only when the page was full, so a client pages
	// by following the header rather than rebuilding a lossy cursor itself.
	if next != nil {
		w.Header().Set("X-Next-Cursor", encodeCursor(next.CreatedAt, next.ID))
	}
	out := make([]fileResp, 0, len(list))
	for i := range list {
		out = append(out, fileToResp(&list[i]))
	}
	_ = apiframework.Encode(w, r, http.StatusOK, out)
}

func (s *Service) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.UpdateOperation)
		return
	}
	up, err := readUpload(w, r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest(err.Error()), apiframework.UpdateOperation)
		return
	}

	// An update replaces metadata wholesale, so a request that carries no
	// metadata field preserves what is stored: a client sending only new content
	// would otherwise wipe a title or an importer's source token.
	metadata := up.metadata
	if !up.metadataSet {
		existing, gerr := s.vfs.GetFileByID(ctx, tenantID, id)
		if gerr != nil {
			_ = apiframework.Error(w, r, apiErr(gerr), apiframework.UpdateOperation)
			return
		}
		metadata = existing.Metadata
	}

	file, err := s.vfs.UpdateFile(ctx, tenantID, &vfs.File{
		ID:          id,
		ParentID:    up.parentID,
		ContentType: up.contentType,
		Data:        up.data,
		Size:        up.size,
		Metadata:    metadata,
	})
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, fileToResp(file))
}

func (s *Service) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	ctx, actor, tenantID, ok := s.request(w, r, apiframework.DeleteOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.DeleteOperation)
		return
	}
	if !s.requireManage(ctx, w, r, actor, tenantID, vfs.ResourceTypeFiles, id, apiframework.DeleteOperation) {
		return
	}
	if err := s.vfs.DeleteFile(ctx, tenantID, id); err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, messageResp{Message: "file removed"})
}

func (s *Service) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.GetOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.GetOperation)
		return
	}
	file, err := s.vfs.GetFileByID(ctx, tenantID, id)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.GetOperation)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	// The stored bytes are served as an attachment, never as the type an
	// uploader claimed, and never sniffed into something executable.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	if r.URL.Query().Get("skip") != "true" {
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(file.Name))
	}
	if _, err := bytes.NewReader(file.Data).WriteTo(w); err != nil {
		// The status and headers are already on the wire, so the client sees a
		// truncated body and there is nothing left to answer with. The failure is
		// reported instead of dropped.
		reportErr, _, end := s.tracker.Start(ctx, "download", "vfsapi")
		defer end()
		reportErr(err)
	}
}

func (s *Service) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.UpdateOperation)
		return
	}
	body, err := apiframework.Decode[nameBody](r)
	if err != nil || body.Name == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("name is required"), apiframework.UpdateOperation)
		return
	}
	file, err := s.vfs.RenameFile(ctx, tenantID, id, body.Name)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, fileToResp(file))
}

func (s *Service) handleMoveFile(w http.ResponseWriter, r *http.Request) {
	ctx, _, tenantID, ok := s.request(w, r, apiframework.UpdateOperation)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadPathValue("id", "missing file id"), apiframework.UpdateOperation)
		return
	}
	body, err := apiframework.Decode[moveBody](r)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("invalid json"), apiframework.UpdateOperation)
		return
	}
	file, err := s.vfs.MoveFile(ctx, tenantID, id, body.NewParentID)
	if err != nil {
		_ = apiframework.Error(w, r, apiErr(err), apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, fileToResp(file))
}

func fileToResp(f *vfs.File) fileResp {
	return fileResp{
		ID:          f.ID,
		Path:        f.Path,
		Name:        f.Name,
		ContentType: f.ContentType,
		Size:        f.Size,
		ParentID:    f.ParentID,
		IsDirectory: f.IsDirectory,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
		Metadata:    f.Metadata,
	}
}

func folderToResp(f *vfs.Folder) folderResp {
	return folderResp{
		ID:        f.ID,
		Path:      f.Path,
		Name:      f.Name,
		ParentID:  f.ParentID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}
