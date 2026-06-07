package taskchainapi

import (
	"fmt"
	"net/http"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

func AddTaskChainRoutes(mux *http.ServeMux, service taskchainservice.Service) {
	h := &handler{service: service}
	mux.HandleFunc("GET /taskchains/list", h.listTaskChains)
	mux.HandleFunc("GET /taskchains", h.getTaskChain)
	mux.HandleFunc("POST /taskchains", h.createTaskChain)
	mux.HandleFunc("PUT /taskchains", h.updateTaskChain)
	mux.HandleFunc("DELETE /taskchains", h.deleteTaskChain)
}

type handler struct {
	service taskchainservice.Service
}

func normalizeChainPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: query parameter path is required", apiframework.ErrBadRequest)
	}
	return taskchainservice.NormalizePath(raw)
}

func (h *handler) listTaskChains(w http.ResponseWriter, r *http.Request) {
	paths, err := h.service.List(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	if paths == nil {
		paths = []string{}
	}
	_ = apiframework.Encode(w, r, http.StatusOK, paths) // @response []string
}

func (h *handler) getTaskChain(w http.ResponseWriter, r *http.Request) {
	rawPath := apiframework.GetQueryParam(r, "path", "", "Relative chain JSON path inside .contenox.")
	path, err := normalizeChainPath(rawPath)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	chain, err := h.service.Get(r.Context(), path)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, chain) // @response taskengine.TaskChainDefinition
}

func (h *handler) createTaskChain(w http.ResponseWriter, r *http.Request) {
	rawPath := apiframework.GetQueryParam(r, "path", "", "Relative chain JSON path inside .contenox.")
	path, err := normalizeChainPath(rawPath)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	chain, err := apiframework.Decode[taskengine.TaskChainDefinition](r) // @request taskengine.TaskChainDefinition
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if err := h.service.CreateAtPath(r.Context(), path, &chain); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, chain) // @response taskengine.TaskChainDefinition
}

func (h *handler) updateTaskChain(w http.ResponseWriter, r *http.Request) {
	rawPath := apiframework.GetQueryParam(r, "path", "", "Relative chain JSON path inside .contenox.")
	path, err := normalizeChainPath(rawPath)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	chain, err := apiframework.Decode[taskengine.TaskChainDefinition](r) // @request taskengine.TaskChainDefinition
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	if err := h.service.UpdateAtPath(r.Context(), path, &chain); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, chain) // @response taskengine.TaskChainDefinition
}

func (h *handler) deleteTaskChain(w http.ResponseWriter, r *http.Request) {
	rawPath := apiframework.GetQueryParam(r, "path", "", "Relative chain JSON path inside .contenox.")
	path, err := normalizeChainPath(rawPath)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	if err := h.service.DeleteByPath(r.Context(), path); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, fmt.Sprintf("task chain file %s deleted", path)) // @response string
}
