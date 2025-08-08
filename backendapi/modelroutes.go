package backendapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/downloadservice"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/runtimetypes"
)

func AddModelRoutes(mux *http.ServeMux, modelService modelservice.Service, dwService downloadservice.Service) {
	s := &service{service: modelService, dwService: dwService}

	mux.HandleFunc("POST /models", s.append)
	mux.HandleFunc("GET /models", s.list)
	// mux.HandleFunc("GET /v1/models/{model}", s.modelDetails) // TODO: Implement model details endpoint
	mux.HandleFunc("DELETE /models/{model}", s.delete)
}

type service struct {
	service   modelservice.Service
	dwService downloadservice.Service
}

// Declares a new model to the system.
// The model must be available in a configured backend or will be queued for download.
// IMPORTANT: Models not assigned to any pool will NOT be available for request processing.
// If pools are enabled, to make a model available to backends, it must be explicitly added to at least one pool.
func (s *service) append(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	model, err := serverops.Decode[runtimetypes.Model](r) // @request runtimetypes.Model
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	model.ID = model.Model
	if err := s.service.Append(ctx, &model); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, model) // @response runtimetypes.Model
}

type OpenAIModel struct {
	ID      string `json:"id" example:"mistral:latest"`
	Object  string `json:"object" example:"model"`
	Created int64  `json:"created" example:"1717020800"`
	OwnedBy string `json:"owned_by" example:"system"`
}

type ListResponse struct {
	Object string        `json:"object" example:"list"`
	Data   []OpenAIModel `json:"data"`
}

// Lists all registered models in OpenAI-compatible format.
// Returns models as they would appear in OpenAI's /v1/models endpoint.
// NOTE: Only models assigned to at least one pool will be available for request processing.
// Models not assigned to any pool exist in the configuration but are completely ignored by the routing system.
func (s *service) list(w http.ResponseWriter, r *http.Request) {
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

	// Get internal models with pagination
	internalModels, err := s.service.List(ctx, cursor, limit)
	if err != nil {
		serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	openAIModels := make([]OpenAIModel, len(internalModels))

	for i, m := range internalModels {
		openAIModels[i] = OpenAIModel{
			ID:      m.Model,
			Object:  "model",
			Created: m.CreatedAt.Unix(),
			OwnedBy: "system",
		}
	}

	response := ListResponse{
		Object: "list",
		Data:   openAIModels,
	}

	serverops.Encode(w, r, http.StatusOK, response) // @response backendapi.ListResponse
}

// Deletes a model from the system registry.
// - Does not remove the model from backend storage (requires separate backend operation)
// - Accepts 'purge=true' query parameter to also remove related downloads from queue
func (s *service) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelName := url.PathEscape(r.PathValue("model"))
	if modelName == "" {
		serverops.Error(w, r, fmt.Errorf("model name is required: %w", serverops.ErrBadPathValue), serverops.DeleteOperation)
		return
	}
	if err := s.service.Delete(ctx, modelName); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}
	queue := r.URL.Query().Get("purge")
	if queue == "true" {
		if err := s.dwService.RemoveDownloadFromQueue(r.Context(), modelName); err != nil {
			_ = serverops.Error(w, r, err, serverops.DeleteOperation)
			return
		}
		if err := s.dwService.CancelDownloads(r.Context(), modelName); err != nil {
			_ = serverops.Error(w, r, err, serverops.DeleteOperation)
			return
		}
	}

	_ = serverops.Encode(w, r, http.StatusOK, "model removed") // @response string
}
