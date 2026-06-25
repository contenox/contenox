package modelregistryapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
)

func AddRoutes(
	mux *http.ServeMux,
	svc modelregistryservice.Service,
	reg modelregistry.Registry,
	backendSvc backendservice.Service,
	store runtimetypes.Store,
) {
	h := &handler{svc: svc, reg: reg, backendSvc: backendSvc, store: store}
	mux.HandleFunc("POST /model-registry", h.create)
	mux.HandleFunc("GET /model-registry", h.list)
	mux.HandleFunc("POST /model-registry/download", h.download)
	mux.HandleFunc("GET /model-registry/{id}", h.get)
	mux.HandleFunc("PUT /model-registry/{id}", h.update)
	mux.HandleFunc("DELETE /model-registry/{id}", h.delete)
}

type handler struct {
	svc        modelregistryservice.Service
	reg        modelregistry.Registry
	backendSvc backendservice.Service
	store      runtimetypes.Store
}

type downloadRequest struct {
	Name string `json:"name"`
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, err := apiframework.Decode[runtimetypes.ModelRegistryEntry](r) // @request runtimetypes.ModelRegistryEntry
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	e.ID = uuid.NewString()
	if err := h.svc.Create(ctx, &e); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, e) // @response runtimetypes.ModelRegistryEntry
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	entries, err := h.reg.List(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, entries) // @response []modelregistry.ModelDescriptor
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the model registry entry.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.GetOperation)
		return
	}
	e, err := h.svc.Get(ctx, id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, e) // @response runtimetypes.ModelRegistryEntry
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the model registry entry.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.UpdateOperation)
		return
	}
	e, err := apiframework.Decode[runtimetypes.ModelRegistryEntry](r) // @request runtimetypes.ModelRegistryEntry
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	e.ID = id
	if err := h.svc.Update(ctx, &e); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, e) // @response runtimetypes.ModelRegistryEntry
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := apiframework.GetPathParam(r, "id", "The unique identifier for the model registry entry.")
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("missing id parameter %w", apiframework.ErrBadPathValue), apiframework.DeleteOperation)
		return
	}
	if err := h.svc.Delete(ctx, id); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, "model registry entry removed") // @response string
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, err := apiframework.Decode[downloadRequest](r) // @request modelregistryapi.downloadRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if req.Name == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: name is required", apiframework.ErrUnprocessableEntity), apiframework.CreateOperation)
		return
	}

	desc, err := h.reg.Resolve(ctx, req.Name)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("resolve home directory: %w", err), apiframework.CreateOperation)
		return
	}

	modelDir := filepath.Join(homeDir, ".contenox", "models", req.Name)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("create model directory: %w", err), apiframework.CreateOperation)
		return
	}

	destPath := filepath.Join(modelDir, "model.gguf")
	if _, statErr := os.Stat(destPath); statErr == nil {
		h.ensureLocalSetup(ctx, req.Name, homeDir)
		_ = apiframework.Encode(w, r, http.StatusOK, "already downloaded") // @response string
		return
	}

	dlResp, err := http.Get(desc.SourceURL) //nolint:gosec
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("download failed: %w", err), apiframework.CreateOperation)
		return
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		_ = apiframework.Error(w, r, fmt.Errorf("download failed: HTTP %s", dlResp.Status), apiframework.CreateOperation)
		return
	}

	f, err := os.Create(destPath)
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("create file: %w", err), apiframework.CreateOperation)
		return
	}
	if _, err := io.Copy(f, dlResp.Body); err != nil {
		f.Close()
		_ = os.Remove(destPath)
		_ = apiframework.Error(w, r, fmt.Errorf("write file: %w", err), apiframework.CreateOperation)
		return
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(destPath)
		_ = apiframework.Error(w, r, fmt.Errorf("sync file: %w", err), apiframework.CreateOperation)
		return
	}
	f.Close()

	_ = h.svc.Create(ctx, &runtimetypes.ModelRegistryEntry{
		ID:        uuid.NewString(),
		Name:      req.Name,
		SourceURL: desc.SourceURL,
		SizeBytes: desc.SizeBytes,
	})

	h.ensureLocalSetup(ctx, req.Name, homeDir)

	_ = apiframework.Encode(w, r, http.StatusOK, "downloaded") // @response string
}

func (h *handler) ensureLocalSetup(ctx context.Context, modelName, homeDir string) {
	backends, err := h.backendSvc.List(ctx, nil, 100)
	if err != nil {
		slog.Warn("modelregistryapi: failed to list backends for local setup", "err", err)
	} else {
		hasLocal := false
		for _, b := range backends {
			if strings.EqualFold(b.Type, "local") {
				hasLocal = true
				break
			}
		}
		if !hasLocal {
			modelsDir := filepath.Join(homeDir, ".contenox", "models")
			if createErr := h.backendSvc.Create(ctx, &runtimetypes.Backend{
				ID:      uuid.NewString(),
				Name:    "local",
				Type:    "local",
				BaseURL: modelsDir,
			}); createErr != nil {
				slog.Warn("modelregistryapi: failed to create local backend", "err", createErr)
			}
		}
	}

	if provider := clikv.Read(ctx, h.store, "default-provider"); provider == "" || strings.EqualFold(provider, "ollama") {
		if setErr := clikv.SetString(ctx, h.store, "default-provider", "local"); setErr != nil {
			slog.Warn("modelregistryapi: failed to set default-provider", "err", setErr)
		}
	}

	if model := clikv.Read(ctx, h.store, "default-model"); model == "" {
		if setErr := clikv.SetString(ctx, h.store, "default-model", modelName); setErr != nil {
			slog.Warn("modelregistryapi: failed to set default-model", "err", setErr)
		}
	}
}
