package providerapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	libdb "github.com/contenox/dbexec"
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
	// mux.HandleFunc("GET /providers", p.listProviders)
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
		var req ConfigureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		_ = serverops.Encode(w, r, http.StatusOK, StatusResponse{
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
		_ = serverops.Encode(w, r, http.StatusOK, StatusResponse{
			Configured: true,
			Provider:   providerType,
		})
	}
}
