package backendapi

import (
	"fmt"
	"net/http"
	"net/url"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/downloadservice"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/store"
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

func (s *service) append(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	model, err := serverops.Decode[store.Model](r)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	model.ID = model.Model
	if err := s.service.Append(ctx, &model); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, model)
}

func (s *service) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get internal models
	internalModels, err := s.service.List(ctx)
	if err != nil {
		serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	type OpenAIModel struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	type ListResponse struct {
		Object string        `json:"object"`
		Data   []OpenAIModel `json:"data"`
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

	serverops.Encode(w, r, http.StatusOK, response)
}

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

	_ = serverops.Encode(w, r, http.StatusOK, "model removed")
}
