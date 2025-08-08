package backendapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/backendservice"
	"github.com/contenox/runtime/runtimestate"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/stateservice"
	"github.com/google/uuid"
)

func AddBackendRoutes(mux *http.ServeMux, backendService backendservice.Service, stateService stateservice.Service) {
	b := &backendManager{service: backendService, stateService: stateService}

	mux.HandleFunc("POST /backends", b.create)
	mux.HandleFunc("GET /backends", b.list)
	mux.HandleFunc("GET /backends/{id}", b.get)
	mux.HandleFunc("PUT /backends/{id}", b.update)
	mux.HandleFunc("DELETE /backends/{id}", b.delete)
}

type respBackendList struct {
	ID      string `json:"id" example:"backend-id"`
	Name    string `json:"name" example:"backend-name"`
	BaseURL string `json:"baseUrl" example:"http://localhost:11434"`
	Type    string `json:"type" example:"ollama"`

	Models       []string                         `json:"models"`
	PulledModels []runtimestate.ListModelResponse `json:"pulledModels" @include:runtimestate.ListModelResponse`
	Error        string                           `json:"error,omitempty" example:"error-message"`

	CreatedAt time.Time `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt time.Time `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
}

type backendManager struct {
	service      backendservice.Service
	stateService stateservice.Service
}

// Creates a new backend connection to an LLM provider.
// Backends represent connections to LLM services (e.g., Ollama, OpenAI) that can host models.
// Note: Creating a backend will be provisioned on the next synchronization cycle.
func (b *backendManager) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	backend, err := serverops.Decode[runtimetypes.Backend](r) // @request runtimetypes.Backend
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}
	backend.ID = uuid.NewString()
	if err := b.service.Create(ctx, &backend); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, backend) // @response runtimetypes.Backend
}

// Lists all configured backend connections with runtime status.
// NOTE: Only backends assigned to at least one pool will be used for request processing.
// Backends not assigned to any pool exist in the configuration but are completely ignored by the routing system.
func (b *backendManager) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	backends, err := b.service.List(ctx, cursor, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	backendState, err := b.stateService.Get(ctx)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	resp := []respBackendList{}
	for _, backend := range backends {
		item := respBackendList{
			ID:      backend.ID,
			Name:    backend.Name,
			BaseURL: backend.BaseURL,
			Type:    backend.Type,
		}
		ok := false
		var itemState runtimestate.LLMState
		for _, l := range backendState {
			if l.ID == backend.ID {
				ok = true

				itemState = l
				break
			}
		}
		if ok {
			item.Models = itemState.Models
			item.PulledModels = itemState.PulledModels
			item.Error = itemState.Error
		}
		resp = append(resp, item)
	}

	_ = serverops.Encode(w, r, http.StatusOK, resp) // @response []backendapi.respBackendList
}

type respBackend struct {
	ID           string                           `json:"id"`
	Name         string                           `json:"name"`
	BaseURL      string                           `json:"baseUrl"`
	Type         string                           `json:"type"`
	Models       []string                         `json:"models"`
	PulledModels []runtimestate.ListModelResponse `json:"pulledModels" @include:"runtimestate.ListModelResponse"`
	Error        string                           `json:"error,omitempty"`
	CreatedAt    time.Time                        `json:"createdAt"`
	UpdatedAt    time.Time                        `json:"updatedAt"`
}

// Retrieves complete information for a specific backend
func (b *backendManager) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		serverops.Error(w, r, fmt.Errorf("missing id parameter %w", serverops.ErrBadPathValue), serverops.GetOperation)
		return
	}

	// Get static backend info
	backend, err := b.service.Get(ctx, id)
	if err != nil {
		serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	// Get dynamic runtime state
	state, err := b.stateService.Get(ctx)
	if err != nil {
		serverops.Error(w, r, err, serverops.GetOperation)
		return
	}
	ok := false
	var itemState runtimestate.LLMState
	for _, l := range state {
		if l.ID == id {
			ok = true

			itemState = l
			break
		}
	}

	resp := respBackend{
		ID:           backend.ID,
		Name:         backend.Name,
		BaseURL:      backend.BaseURL,
		Type:         backend.Type,
		Models:       []string{},
		PulledModels: []runtimestate.ListModelResponse{},
		Error:        "",
		CreatedAt:    backend.CreatedAt,
		UpdatedAt:    backend.UpdatedAt,
	}

	if ok {
		resp.Models = itemState.Models
		resp.PulledModels = itemState.PulledModels
		resp.Error = itemState.Error
	}

	serverops.Encode(w, r, http.StatusOK, resp) // @response backendapi.respBackend
}

// Updates an existing backend configuration.
// The ID from the URL path overrides any ID in the request body.
// Note: Updating a backend will be provisioned on the next synchronization cycle.
func (b *backendManager) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		_ = serverops.Error(w, r, fmt.Errorf("missing id parameter %w", serverops.ErrBadPathValue), serverops.UpdateOperation)
		return
	}
	backend, err := serverops.Decode[runtimetypes.Backend](r) // @request runtimetypes.Backend
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	backend.ID = id
	if err := b.service.Update(ctx, &backend); err != nil {
		_ = serverops.Error(w, r, err, serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, backend) // @response runtimetypes.Backend
}

// Removes a backend connection.
// This does not delete models from the remote provider, only removes the connection.
// Returns a simple "backend removed" confirmation message on success.
func (b *backendManager) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		_ = serverops.Error(w, r, fmt.Errorf("missing id parameter %w", serverops.ErrBadPathValue), serverops.DeleteOperation)
		return
	}
	if err := b.service.Delete(ctx, id); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "backend removed") // @response string
}
