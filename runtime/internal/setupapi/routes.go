package setupapi

import (
	"net/http"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/runtime/stateservice"
)

func AddSetupRoutes(mux *http.ServeMux, stateService stateservice.Service, auth middleware.AuthZReader) {
	h := &setupHandler{state: stateService, auth: auth}
	mux.HandleFunc("GET /setup-status", h.getStatus)
	mux.HandleFunc("POST /setup/refresh", h.refreshStatus)
	mux.HandleFunc("PUT /cli-config", h.putCLIConfig)
}

type setupHandler struct {
	state stateservice.Service
	auth  middleware.AuthZReader
}

func (h *setupHandler) authorize(r *http.Request) error {
	if h.auth == nil {
		return nil
	}
	_, err := h.auth.GetIdentity(r.Context())
	return err
}

func (h *setupHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	res, err := h.state.SetupStatus(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, res) // @response setupcheck.Result
}

func (h *setupHandler) refreshStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	res, err := h.state.Refresh(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, res) // @response setupcheck.Result
}

type putCLIConfigRequest struct {
	DefaultModel       *string `json:"default-model"`
	DefaultProvider    *string `json:"default-provider"`
	DefaultAltModel    *string `json:"default-alt-model"`
	DefaultAltProvider *string `json:"default-alt-provider"`
	DefaultMaxTokens   *string `json:"default-max-tokens"`
	DefaultChain       *string `json:"default-chain"`
	HITLPolicyName     *string `json:"hitl-policy-name"`
}

type putCLIConfigResponse struct {
	DefaultModel       string            `json:"defaultModel"`
	DefaultProvider    string            `json:"defaultProvider"`
	DefaultAltModel    string            `json:"defaultAltModel"`
	DefaultAltProvider string            `json:"defaultAltProvider"`
	DefaultMaxTokens   string            `json:"defaultMaxTokens"`
	DefaultChain       string            `json:"defaultChain"`
	HITLPolicyName     string            `json:"hitlPolicyName"`
	ResolvedFrom       map[string]string `json:"resolvedFrom,omitempty"`
}

func (h *setupHandler) putCLIConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	body, err := apiframework.Decode[putCLIConfigRequest](r) // @request setupapi.putCLIConfigRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	if body.DefaultModel == nil &&
		body.DefaultProvider == nil &&
		body.DefaultAltModel == nil &&
		body.DefaultAltProvider == nil &&
		body.DefaultMaxTokens == nil &&
		body.DefaultChain == nil &&
		body.HITLPolicyName == nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("Provide at least one CLI config key."), apiframework.UpdateOperation)
		return
	}
	snap, err := h.state.SetCLIConfig(ctx, stateservice.CLIConfigPatch{
		DefaultModel:       body.DefaultModel,
		DefaultProvider:    body.DefaultProvider,
		DefaultAltModel:    body.DefaultAltModel,
		DefaultAltProvider: body.DefaultAltProvider,
		DefaultMaxTokens:   body.DefaultMaxTokens,
		DefaultChain:       body.DefaultChain,
		HITLPolicyName:     body.HITLPolicyName,
	})
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	resp := putCLIConfigResponse{
		DefaultModel:       snap.DefaultModel,
		DefaultProvider:    snap.DefaultProvider,
		DefaultAltModel:    snap.DefaultAltModel,
		DefaultAltProvider: snap.DefaultAltProvider,
		DefaultMaxTokens:   snap.DefaultMaxTokens,
		DefaultChain:       snap.DefaultChain,
		HITLPolicyName:     snap.HITLPolicyName,
		ResolvedFrom:       snap.ResolvedFrom,
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response setupapi.putCLIConfigResponse
}
