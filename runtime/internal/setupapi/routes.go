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
	mux.HandleFunc("GET /cli-config", h.getCLIConfig)
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

// getStatus reports the cached setup-check result: which pieces of the local
// runtime are configured and working.
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

// refreshStatus re-runs the setup checks and returns the fresh result.
func (h *setupHandler) refreshStatus(w http.ResponseWriter, r *http.Request) {
	// @request none triggers a server-side re-check of the setup state; the request carries no body
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
	DefaultModel                *string `json:"default-model"`
	DefaultProvider             *string `json:"default-provider"`
	DefaultAltModel             *string `json:"default-alt-model"`
	DefaultAltProvider          *string `json:"default-alt-provider"`
	DefaultAutocompleteModel    *string `json:"default-autocomplete-model"`
	DefaultAutocompleteProvider *string `json:"default-autocomplete-provider"`
	DefaultMaxTokens            *string `json:"default-max-tokens"`
	DefaultThink                *string `json:"default-think"`
	DefaultChain                *string `json:"default-chain"`
	HITLPolicyName              *string `json:"hitl-policy-name"`
	TelemetryEnabled            *string `json:"telemetry-enabled"`
	UpdateCheck                 *string `json:"update-check"`
}

// putCLIConfigResponse is the resolved CLI config snapshot, shared by
// GET /cli-config (full read) and PUT /cli-config (post-update read).
type putCLIConfigResponse struct {
	DefaultModel                string            `json:"defaultModel"`
	DefaultProvider             string            `json:"defaultProvider"`
	DefaultAltModel             string            `json:"defaultAltModel"`
	DefaultAltProvider          string            `json:"defaultAltProvider"`
	DefaultAutocompleteModel    string            `json:"defaultAutocompleteModel"`
	DefaultAutocompleteProvider string            `json:"defaultAutocompleteProvider"`
	DefaultMaxTokens            string            `json:"defaultMaxTokens"`
	DefaultThink                string            `json:"defaultThink"`
	DefaultChain                string            `json:"defaultChain"`
	HITLPolicyName              string            `json:"hitlPolicyName"`
	TelemetryEnabled            string            `json:"telemetryEnabled"`
	UpdateCheck                 string            `json:"updateCheck"`
	ResolvedFrom                map[string]string `json:"resolvedFrom,omitempty"`
}

func cliConfigResponseFromSnapshot(snap stateservice.CLIConfigSnapshot) putCLIConfigResponse {
	return putCLIConfigResponse{
		DefaultModel:                snap.DefaultModel,
		DefaultProvider:             snap.DefaultProvider,
		DefaultAltModel:             snap.DefaultAltModel,
		DefaultAltProvider:          snap.DefaultAltProvider,
		DefaultAutocompleteModel:    snap.DefaultAutocompleteModel,
		DefaultAutocompleteProvider: snap.DefaultAutocompleteProvider,
		DefaultMaxTokens:            snap.DefaultMaxTokens,
		DefaultThink:                snap.DefaultThink,
		DefaultChain:                snap.DefaultChain,
		HITLPolicyName:              snap.HITLPolicyName,
		TelemetryEnabled:            snap.TelemetryEnabled,
		UpdateCheck:                 snap.UpdateCheck,
		ResolvedFrom:                snap.ResolvedFrom,
	}
}

// getCLIConfig returns the resolved CLI configuration snapshot (default
// model, provider, chain, and related settings).
func (h *setupHandler) getCLIConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(r); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	snap, err := h.state.CLIConfig(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, cliConfigResponseFromSnapshot(snap)) // @response setupapi.putCLIConfigResponse
}

// putCLIConfig updates the provided CLI configuration keys and returns the
// resolved snapshot after the change.
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
		body.DefaultAutocompleteModel == nil &&
		body.DefaultAutocompleteProvider == nil &&
		body.DefaultMaxTokens == nil &&
		body.DefaultThink == nil &&
		body.DefaultChain == nil &&
		body.HITLPolicyName == nil &&
		body.TelemetryEnabled == nil &&
		body.UpdateCheck == nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("Provide at least one CLI config key."), apiframework.UpdateOperation)
		return
	}
	snap, err := h.state.SetCLIConfig(ctx, stateservice.CLIConfigPatch{
		DefaultModel:                body.DefaultModel,
		DefaultProvider:             body.DefaultProvider,
		DefaultAltModel:             body.DefaultAltModel,
		DefaultAltProvider:          body.DefaultAltProvider,
		DefaultAutocompleteModel:    body.DefaultAutocompleteModel,
		DefaultAutocompleteProvider: body.DefaultAutocompleteProvider,
		DefaultMaxTokens:            body.DefaultMaxTokens,
		DefaultThink:                body.DefaultThink,
		DefaultChain:                body.DefaultChain,
		HITLPolicyName:              body.HITLPolicyName,
		TelemetryEnabled:            body.TelemetryEnabled,
		UpdateCheck:                 body.UpdateCheck,
	})
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.UpdateOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, cliConfigResponseFromSnapshot(snap)) // @response setupapi.putCLIConfigResponse
}
