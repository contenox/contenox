package providerapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/apiframework"
	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/providerservice"
	"github.com/contenox/runtime/runtimestate"
)

func AddProviderRoutes(mux *http.ServeMux, providerService providerservice.Service) {
	p := &providerManager{providerService: providerService}

	mux.HandleFunc("POST /providers/openai/configure", p.configure("openai"))
	mux.HandleFunc("POST /providers/gemini/configure", p.configure("gemini"))
	mux.HandleFunc("GET /providers/openai/status", p.status("openai"))
	mux.HandleFunc("GET /providers/gemini/status", p.status("gemini"))
	mux.HandleFunc("DELETE /providers/{providerType}/config", p.deleteConfig)
	mux.HandleFunc("GET /providers/configs", p.listConfigs)
	mux.HandleFunc("GET /providers/{providerType}/config", p.get)

}

type providerManager struct {
	providerService providerservice.Service
}

type ConfigureRequest struct {
	APIKey string `json:"apiKey"`
	Upsert bool   `json:"upsert"`
}

type StatusResponse struct {
	Configured bool      `json:"configured"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Provider   string    `json:"provider"`
}

func (p *providerManager) configure(providerType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := apiframework.Decode[ConfigureRequest](r) // @request providerapi.ConfigureRequest
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.CreateOperation)
			return
		}

		if req.APIKey == "" {
			_ = serverops.Error(w, r, fmt.Errorf("api key is required"), serverops.CreateOperation)
			return
		}

		cfg := &runtimestate.ProviderConfig{
			APIKey: req.APIKey,
			Type:   providerType,
		}

		if err := p.providerService.SetProviderConfig(r.Context(), providerType, req.Upsert, cfg); err != nil {
			_ = serverops.Error(w, r, err, serverops.CreateOperation)
			return
		}
		_ = serverops.Encode(w, r, http.StatusOK, StatusResponse{ // @response providerapi.StatusResponse
			Configured: true,
			Provider:   providerType,
		})
	}
}

func (p *providerManager) status(providerType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := p.providerService.GetProviderConfig(r.Context(), providerType)
		if errors.Is(err, libdb.ErrNotFound) {
			_ = serverops.Encode(w, r, http.StatusOK, StatusResponse{
				Configured: false,
				Provider:   providerType,
			})
			return
		}
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.GetOperation)
			return
		}
		_ = serverops.Encode(w, r, http.StatusOK, StatusResponse{ // @response providerapi.StatusResponse
			Configured: true,
			Provider:   providerType,
		})
	}
}

func (p *providerManager) deleteConfig(w http.ResponseWriter, r *http.Request) {
	providerType := r.PathValue("providerType")
	if providerType == "" {
		_ = serverops.Error(w, r, errors.New("providerType is required in path"), serverops.DeleteOperation)
		return
	}

	if err := p.providerService.DeleteProviderConfig(r.Context(), providerType); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "Provider config deleted successfully") // @response string
}

type ListConfigsResponse struct {
	Providers []*runtimestate.ProviderConfig `json:"providers"`
}

func (p *providerManager) listConfigs(w http.ResponseWriter, r *http.Request) {
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

	configs, err := p.providerService.ListProviderConfigs(ctx, cursor, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, configs) // @response []runtimestate.ProviderConfig
}

func (p *providerManager) get(w http.ResponseWriter, r *http.Request) {
	providerType := r.PathValue("providerType")
	if providerType == "" {
		_ = serverops.Error(w, r, errors.New("providerType is required in path"), serverops.GetOperation)
		return
	}

	config, err := p.providerService.GetProviderConfig(r.Context(), providerType)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, config) // @response runtimestate.ProviderConfig
}
