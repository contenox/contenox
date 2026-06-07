package backendapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/google/uuid"
)

func AddBackendRoutes(mux *http.ServeMux, backendService backendservice.Service, stateService stateservice.Service) {
	b := &backendManager{service: backendService, stateService: stateService}

	mux.HandleFunc("POST /backends", b.createBackend)
	mux.HandleFunc("GET /backends", b.listBackends)
	mux.HandleFunc("GET /backends/{id}", b.getBackend)
	mux.HandleFunc("PUT /backends/{id}", b.updateBackend)
	mux.HandleFunc("DELETE /backends/{id}", b.deleteBackend)
}

type backendSummary struct {
	ID      string `json:"id" example:"backend-id"`
	Name    string `json:"name" example:"backend-name"`
	BaseURL string `json:"baseUrl" example:"http://localhost:11434"`
	Type    string `json:"type" example:"ollama"`

	Models       []string                    `json:"models"`
	PulledModels []statetype.ModelPullStatus `json:"pulledModels" openapi_include_type:"statetype.ModelPullStatus"`
	Error        string                      `json:"error,omitempty" example:"error-message"`

	CreatedAt time.Time `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt time.Time `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
}

type backendManager struct {
	service      backendservice.Service
	stateService stateservice.Service
}

func (b *backendManager) createBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	backend, err := apiframework.Decode[runtimetypes.Backend](r) // @request runtimetypes.Backend
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	backend.ID = uuid.NewString()
	if err := b.service.Create(ctx, &backend); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusCreated, backend) // @response runtimetypes.Backend
}

func (b *backendManager) listBackends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limitStr := apiframework.GetQueryParam(r, "limit", "100", "The maximum number of items to return per page.")
	cursorStr := apiframework.GetQueryParam(r, "cursor", "", "An optional RFC3339Nano timestamp to fetch the next page of results.")

	var cursor *time.Time
	if cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			err = fmt.Errorf("%w: invalid cursor format, expected RFC3339Nano", apiframework.ErrUnprocessableEntity)
			_ = apiframework.Error(w, r, err, apiframework.ListOperation)
			return
		}
		cursor = &t
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		err = fmt.Errorf("%w: invalid limit format, expected integer", apiframework.ErrUnprocessableEntity)
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	backends, err := b.service.List(ctx, cursor, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	backendState, err := b.stateService.Get(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	resp := []backendSummary{}
	for _, backend := range backends {
		item := backendSummary{
			ID:        backend.ID,
			Name:      backend.Name,
			BaseURL:   backend.BaseURL,
			Type:      backend.Type,
			CreatedAt: backend.CreatedAt,
			UpdatedAt: backend.UpdatedAt,
		}
		ok := false
		var itemState statetype.BackendRuntimeState
		for _, l := range backendState {
			if l.ID == backend.ID {
				ok = true
				itemState = l
				break
			}
		}
		if ok {
			item.Models = observedModelNames(itemState)
			item.PulledModels = itemState.PulledModels
			item.Error = itemState.Error
		}
		resp = append(resp, item)
	}

	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response []backendapi.backendSummary
}

type backendDetails struct {
	ID           string                      `json:"id" example:"b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e"`
	Name         string                      `json:"name" example:"ollama-production"`
	BaseURL      string                      `json:"baseUrl" example:"http://ollama-prod.internal:11434"`
	Type         string                      `json:"type" example:"ollama"`
	Models       []string                    `json:"models" example:"[\"mistral:instruct\", \"llama2:7b\", \"nomic-embed-text:latest\"]"`
	PulledModels []statetype.ModelPullStatus `json:"pulledModels" openapi_include_type:"statetype.ModelPullStatus"`
	Error        string                      `json:"error,omitempty" example:"connection timeout: context deadline exceeded"`
	CreatedAt    time.Time                   `json:"createdAt" example:"2023-11-15T14:30:45Z"`
	UpdatedAt    time.Time                   `json:"updatedAt" example:"2023-11-15T14:30:45Z"`
}

func (b *backendManager) getBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the backend.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.GetOperation)
		return
	}

	backend, err := b.service.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	state, err := b.stateService.Get(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	ok := false
	var itemState statetype.BackendRuntimeState
	for _, l := range state {
		if l.ID == id {
			ok = true
			itemState = l
			break
		}
	}

	resp := backendDetails{
		ID:           backend.ID,
		Name:         backend.Name,
		BaseURL:      backend.BaseURL,
		Type:         backend.Type,
		Models:       []string{},
		PulledModels: []statetype.ModelPullStatus{},
		Error:        "",
		CreatedAt:    backend.CreatedAt,
		UpdatedAt:    backend.UpdatedAt,
	}

	if ok {
		resp.Models = observedModelNames(itemState)
		resp.PulledModels = itemState.PulledModels
		resp.Error = itemState.Error
	}

	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response backendapi.backendDetails
}

func (b *backendManager) updateBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the backend.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.UpdateOperation)
		return
	}
	backend, err := apiframework.Decode[runtimetypes.Backend](r) // @request runtimetypes.Backend
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	backend.ID = id
	if err := b.service.Update(ctx, &backend); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, backend) // @response runtimetypes.Backend
}

func (b *backendManager) deleteBackend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the backend.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.DeleteOperation)
		return
	}
	if err := b.service.Delete(ctx, id); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, "backend removed") // @response string
}
