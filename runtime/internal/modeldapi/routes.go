package modeldapi

import (
	"context"
	"net/http"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

const statusTimeout = 2 * time.Second

// AddRoutes registers read-only modeld observability routes. Routes are mounted
// below /api by the containing server.
func AddRoutes(mux *http.ServeMux) {
	addRoutesWithProvider(mux, liveStatusProvider{})
}

type statusProvider interface {
	Detect(context.Context) modeldprobe.Status
	Status(context.Context) (transport.DaemonStatus, error)
}

type liveStatusProvider struct{}

func (liveStatusProvider) Detect(ctx context.Context) modeldprobe.Status {
	return modeldprobe.New("").Probe(ctx)
}

func (liveStatusProvider) Status(ctx context.Context) (transport.DaemonStatus, error) {
	return modeldconn.Status(ctx)
}

func addRoutesWithProvider(mux *http.ServeMux, provider statusProvider) {
	h := &handler{provider: provider}
	mux.HandleFunc("GET /modeld/status", h.status)
}

type handler struct {
	provider statusProvider
}

// StatusResponse is Beam's curated view of modeld daemon state. It deliberately
// excludes filesystem paths from ActiveModel; the browser gets logical model
// identity and slot state, not daemon-local path details.
type StatusResponse struct {
	State              string      `json:"state" example:"running"`
	Available          bool        `json:"available" example:"true"`
	Binary             string      `json:"binary,omitempty" example:"/home/user/.contenox/modeld/v0.1.0/linux-amd64/modeld"`
	Endpoint           string      `json:"endpoint,omitempty" example:"127.0.0.1:42001"`
	Instance           string      `json:"instance,omitempty" example:"5f2a23ad-3d9f-46dd-bc21-4c6c2f901e22"`
	Backend            string      `json:"backend,omitempty" example:"llama"`
	Error              string      `json:"error,omitempty" example:"modeld is not running"`
	RuntimeProtocol    int         `json:"runtimeProtocol" example:"1"`
	MinRuntimeProtocol int         `json:"minRuntimeProtocol" example:"1"`
	Slot               *SlotStatus `json:"slot,omitempty" openapi_include_type:"modeldapi.SlotStatus"`
}

type SlotStatus struct {
	OwnerInstanceID string       `json:"ownerInstanceId,omitempty" example:"5f2a23ad-3d9f-46dd-bc21-4c6c2f901e22"`
	Backend         string       `json:"backend,omitempty" example:"llama"`
	State           string       `json:"state,omitempty" example:"Ready"`
	Active          *ActiveModel `json:"active,omitempty" openapi_include_type:"modeldapi.ActiveModel"`
	BusyOperation   string       `json:"busyOperation,omitempty" example:"load"`
	LastError       string       `json:"lastError,omitempty" example:"model does not fit"`
}

type ActiveModel struct {
	ModelName  string        `json:"modelName,omitempty" example:"qwen3-8b"`
	Type       string        `json:"type,omitempty" example:"llama"`
	Digest     string        `json:"digest,omitempty" example:"sha256:abcdef"`
	Config     RuntimeConfig `json:"config"`
	Generation uint64        `json:"generation" example:"3"`
}

type RuntimeConfig struct {
	NumCtx                  int       `json:"numCtx,omitempty"`
	HotContextTokens        int       `json:"hotContextTokens,omitempty"`
	PlannerEffectiveContext int       `json:"plannerEffectiveContext,omitempty"`
	NumBatch                int       `json:"numBatch,omitempty"`
	NumThreads              int       `json:"numThreads,omitempty"`
	NumGpuLayers            int       `json:"numGpuLayers,omitempty"`
	TensorSplit             []float32 `json:"tensorSplit,omitempty"`
	FlashAttn               bool      `json:"flashAttn,omitempty"`
	KVCacheType             string    `json:"kvCacheType,omitempty"`
	PromptFormat            string    `json:"promptFormat,omitempty"`
	PromptTemplateDigest    string    `json:"promptTemplateDigest,omitempty"`
	DisableBOS              bool      `json:"disableBOS,omitempty"`
	ReasoningFormat         string    `json:"reasoningFormat,omitempty"`
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()

	detected := h.provider.Detect(ctx)
	resp := StatusResponse{
		State:              detected.State.String(),
		Binary:             detected.Binary,
		Endpoint:           detected.Endpoint,
		Instance:           detected.Instance,
		Backend:            detected.Backend,
		RuntimeProtocol:    transport.ProtocolVersion,
		MinRuntimeProtocol: transport.MinProtocol,
	}

	if err := detected.Err(); err != nil {
		resp.Error = err.Error()
		_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response modeldapi.StatusResponse
		return
	}

	slot, err := h.provider.Status(ctx)
	if err != nil {
		resp.Error = err.Error()
		_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response modeldapi.StatusResponse
		return
	}

	resp.Available = true
	if resp.Backend == "" {
		resp.Backend = slot.Backend
	}
	resp.Slot = sanitizeSlot(slot)
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response modeldapi.StatusResponse
}

func sanitizeSlot(status transport.DaemonStatus) *SlotStatus {
	out := &SlotStatus{
		OwnerInstanceID: status.OwnerInstanceID,
		Backend:         status.Backend,
		State:           string(status.State),
		BusyOperation:   status.BusyOperation,
		LastError:       status.LastError,
	}
	if status.Active != nil {
		out.Active = &ActiveModel{
			ModelName:  status.Active.ModelName,
			Type:       status.Active.Type,
			Digest:     status.Active.Digest,
			Config:     sanitizeConfig(status.Active.Config),
			Generation: status.Active.Generation,
		}
	}
	return out
}

func sanitizeConfig(cfg transport.Config) RuntimeConfig {
	return RuntimeConfig{
		NumCtx:                  cfg.NumCtx,
		HotContextTokens:        cfg.HotContextTokens,
		PlannerEffectiveContext: cfg.PlannerEffectiveContext,
		NumBatch:                cfg.NumBatch,
		NumThreads:              cfg.NumThreads,
		NumGpuLayers:            cfg.NumGpuLayers,
		TensorSplit:             cfg.TensorSplit,
		FlashAttn:               cfg.FlashAttn,
		KVCacheType:             cfg.KVCacheType,
		PromptFormat:            cfg.PromptFormat,
		PromptTemplateDigest:    cfg.PromptTemplateDigest,
		DisableBOS:              cfg.DisableBOS,
		ReasoningFormat:         cfg.ReasoningFormat,
	}
}
