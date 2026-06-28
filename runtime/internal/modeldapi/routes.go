package modeldapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/transport"
)

const statusTimeout = 2 * time.Second
const controlTimeout = 10 * time.Second

// AddRoutes registers modeld observability and safe single-slot control routes.
// Routes are mounted below /api by the containing server.
func AddRoutes(mux *http.ServeMux, opts ...Option) {
	h := &handler{provider: liveModeldProvider{}}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	h.register(mux)
}

// Option configures modeld routes.
type Option func(*handler)

type stateReader interface {
	Get(context.Context) ([]statetype.BackendRuntimeState, error)
}

// WithStateReader enables registry-backed local model listing and capacity
// diagnostics. The reader is normally the runtime stateservice.
func WithStateReader(reader stateReader) Option {
	return func(h *handler) {
		h.state = reader
	}
}

type modeldProvider interface {
	Detect(context.Context) modeldprobe.Status
	Status(context.Context) (transport.DaemonStatus, error)
	LoadModel(context.Context, modeldconn.ModelRef, transport.Config, uint64) (transport.ActiveModel, error)
	UnloadModel(context.Context, uint64) error
	Describe(context.Context, modeldconn.ModelRef, transport.Config) (transport.ModelInfo, error)
}

type liveModeldProvider struct{}

func (liveModeldProvider) Detect(ctx context.Context) modeldprobe.Status {
	return modeldprobe.New("").Probe(ctx)
}

func (liveModeldProvider) Status(ctx context.Context) (transport.DaemonStatus, error) {
	return modeldconn.Status(ctx)
}

func (liveModeldProvider) LoadModel(ctx context.Context, ref modeldconn.ModelRef, cfg transport.Config, expectedGeneration uint64) (transport.ActiveModel, error) {
	return modeldconn.LoadModel(ctx, ref, cfg, expectedGeneration)
}

func (liveModeldProvider) UnloadModel(ctx context.Context, expectedGeneration uint64) error {
	return modeldconn.UnloadModel(ctx, expectedGeneration)
}

func (liveModeldProvider) Describe(ctx context.Context, ref modeldconn.ModelRef, cfg transport.Config) (transport.ModelInfo, error) {
	return modeldconn.Describe(ctx, ref, cfg)
}

func addRoutesWithProvider(mux *http.ServeMux, provider modeldProvider) {
	h := &handler{provider: provider}
	h.register(mux)
}

func addRoutesForTest(mux *http.ServeMux, provider modeldProvider, state stateReader) {
	h := &handler{provider: provider, state: state}
	h.register(mux)
}

func (h *handler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /modeld/status", h.status)
	mux.HandleFunc("POST /modeld/load", h.load)
	mux.HandleFunc("POST /modeld/unload", h.unload)
	mux.HandleFunc("GET /modeld/models", h.models)
	mux.HandleFunc("GET /modeld/capacity", h.capacity)
}

type handler struct {
	provider modeldProvider
	state    stateReader
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
	Adapters   []AdapterInfo `json:"adapters,omitempty" openapi_include_type:"modeldapi.AdapterInfo"`
	Config     RuntimeConfig `json:"config"`
	Generation uint64        `json:"generation" example:"3"`
}

type AdapterInfo struct {
	Name   string  `json:"name,omitempty" example:"style"`
	Digest string  `json:"digest,omitempty" example:"sha256:abcdef"`
	Scale  float32 `json:"scale,omitempty" example:"1"`
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

type UnloadRequest struct {
	ExpectedGeneration *uint64 `json:"expectedGeneration" example:"3"`
}

type LoadRequest struct {
	Model              string  `json:"model" example:"llama:qwen3-8b"`
	ExpectedGeneration *uint64 `json:"expectedGeneration,omitempty" example:"3"`
}

type LoadResponse struct {
	Loaded             bool        `json:"loaded" example:"true"`
	ExpectedGeneration uint64      `json:"expectedGeneration,omitempty" example:"3"`
	Active             ActiveModel `json:"active" openapi_include_type:"modeldapi.ActiveModel"`
}

type UnloadResponse struct {
	Unloaded           bool   `json:"unloaded" example:"true"`
	ExpectedGeneration uint64 `json:"expectedGeneration" example:"3"`
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

func (h *handler) load(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[LoadRequest](r) // @request modeldapi.LoadRequest
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: %v", apiframework.ErrBadRequest, err), apiframework.ExecuteOperation)
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		_ = apiframework.Error(w, r, apiframework.MissingParameter("model"), apiframework.ExecuteOperation)
		return
	}
	if h.state == nil {
		_ = apiframework.Error(w, r, apiframework.NotFound("local model registry is not available"), apiframework.ExecuteOperation)
		return
	}
	resolved, err := h.resolveLocalModel(r.Context(), req.Model)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ExecuteOperation)
		return
	}

	var expected uint64
	if req.ExpectedGeneration != nil {
		expected = *req.ExpectedGeneration
	}
	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	active, err := h.provider.LoadModel(ctx, resolved.Ref, resolved.Config, expected)
	if err != nil {
		_ = apiframework.Error(w, r, modeldAPIError(err), apiframework.ExecuteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, LoadResponse{
		Loaded:             true,
		ExpectedGeneration: expected,
		Active:             sanitizeActiveModel(active),
	}) // @response modeldapi.LoadResponse
}

