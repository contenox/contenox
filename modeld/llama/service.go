package llama

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/contenox/runtime/modeld/capacity"
	"github.com/contenox/runtime/runtime/transport"
)

// Service implements the runtime/transport.Service boundary.
// It acts as the opener for native llama.cpp backend sessions.
type Service struct {
	memory     capacity.MemorySource
	hostMemory capacity.MemorySource
	policy     capacity.Policy
}

type ServiceOption func(*Service)

func NewService(opts ...ServiceOption) *Service {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithCapacityPolicy(p capacity.Policy) ServiceOption {
	return func(s *Service) { s.policy = p }
}

func WithMemorySource(src capacity.MemorySource) ServiceOption {
	return func(s *Service) { s.memory = src }
}

func WithHostMemorySource(src capacity.MemorySource) ServiceOption {
	return func(s *Service) { s.hostMemory = src }
}

var _ transport.Service = (*Service)(nil)

// OpenSession binds a session to the requested model. It rejects a model typed
// for a different backend (ErrBackendMismatch) before loading, so a GGUF request
// sent to an openvino-mode daemon — or vice versa — fails at the boundary, not
// deep in the engine. The model is loaded from req.Path (resolved by the
// runtime); identity/caching uses req.Digest.
func (s *Service) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if req.Type != "" && req.Type != "llama" {
		return nil, fmt.Errorf("%w: requested %q, this daemon serves llama", transport.ErrBackendMismatch, req.Type)
	}
	plan, err := s.resolveSession(req)
	if err != nil {
		return nil, err
	}
	cfg := plan.config
	info := plan.info
	slog.Info("llama session config",
		"num_ctx", cfg.NumCtx,
		"hot_context_tokens", info.HotContextTokens,
		"planner_effective_context", cfg.PlannerEffectiveContext,
		"host_cold_budget_bytes", info.HostColdBudgetBytes,
		"num_batch", cfg.NumBatch,
		"num_gpu_layers", cfg.NumGpuLayers,
		"requested_gpu_layers", info.RequestedGpuLayers,
		"resolved_gpu_layers", info.ResolvedGpuLayers,
		"free_bytes", info.FreeBytes,
		"user_limit_bytes", info.UserLimitBytes,
		"usable_bytes", info.UsableBytes,
		"weights_bytes", info.WeightsBytes,
		"overhead_bytes", info.OverheadBytes,
		"required_bytes", info.RequiredBytes,
		"capacity_reason", info.Reason,
		"flash_attention", cfg.FlashAttn,
		"kv_cache_type", cfg.KVCacheType,
		"sparse_attention", info.SparseAttention,
		"sliding_window_attention_tokens", info.SlidingWindowAttentionTokens,
	)
	return newSession(req.Path, cfg, toAdapterSpecs(req.Adapters))
}

// toAdapterSpecs maps the transport adapter handles onto the backend-local
// AdapterSpec the session factory applies. The two types are kept distinct so the
// CGo session package never imports the wire shape; they carry the same fields.
func toAdapterSpecs(in []transport.AdapterSpec) []AdapterSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]AdapterSpec, len(in))
	for i, a := range in {
		out[i] = AdapterSpec{Name: a.Name, Path: a.Path, Digest: a.Digest, Scale: a.Scale}
	}
	return out
}

// Describe reports the model's trained context window read from the GGUF header
// (no tensor load). The runtime consumes this as the model's capacity; it never
// reads the GGUF itself.
func (s *Service) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	if req.Type != "" && req.Type != "llama" {
		return transport.ModelInfo{}, fmt.Errorf("%w: requested %q, this daemon serves llama", transport.ErrBackendMismatch, req.Type)
	}
	info, err := s.describe(req)
	if err != nil {
		return transport.ModelInfo{}, err
	}
	return info, nil
}

// Embed is not served through the modeld transport for llama yet. The runtime's
// llama provider still owns its native one-shot embedding path separately.
func (s *Service) Embed(_ context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	if req.Type != "" && req.Type != "llama" {
		return transport.EmbedResult{}, fmt.Errorf("%w: requested %q, this daemon serves llama", transport.ErrBackendMismatch, req.Type)
	}
	return transport.EmbedResult{}, fmt.Errorf("%w: llama embeddings are not served over modeld transport", transport.ErrUnsupportedFeature)
}

