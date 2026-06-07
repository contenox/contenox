package localfileapi

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/localfileservice"
)

func AddRoutes(mux *http.ServeMux, service localfileservice.Service) {
	h := &handler{service: service}
	mux.HandleFunc("GET /files", h.list)
	mux.HandleFunc("GET /files/stat", h.stat)
	mux.HandleFunc("GET /files/content", h.content)
	mux.HandleFunc("GET /files/download", h.download)
	mux.HandleFunc("POST /files", h.createFile)
	mux.HandleFunc("PUT /files", h.updateFile)
	mux.HandleFunc("DELETE /files", h.deletePath)
	mux.HandleFunc("PUT /files/move", h.movePath)
	mux.HandleFunc("POST /folders", h.createFolder)
	mux.HandleFunc("DELETE /folders", h.deletePath)
}

type handler struct {
	service localfileservice.Service
}

type writeFileRequest struct {
	Path          string `json:"path"`
	Content       string `json:"content"`
	ContentBase64 string `json:"contentBase64"`
}

type createFolderRequest struct {
	Path string `json:"path"`
}

type moveRequest struct {
	Path    string `json:"path"`
	NewPath string `json:"newPath"`
}

type fileContentResponse struct {
	Path          string                 `json:"path"`
	Content       string                 `json:"content"`
	ContentBase64 string                 `json:"contentBase64,omitempty"`
	Encoding      string                 `json:"encoding"`
	Metadata      localfileservice.Entry `json:"metadata"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", ".", "Directory path relative to the project root.")
	entries, err := h.service.List(r.Context(), path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entries) // @response []localfileservice.Entry
}

func (h *handler) stat(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", "", "Path relative to the project root.")
	entry, err := h.service.Stat(r.Context(), path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entry) // @response localfileservice.Entry
}

func (h *handler) content(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", "", "File path relative to the project root.")
	data, meta, err := h.service.Read(r.Context(), path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	resp := fileContentResponse{
		Path:     meta.Path,
		Metadata: *meta,
	}
	if looksText(data) {
		resp.Content = string(data)
		resp.Encoding = "utf-8"
	} else {
		resp.ContentBase64 = base64.StdEncoding.EncodeToString(data)
		resp.Encoding = "base64"
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response localfileapi.fileContentResponse
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", "", "File path relative to the project root.")
	data, meta, err := h.service.Read(r.Context(), path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (h *handler) createFile(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[writeFileRequest](r) // @request localfileapi.writeFileRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	data, err := decodeFileContent(req)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	entry, err := h.service.Write(r.Context(), req.Path, data, true)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, entry) // @response localfileservice.Entry
}

func (h *handler) updateFile(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[writeFileRequest](r) // @request localfileapi.writeFileRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	data, err := decodeFileContent(req)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	entry, err := h.service.Write(r.Context(), req.Path, data, false)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entry) // @response localfileservice.Entry
}

func (h *handler) createFolder(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[createFolderRequest](r) // @request localfileapi.createFolderRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	entry, err := h.service.Mkdir(r.Context(), req.Path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, entry) // @response localfileservice.Entry
}

func (h *handler) movePath(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[moveRequest](r) // @request localfileapi.moveRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	entry, err := h.service.Move(r.Context(), req.Path, req.NewPath)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entry) // @response localfileservice.Entry
}

func (h *handler) deletePath(w http.ResponseWriter, r *http.Request) {
	path := apiframework.GetQueryParam(r, "path", "", "Path relative to the project root.")
	if err := h.service.Delete(r.Context(), path); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, apiframework.MessageResponse{Message: "path removed"}) // @response apiframework.MessageResponse
}

func decodeFileContent(req writeFileRequest) ([]byte, error) {
	if req.ContentBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid contentBase64", apiframework.ErrUnprocessableEntity)
		}
		return data, nil
	}
	return []byte(req.Content), nil
}

func looksText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}
