package providerapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/providerservice"
)

func AddProviderRoutes(mux *http.ServeMux, providerService providerservice.Service) {
	p := &providerManager{providerService: providerService}

	mux.HandleFunc("POST /providers/{providerType}/configure", p.configure)
	mux.HandleFunc("GET /providers/{providerType}/status", p.status)
	mux.HandleFunc("DELETE /providers/{providerType}/config", p.deleteConfig)
	mux.HandleFunc("GET /providers/{providerType}/config", p.get)
	mux.HandleFunc("GET /providers/configs", p.listConfigs)
	mux.HandleFunc("GET /providers/supported", p.supported)
}

type providerManager struct {
	providerService providerservice.Service
}

type ConfigureRequest struct {
	APIKey       string `json:"apiKey"`
	APIKeyEnv    string `json:"apiKeyEnv"`
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel"`
	Upsert       bool   `json:"upsert"`
	SetDefault   *bool  `json:"setDefault"`
}

func (p *providerManager) configure(w http.ResponseWriter, r *http.Request) {
	providerType := apiframework.GetPathParam(r, "providerType", "Provider type to configure.")
	if providerType == "" {
		_ = apiframework.Error(w, r, errors.New("providerType is required in path"), apiframework.CreateOperation)
		return
	}
	req, err := apiframework.Decode[ConfigureRequest](r) // @request providerapi.ConfigureRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	setDefault := true
	if req.SetDefault != nil {
		setDefault = *req.SetDefault
	}
	status, err := p.providerService.Configure(r.Context(), providerType, providerservice.ConfigureProviderRequest{
		APIKey:       req.APIKey,
		APIKeyEnv:    req.APIKeyEnv,
		BaseURL:      req.BaseURL,
		DefaultModel: req.DefaultModel,
		Upsert:       req.Upsert,
		SetDefault:   setDefault,
	})
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, status) // @response providerservice.ProviderStatus
}

func (p *providerManager) status(w http.ResponseWriter, r *http.Request) {
	providerType := apiframework.GetPathParam(r, "providerType", "Provider type to inspect.")
	if providerType == "" {
		_ = apiframework.Error(w, r, errors.New("providerType is required in path"), apiframework.GetOperation)
		return
	}
	status, err := p.providerService.GetProviderConfig(r.Context(), providerType)
	if errors.Is(err, libdb.ErrNotFound) {
		_ = apiframework.Encode(w, r, http.StatusOK, providerservice.ProviderStatus{
			Provider:     providerType,
			Configured:   false,
			SecretSource: "none",
		}) // @response providerservice.ProviderStatus
		return
	}
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, status) // @response providerservice.ProviderStatus
}

func (p *providerManager) deleteConfig(w http.ResponseWriter, r *http.Request) {
	providerType := apiframework.GetPathParam(r, "providerType", "Provider type to delete.")
	if providerType == "" {
		_ = apiframework.Error(w, r, errors.New("providerType is required in path"), apiframework.DeleteOperation)
		return
	}
	if err := p.providerService.DeleteProviderConfig(r.Context(), providerType); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, "provider config deleted") // @response string
}

func (p *providerManager) listConfigs(w http.ResponseWriter, r *http.Request) {
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

	configs, err := p.providerService.ListProviderConfigs(ctx, cursor, limit)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, configs) // @response []providerservice.ProviderStatus
}

func (p *providerManager) supported(w http.ResponseWriter, r *http.Request) {
	providers, err := p.providerService.ListSupportedProviders(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, providers) // @response []providerservice.ProviderCapability
}

func (p *providerManager) get(w http.ResponseWriter, r *http.Request) {
	providerType := apiframework.GetPathParam(r, "providerType", "Provider type to retrieve.")
	if providerType == "" {
		_ = apiframework.Error(w, r, errors.New("providerType is required in path"), apiframework.GetOperation)
		return
	}

	config, err := p.providerService.GetProviderConfig(r.Context(), providerType)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	_ = apiframework.Encode(w, r, http.StatusOK, config) // @response providerservice.ProviderStatus
}