func (s *Service) resolveConfig(req transport.OpenSessionRequest) (transport.Config, error) {
	plan, err := s.resolveSession(req)
	if err != nil {
		return transport.Config{}, err
	}
	return plan.config, nil
}

func (s *Service) resolveSession(req transport.OpenSessionRequest) (sessionPlan, error) {
	plan, err := s.plan(req)
	if err != nil {
		return sessionPlan{}, err
	}
	cfg := plan.config
	info := plan.info
	if info.EffectiveContext <= 0 && info.Reason != "" {
		return sessionPlan{}, fmt.Errorf("%w: model %q cannot fit in the selected %s memory budget (%s)",
			transport.ErrContextOverflow, req.ModelName, info.DeviceKind, info.Reason)
	}
	// Fail only when an accelerator is present but cannot fit even one layer.
	// With no accelerator (e.g. a universal binary on a CPU-only host) GPU layers
	// resolve to 0 by design and the session runs on CPU instead of erroring.
	if info.RequestedGpuLayers > 0 && info.ResolvedGpuLayers <= 0 && isAcceleratorSnapshot(capacity.DeviceSnapshot{Kind: info.DeviceKind}) {
		return sessionPlan{}, fmt.Errorf("%w: requested gpu_layers=%d but no layer fits in the selected %s memory budget (%s)",
			transport.ErrContextOverflow, info.RequestedGpuLayers, info.DeviceKind, info.Reason)
	}
	if cfg.NumCtx <= 0 {
		cfg.NumCtx = info.HotContextTokens
		cfg.HotContextTokens = info.HotContextTokens
		cfg.PlannerEffectiveContext = transport.ResolvePlannerEffectiveContext(cfg.PlannerEffectiveContext, cfg.NumCtx, info)
		plan.config = cfg
		return plan, nil
	}
	if cfg.NumCtx > info.EffectiveContext {
		return sessionPlan{}, fmt.Errorf("%w: requested num_ctx=%d exceeds modeld effective context=%d (%s)",
			transport.ErrContextOverflow, cfg.NumCtx, info.EffectiveContext, info.Reason)
	}
	cfg.HotContextTokens = info.HotContextTokens
	cfg.PlannerEffectiveContext = transport.ResolvePlannerEffectiveContext(cfg.PlannerEffectiveContext, cfg.NumCtx, info)
	plan.config = cfg
	return plan, nil
}

func applyDaemonEnvOverrides(cfg transport.Config) transport.Config {
	if v := os.Getenv("CONTENOX_LLAMA_GPU_LAYERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NumGpuLayers = n
		}
	}
	if v := os.Getenv("CONTENOX_LLAMA_CTX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.NumCtx = n
		}
	}
	return cfg
}

var defaultMemorySource = func(transport.Config) capacity.MemorySource {
	return capacity.SystemRAM{}
}

var llamaRuntimeInfo = func() transport.ModelInfo {
	return transport.ModelInfo{}
}

// RuntimeInfo reports the linked llama.cpp runtime identity and device
// inventory. In non-direct builds this returns an empty record.
func RuntimeInfo() transport.ModelInfo {
	return llamaRuntimeInfo()
}

// Set by Makefile builds so Describe can report the exact pinned llama.cpp
// source used for the direct runtime.
var llamaCPPCommit string

// BuildCommit returns the pinned llama.cpp source commit this backend was built
// against, as injected at link time. It is empty for a plain `go build` with no
// -ldflags. Cheap and side-effect free, so `modeld version` can report it without
// loading native libraries.
func BuildCommit() string { return llamaCPPCommit }

func (s *Service) memorySource(cfg transport.Config) capacity.MemorySource {
	if s.memory != nil {
		return s.memory
	}
	return defaultMemorySource(cfg)
}

func (s *Service) hostMemorySource() capacity.MemorySource {
	if s.hostMemory != nil {
		return s.hostMemory
	}
	return capacity.SystemRAM{}
}

func (s *Service) resolvePolicy(st capacity.DeviceSnapshot) (capacity.Policy, error) {
	policy := capacity.WithResidentDefault(s.policy, st)
	host, err := capacity.Snapshot(s.hostMemorySource())
	if err != nil {
		return capacity.Policy{}, fmt.Errorf("llama host capacity memory probe: %w", err)
	}
	return capacity.WithHostColdDefaults(policy, host), nil
}

