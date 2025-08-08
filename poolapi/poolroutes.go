package poolapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/runtimetypes"
)

func AddPoolRoutes(mux *http.ServeMux, poolService poolservice.Service) {
	s := &poolHandler{service: poolService}

	mux.HandleFunc("POST /pools", s.create)
	mux.HandleFunc("GET /pools", s.listAll)
	mux.HandleFunc("GET /pools/{id}", s.getByID)
	mux.HandleFunc("PUT /pools/{id}", s.update)
	mux.HandleFunc("DELETE /pools/{id}", s.delete)
	mux.HandleFunc("GET /pool-by-name/{name}", s.getByName)
	mux.HandleFunc("GET /pool-by-purpose/{purpose}", s.listByPurpose)

	// Backend associations
	mux.HandleFunc("POST /backend-associations/{poolID}/backends/{backendID}", s.assignBackend)
	mux.HandleFunc("DELETE /backend-associations/{poolID}/backends/{backendID}", s.removeBackend)
	mux.HandleFunc("GET /backend-associations/{poolID}/backends", s.listBackends)
	mux.HandleFunc("GET /backend-associations/{backendID}/pools", s.listPoolsForBackend)

	// Model associations
	mux.HandleFunc("POST /model-associations/{poolID}/models/{modelID}", s.assignModel)
	mux.HandleFunc("DELETE /model-associations/{poolID}/models/{modelID}", s.removeModel)
	mux.HandleFunc("GET /model-associations/{poolID}/models", s.listModels)
	mux.HandleFunc("GET /model-associations/{modelID}/pools", s.listPoolsForModel)
}

type poolHandler struct {
	service poolservice.Service
}

func (h *poolHandler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pool, err := serverops.Decode[runtimetypes.Pool](r) // @request runtimetypes.Pool
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	if err := h.service.Create(ctx, &pool); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, pool) // @response runtimetypes.Pool
}

func (h *poolHandler) getByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("id required: %w", serverops.ErrBadPathValue), serverops.GetOperation)
		return
	}

	pool, err := h.service.GetByID(ctx, id)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pool) // @response runtimetypes.Pool
}

func (h *poolHandler) getByName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := url.PathEscape(r.PathValue("name"))
	if name == "" {
		serverops.Error(w, r, fmt.Errorf("id required: %w", serverops.ErrBadPathValue), serverops.GetOperation)
		return
	}

	pool, err := h.service.GetByName(ctx, name)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pool) // @response runtimetypes.Pool
}

func (h *poolHandler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("id required: %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}

	pool, err := serverops.Decode[runtimetypes.Pool](r) // @request runtimetypes.Pool
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}
	pool.ID = id

	if err := h.service.Update(ctx, &pool); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pool) // @response runtimetypes.Pool
}

func (h *poolHandler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := url.PathEscape(r.PathValue("id"))
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("id required: %w", serverops.ErrBadPathValue), serverops.DeleteOperation)
		return
	}

	if err := h.service.Delete(ctx, id); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "deleted") // @response string
}

func (h *poolHandler) listAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pools, err := h.service.ListAll(ctx)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pools) // @response []runtimetypes.Pool
}

func (h *poolHandler) listByPurpose(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	purpose := url.PathEscape(r.PathValue("purpose"))
	if purpose == "" {
		serverops.Error(w, r, fmt.Errorf("id required: %w", serverops.ErrBadPathValue), serverops.ListOperation)
		return
	}

	// Parse pagination parameters from query string
	var cursor *time.Time
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			err = fmt.Errorf("%w: invalid cursor format, expected RFC3339Nano", serverops.ErrUnprocessableEntity)
			_ = serverops.Error(w, r, err, serverops.ListOperation)
			return
		}
		cursor = &t
	}

	limit := 100 // Default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		i, err := strconv.Atoi(limitStr)
		if err != nil {
			err = fmt.Errorf("%w: invalid limit format, expected integer", serverops.ErrUnprocessableEntity)
			_ = serverops.Error(w, r, err, serverops.ListOperation)
			return
		}
		limit = i
	}

	pools, err := h.service.ListByPurpose(ctx, purpose, cursor, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pools) // @response []runtimetypes.Pool
}

// Backend association handlers
func (h *poolHandler) assignBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	backendID := url.PathEscape(r.PathValue("backendID"))

	if poolID == "" || backendID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID and backendID are required: %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}

	if err := h.service.AssignBackend(ctx, poolID, backendID); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}
	_ = serverops.Encode(w, r, http.StatusCreated, "backend assigned") // @response string
}

func (h *poolHandler) removeBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	backendID := url.PathEscape(r.PathValue("backendID"))

	if poolID == "" || backendID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID and backendID required: %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}

	if err := h.service.RemoveBackend(ctx, poolID, backendID); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "backend removed") // @response string
}

func (h *poolHandler) listBackends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	if poolID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID required: %w", serverops.ErrBadPathValue), serverops.ListOperation)
		return
	}

	backends, err := h.service.ListBackends(ctx, poolID)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, backends) // @response []runtimetypes.Backend
}

func (h *poolHandler) listPoolsForBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	backendID := url.PathEscape(r.PathValue("backendID"))
	if backendID == "" {
		serverops.Error(w, r, fmt.Errorf("backendID required: %w", serverops.ErrBadPathValue), serverops.ListOperation)
		return
	}

	pools, err := h.service.ListPoolsForBackend(ctx, backendID)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pools) // @response []runtimetypes.Pool
}

// Model association handlers
func (h *poolHandler) assignModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	modelID := url.PathEscape(r.PathValue("modelID"))

	if poolID == "" || modelID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID and modelID required: %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}

	if err := h.service.AssignModel(ctx, poolID, modelID); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "model assigned") // @response string
}

func (h *poolHandler) removeModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	modelID := url.PathEscape(r.PathValue("modelID"))

	if poolID == "" || modelID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID and modelID required: %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}

	if err := h.service.RemoveModel(ctx, poolID, modelID); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "model removed") // @response string
}

func (h *poolHandler) listModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	poolID := url.PathEscape(r.PathValue("poolID"))
	if poolID == "" {
		serverops.Error(w, r, fmt.Errorf("poolID required: %w", serverops.ErrBadPathValue), serverops.ListOperation)
		return
	}

	models, err := h.service.ListModels(ctx, poolID)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, models) // @response []runtimetypes.Model
}

func (h *poolHandler) listPoolsForModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := url.PathEscape(r.PathValue("modelID"))
	if modelID == "" {
		serverops.Error(w, r, fmt.Errorf("modelID required: %w", serverops.ErrBadPathValue), serverops.ListOperation)
		return
	}

	pools, err := h.service.ListPoolsForModel(ctx, modelID)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, pools) // @response []runtimetypes.Pool
}