func (h *handler) unload(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[UnloadRequest](r) // @request modeldapi.UnloadRequest
	if err != nil {
		_ = apiframework.Error(w, r, fmt.Errorf("%w: %v", apiframework.ErrBadRequest, err), apiframework.ExecuteOperation)
		return
	}
	if req.ExpectedGeneration == nil {
		_ = apiframework.Error(w, r, apiframework.MissingParameter("expectedGeneration"), apiframework.ExecuteOperation)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	if err := h.provider.UnloadModel(ctx, *req.ExpectedGeneration); err != nil {
		_ = apiframework.Error(w, r, modeldAPIError(err), apiframework.ExecuteOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, UnloadResponse{
		Unloaded:           true,
		ExpectedGeneration: *req.ExpectedGeneration,
	}) // @response modeldapi.UnloadResponse
}

func (h *handler) models(w http.ResponseWriter, r *http.Request) {
	if h.state == nil {
		_ = apiframework.Encode(w, r, http.StatusOK, []LocalModel{}) // @response []modeldapi.LocalModel
		return
	}
	models, err := h.listLocalModels(r.Context())
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, models) // @response []modeldapi.LocalModel
}

func (h *handler) capacity(w http.ResponseWriter, r *http.Request) {
	name := apiframework.GetQueryParam(r, "model", "", "Registered local model name or id to describe.")
	if name == "" {
		_ = apiframework.Error(w, r, apiframework.MissingParameter("model"), apiframework.GetOperation)
		return
	}
	if h.state == nil {
		_ = apiframework.Error(w, r, apiframework.NotFound("local model registry is not available"), apiframework.GetOperation)
		return
	}

	resolved, err := h.resolveLocalModel(r.Context(), name)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), controlTimeout)
	defer cancel()
	info, err := h.provider.Describe(ctx, resolved.Ref, resolved.Config)
	if err != nil {
		_ = apiframework.Error(w, r, modeldAPIError(err), apiframework.GetOperation)
		return
	}
	model := resolved.Model
	switch {
	case info.PlannerEffectiveContext > 0:
		model.ContextLength = info.PlannerEffectiveContext
	case info.EffectiveContext > 0:
		model.ContextLength = info.EffectiveContext
	}
	_ = apiframework.Encode(w, r, http.StatusOK, CapacityResponse{
		Model: model,
		Info:  capacityInfoFromTransport(info),
	}) // @response modeldapi.CapacityResponse
}

func modeldAPIError(err error) error {
	switch {
	case errors.Is(err, transport.ErrSlotGenerationStale), errors.Is(err, transport.ErrStaleFence), errors.Is(err, transport.ErrModelBusy):
		return apiframework.Conflict(err.Error())
	case errors.Is(err, transport.ErrBackendMismatch), errors.Is(err, transport.ErrUnsupportedFeature):
		return apiframework.BadRequest(err.Error())
	case errors.Is(err, transport.ErrInsufficientMemory), errors.Is(err, transport.ErrModelLoadFailed):
		return apiframework.UnprocessableEntity(err.Error())
	default:
		return err
	}
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
		active := sanitizeActiveModel(*status.Active)
		out.Active = &active
	}
	return out
}

func sanitizeActiveModel(active transport.ActiveModel) ActiveModel {
	return ActiveModel{
		ModelName:  active.ModelName,
		Type:       active.Type,
		Digest:     active.Digest,
		Adapters:   sanitizeAdapters(active.Adapters),
		Config:     sanitizeConfig(active.Config),
		Generation: active.Generation,
	}
}

func sanitizeAdapters(adapters []transport.AdapterSpec) []AdapterInfo {
	if len(adapters) == 0 {
		return nil
	}
	out := make([]AdapterInfo, 0, len(adapters))
	for _, adapter := range adapters {
		out = append(out, AdapterInfo{
			Name:   adapter.Name,
			Digest: adapter.Digest,
			Scale:  adapter.Scale,
		})
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