func (s *Service) describe(req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	cfg := applyDaemonEnvOverrides(req.Config)
	params := ggufModelParams(req.Path)
	st, err := capacity.Snapshot(s.memorySource(cfg))
	if err != nil {
		return transport.ModelInfo{}, fmt.Errorf("llama capacity memory probe: %w", err)
	}
	policy, err := s.resolvePolicy(st)
	if err != nil {
		return transport.ModelInfo{}, err
	}
	weights := fileSize(req.Path)
	kvBytes := capacity.KVBytesPerToken(params.BlockCount, params.kvHeads(), params.headDim(), cfg.KVCacheType)
	overhead := int64(0)
	// modeld derives GPU offload from what it detected at runtime, not from a
	// per-model knob: with an accelerator present it offloads as many layers as
	// fit the VRAM budget; with none it runs on CPU (the CUDA plugin was silently
	// skipped). An explicit cfg.NumGpuLayers (model profile or
	// CONTENOX_LLAMA_GPU_LAYERS) is honored only as an upper cap.
	explicitGpuLayers := cfg.NumGpuLayers
	resolvedGpuLayers := 0
	if isAcceleratorSnapshot(st) {
		cfg.NumGpuLayers = autoGpuLayerCeiling(explicitGpuLayers)
		overhead = llamaGPUComputeReserveBytes(cfg)
		resolvedGpuLayers = resolveGPULayersForBudget(cfg, params, weights, kvBytes, overhead, st, policy)
		cfg.NumGpuLayers = resolvedGpuLayers
		weights = estimateLlamaGPUWeights(weights, params.BlockCount, cfg.NumGpuLayers)
	} else {
		cfg.NumGpuLayers = 0
	}
	resolved := capacity.Resolve(capacity.Params{
		ModelMaxCtx:         params.ContextLength,
		KVBytesPerToken:     kvBytes,
		WeightsBytes:        weights,
		OverheadBytes:       overhead,
		FreeBytes:           st.FreeBytes,
		UserLimitBytes:      policy.MaxResidentBytes,
		MinFreeBytes:        policy.MinFreeBytes,
		HostColdBudgetBytes: policy.HostColdBudgetBytes,
		Request:             cfg.NumCtx,
		HeadroomFrac:        policy.HeadroomFrac,
	})
	info := modelInfo(resolved, st)
	if params.SlidingWindow > 0 {
		info.SparseAttention = true
		info.SlidingWindowAttentionTokens = params.SlidingWindow
	}
	info.RequestedGpuLayers = explicitGpuLayers
	info.ResolvedGpuLayers = resolvedGpuLayers
	if explicitGpuLayers > 0 && resolvedGpuLayers < explicitGpuLayers {
		info.Clamped = true
		if info.Reason == "" {
			if isAcceleratorSnapshot(st) {
				info.Reason = "gpu_layers_exceed_memory_budget"
			} else {
				info.Reason = "no_accelerator_present"
			}
		}
	}
	return info, nil
}

func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return info.Size()
	}
	return 0
}

func modelInfo(c capacity.ModelCapacity, st capacity.DeviceSnapshot) transport.ModelInfo {
	info := llamaRuntimeInfo()
	info.ModelMaxContext = c.ModelMaxContext
	info.EffectiveContext = c.EffectiveContext
	info.MemoryContextTokens = c.MemoryContextTokens
	info.HotContextTokens = c.HotContextTokens
	info.PlannerEffectiveContext = c.PlannerEffectiveContext
	info.KVBytesPerToken = c.KVBytesPerToken
	info.FreeBytes = c.FreeBytes
	info.WeightsBytes = c.WeightsBytes
	info.OverheadBytes = c.OverheadBytes
	info.ReservedBytes = c.ReservedBytes
	info.UserLimitBytes = c.UserLimitBytes
	info.MinFreeBytes = c.MinFreeBytes
	info.HostColdBudgetBytes = c.HostColdBudgetBytes
	info.UsableBytes = c.UsableBytes
	info.RequiredBytes = c.RequiredBytes
	info.Clamped = c.Clamped
	info.Reason = c.Reason
	info.DeviceKind = st.Kind
	info.DeviceID = st.DeviceID
	info.DeviceTotalBytes = st.TotalBytes
	info.SharedWithDisplay = st.SharedWithDisplay
	if info.RuntimeDigest == "" {
		info.RuntimeDigest = llamaCPPCommit
	}
	return info
}

type sessionPlan struct {
	config transport.Config
	info   transport.ModelInfo
}

func (s *Service) plan(req transport.OpenSessionRequest) (sessionPlan, error) {
	cfg := applyDaemonEnvOverrides(req.Config)
	info, err := s.describe(transport.OpenSessionRequest{
		Fence:     req.Fence,
		ModelName: req.ModelName,
		Type:      req.Type,
		Digest:    req.Digest,
		Path:      req.Path,
		Config:    cfg,
	})
	if err != nil {
		return sessionPlan{}, err
	}
	// describe computes the authoritative offload count (0 on CPU / no accelerator,
	// the VRAM-fitted count on a detected accelerator), so it always wins over the
	// incoming request value.
	cfg.NumGpuLayers = info.ResolvedGpuLayers
	return sessionPlan{config: cfg, info: info}, nil
}

const defaultLlamaGPUComputeReserveBytes int64 = 768 << 20

func llamaGPUComputeReserveBytes(cfg transport.Config) int64 {
	if v, err := capacity.ParseBytes(os.Getenv("CONTENOX_LLAMA_GPU_COMPUTE_RESERVE")); err == nil && v > 0 {
		return v
	}
	batch := cfg.NumBatch
	if batch <= 0 {
		batch = 512
	}
	reserve := defaultLlamaGPUComputeReserveBytes * int64(batch) / 512
	if reserve < 256<<20 {
		return 256 << 20
	}
	return reserve
}

// allGpuLayers is the conventional llama.cpp "offload every layer" sentinel.
// resolveGPULayersForBudget caps it to the model's real layer count, so it just
// means "as many as fit"; it is large enough to exceed any real model's depth.
const allGpuLayers = 999

// autoGpuLayerCeiling is the offload ceiling modeld aims for once an accelerator
// is detected: an explicit cap when the caller set one (model profile or
// CONTENOX_LLAMA_GPU_LAYERS), otherwise all layers. resolveGPULayersForBudget
// then lowers it to what actually fits VRAM.
func autoGpuLayerCeiling(explicit int) int {
	if explicit > 0 {
		return explicit
	}
	return allGpuLayers
}

func resolveGPULayersForBudget(cfg transport.Config, params ggufParams, weights, kvBytes, overhead int64, st capacity.DeviceSnapshot, policy capacity.Policy) int {
	if cfg.NumGpuLayers <= 0 {
		return 0
	}
	if params.BlockCount <= 0 || weights <= 0 || kvBytes <= 0 {
		return cfg.NumGpuLayers
	}
	maxSlots := params.BlockCount
	if cfg.NumGpuLayers > params.BlockCount {
		maxSlots = params.BlockCount + 1 // output layer
	}
	requestedSlots := min(cfg.NumGpuLayers, maxSlots)
	for slots := requestedSlots; slots >= 1; slots-- {
		modelBytes := estimateLlamaGPUWeights(weights, params.BlockCount, slots)
		resolved := capacity.Resolve(capacity.Params{
			ModelMaxCtx:         params.ContextLength,
			KVBytesPerToken:     kvBytes,
			WeightsBytes:        modelBytes,
			OverheadBytes:       overhead,
			FreeBytes:           st.FreeBytes,
			UserLimitBytes:      policy.MaxResidentBytes,
			MinFreeBytes:        policy.MinFreeBytes,
			HostColdBudgetBytes: policy.HostColdBudgetBytes,
			Request:             cfg.NumCtx,
			HeadroomFrac:        policy.HeadroomFrac,
		})
		if resolved.EffectiveContext <= 0 {
			continue
		}
		if cfg.NumCtx > 0 && resolved.EffectiveContext < cfg.NumCtx {
			continue
		}
		return slots
	}
	return 0
}

func estimateLlamaGPUWeights(weights int64, blockCount, gpuLayers int) int64 {
	if weights <= 0 || gpuLayers <= 0 {
		return 0
	}
	if blockCount <= 0 {
		return weights
	}
	maxSlots := blockCount + 1 // repeating layers plus output layer
	slots := min(gpuLayers, maxSlots)
	perSlot := weights / int64(maxSlots)
	if perSlot <= 0 {
		return weights
	}
	est := perSlot * int64(slots)
	if est > weights {
		return weights
	}
	return est
}

func isAcceleratorSnapshot(st capacity.DeviceSnapshot) bool {
	switch strings.ToLower(strings.TrimSpace(st.Kind)) {
	case "gpu", "igpu", "accel":
		return true
	default:
		return false
	}
}
