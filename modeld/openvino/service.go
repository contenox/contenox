package openvino

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/runtime/modeld/capacity"
	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/modeld/residency"
	"github.com/contenox/runtime/runtime/transport"
)

// openvinoEvictionBlock aligns the derived cache-eviction sizes to OpenVINO's KV
// block granularity.
const openvinoEvictionBlock = 32

// buildTokenizersPath is the build-time fallback path to libopenvino_tokenizers.so
// (set via build-modeld -ldflags -X) for an in-place dev build whose libs live in
// the venv. OpenVINO GenAI loads that extension via OPENVINO_TOKENIZERS_PATH_GENAI.
var buildTokenizersPath string

// buildGenAIVersion is the pinned OpenVINO GenAI version this backend was built
// against, injected at link time (empty for a plain `go build`). It lets
// `modeld version` cross-check that a packaged binary matches its bundle manifest.
var buildGenAIVersion string

// BuildGenAIVersion returns the pinned OpenVINO GenAI version. Cheap and
// side-effect free, so `modeld version` can report it without loading native libs.
func BuildGenAIVersion() string { return buildGenAIVersion }

// tokenizersLibName is the extension file the bundle/venv provides.
const tokenizersLibName = "libopenvino_tokenizers.so"

// init points OpenVINO GenAI at the tokenizers extension without requiring the
// caller to set OPENVINO_TOKENIZERS_PATH_GENAI. It prefers a bundle next to the
// binary and falls back to the build-time venv path for local development.
func init() {
	if os.Getenv("OPENVINO_TOKENIZERS_PATH_GENAI") != "" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "modeld-libs", tokenizersLibName)
		if _, err := os.Stat(cand); err == nil {
			_ = os.Setenv("OPENVINO_TOKENIZERS_PATH_GENAI", cand)
			return
		}
	}
	if buildTokenizersPath != "" {
		_ = os.Setenv("OPENVINO_TOKENIZERS_PATH_GENAI", buildTokenizersPath)
	}
}

// Service implements the runtime/transport.Service boundary for the OpenVINO
// GenAI backend. It opens persistent, manifest-keyed sessions on the owned
// device (CPU / GPU / NPU); the runtime reaches it as a client over the
// transport and never imports this package.
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

func (s *Service) memorySource(device string) capacity.MemorySource {
	if s.memory != nil {
		return s.memory
	}
	device = effectiveDevice(device)
	if openvinoDeviceUsesSystemRAM(device) {
		return capacity.SystemRAM{}
	}
	return openvinoDeviceMemorySource{device: device}
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
		return capacity.Policy{}, fmt.Errorf("openvino host capacity memory probe: %w", err)
	}
	return capacity.WithHostColdDefaults(policy, host), nil
}

var _ transport.Service = (*Service)(nil)

// OpenSession makes the model at req.Path (an OpenVINO IR directory, resolved by
// the runtime) resident and returns a session bound to it. It rejects a model
// typed for a different backend (ErrBackendMismatch) before loading, so a request
// for a llama model on an openvino-mode daemon fails at the boundary. In a build
// without the openvino + openvino_genai tags, ovsession.NewGenAI reports the
// backend is not compiled in and that error surfaces here unchanged.
func (s *Service) OpenSession(_ context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if req.Type != "" && req.Type != "openvino" {
		return nil, fmt.Errorf("%w: requested %q, this daemon serves openvino", transport.ErrBackendMismatch, req.Type)
	}
	cfg, err := s.resolveConfig(req)
	if err != nil {
		return nil, err
	}
	// The OpenVINO-specific tuning (KV precision, sparse attention, cache size) is
	// model-driven: read from the model's own contenox-openvino.json profile, not
	// hardcoded. transport.Config carries only the neutral context window; the
	// device (incl. NPU) is resolved from the environment.
	genaiCfg := genAIConfigFromProfile(req.Path, resolveDevice())
	// Enforce the residency policy with OpenVINO's native sink+recent+evictable
	// cache eviction (the declarative parallel to the llama slide). The budget is
	// derived from the served window; tiny windows stay un-evicted.
	hotContext := cfg.HotContextTokens
	if hotContext <= 0 {
		hotContext = cfg.NumCtx
	}
	budget := residency.DeriveEvictionBudget(hotContext, openvinoEvictionBlock)
	eviction := budget.Valid()
	if eviction {
		on := true
		genaiCfg.UseCacheEviction = &on
		genaiCfg.CacheEvictStartSize = budget.SinkTokens
		genaiCfg.CacheEvictRecentSize = budget.RecentTokens
		genaiCfg.CacheEvictMaxSize = budget.MaxTokens
	}
	backend, err := ovsession.NewGenAI(req.Path, genaiCfg)
	if err != nil {
		return nil, err
	}
	return newGenaiSessionWithPlanner(backend, cfg.NumCtx, cfg.PlannerEffectiveContext, eviction), nil
}

// Describe reports the model's trained context window read from the IR's
// config.json (max_position_embeddings) — no pipeline load. The runtime consumes
// this as the model's capacity; it never reads the IR files itself.
func (s *Service) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	if req.Type != "" && req.Type != "openvino" {
		return transport.ModelInfo{}, fmt.Errorf("%w: requested %q, this daemon serves openvino", transport.ErrBackendMismatch, req.Type)
	}
	info, err := s.describe(req)
	if err != nil {
		return transport.ModelInfo{}, err
	}
	return info, nil
}

// Embed runs a one-shot OpenVINO GenAI TextEmbeddingPipeline for req.Text. It is
// deliberately separate from OpenSession: embedding models do not use the chat
// session's prefix/suffix/Decode lifecycle.
func (s *Service) Embed(ctx context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	if req.Type != "" && req.Type != "openvino" {
		return transport.EmbedResult{}, fmt.Errorf("%w: requested %q, this daemon serves openvino", transport.ErrBackendMismatch, req.Type)
	}
	backend, err := newEmbedSession(req.Path, resolveDevice())
	if err != nil {
		return transport.EmbedResult{}, err
	}
	defer backend.Close()
	vec, err := backend.Embed(ctx, req.Text)
	if err != nil {
		return transport.EmbedResult{}, err
	}
	return transport.EmbedResult{Vector: vec}, nil
}

type openvinoParams struct {
	MaxPositionEmbeddings int `json:"max_position_embeddings"`
	NumHiddenLayers       int `json:"num_hidden_layers"`
	NumKeyValueHeads      int `json:"num_key_value_heads"`
	NumAttentionHeads     int `json:"num_attention_heads"`
	HiddenSize            int `json:"hidden_size"`
	HeadDim               int `json:"head_dim"`
}

func (p openvinoParams) kvHeads() int {
	if p.NumKeyValueHeads > 0 {
		return p.NumKeyValueHeads
	}
	return p.NumAttentionHeads
}

func (p openvinoParams) headDim() int {
	if p.HeadDim > 0 {
		return p.HeadDim
	}
	if p.HiddenSize > 0 && p.NumAttentionHeads > 0 {
		return p.HiddenSize / p.NumAttentionHeads
	}
	return 0
}

// openvinoModelParams reads model architecture facts from config.json. Returns
// zero values when absent/unreadable.
func openvinoModelParams(modelDir string) openvinoParams {
	b, err := os.ReadFile(filepath.Join(modelDir, "config.json"))
	if err != nil {
		return openvinoParams{}
	}
	var cfg openvinoParams
	if json.Unmarshal(b, &cfg) != nil {
		return openvinoParams{}
	}
	return cfg
}

func (s *Service) resolveConfig(req transport.OpenSessionRequest) (transport.Config, error) {
	info, err := s.describe(req)
	if err != nil {
		return transport.Config{}, err
	}
	cfg := req.Config
	if info.EffectiveContext <= 0 {
		return cfg, nil
	}
	if cfg.NumCtx <= 0 {
		cfg.NumCtx = info.HotContextTokens
		cfg.HotContextTokens = info.HotContextTokens
		cfg.PlannerEffectiveContext = info.PlannerEffectiveContext
		return cfg, nil
	}
	if cfg.NumCtx > info.EffectiveContext {
		return transport.Config{}, fmt.Errorf("%w: requested num_ctx=%d exceeds modeld effective context=%d (%s)",
			transport.ErrContextOverflow, cfg.NumCtx, info.EffectiveContext, info.Reason)
	}
	cfg.HotContextTokens = info.HotContextTokens
	cfg.PlannerEffectiveContext = info.PlannerEffectiveContext
	return cfg, nil
}

func (s *Service) describe(req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	params := openvinoModelParams(req.Path)
	device := resolveDevice()
	st, err := capacity.Snapshot(s.memorySource(device))
	if err != nil {
		return transport.ModelInfo{}, fmt.Errorf("openvino capacity memory probe: %w", err)
	}
	policy, err := s.resolvePolicy(st)
	if err != nil {
		return transport.ModelInfo{}, err
	}
	genai := genAIConfigFromProfile(req.Path, device)
	kvBytes := capacity.KVBytesPerToken(params.NumHiddenLayers, params.kvHeads(), params.headDim(), genai.KVCachePrecision)
	resolved := capacity.Resolve(capacity.Params{
		ModelMaxCtx:         params.MaxPositionEmbeddings,
		KVBytesPerToken:     kvBytes,
		WeightsBytes:        dirSize(req.Path),
		FreeBytes:           st.FreeBytes,
		UserLimitBytes:      policy.MaxResidentBytes,
		MinFreeBytes:        policy.MinFreeBytes,
		HostColdBudgetBytes: policy.HostColdBudgetBytes,
		Request:             req.Config.NumCtx,
		HeadroomFrac:        policy.HeadroomFrac,
	})
	return modelInfo(resolved, st), nil
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func modelInfo(c capacity.ModelCapacity, st capacity.DeviceSnapshot) transport.ModelInfo {
	info := openvinoRuntimeInfo()
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
	return info
}

func openvinoRuntimeInfo() transport.ModelInfo {
	info := transport.ModelInfo{RuntimeName: "OpenVINO GenAI"}
	rt, err := ovsession.Runtime()
	if err != nil {
		return info
	}
	info.RuntimeName = rt.RuntimeName
	info.RuntimeDigest = rt.RuntimeDigest
	info.RuntimeSystemInfo = rt.RuntimeSystemInfo
	info.SupportsGPUOffload = rt.SupportsGPUOffload
	info.Devices = make([]transport.DeviceInfo, 0, len(rt.Devices))
	for _, d := range rt.Devices {
		info.Devices = append(info.Devices, transport.DeviceInfo{
			Index:       d.Index,
			Name:        d.Name,
			Description: d.Description,
			Type:        d.Type,
			MemoryFree:  int64(d.MemoryFree),
			MemoryTotal: int64(d.MemoryTotal),
		})
	}
	return info
}

// RuntimeInfo reports the linked OpenVINO runtime identity and device inventory.
// In builds without the native OpenVINO GenAI tags, this returns a minimal
// record with RuntimeName set.
func RuntimeInfo() transport.ModelInfo {
	return openvinoRuntimeInfo()
}

type openvinoDeviceMemorySource struct {
	device string
}

func (s openvinoDeviceMemorySource) FreeBytes() (int64, error) {
	st, err := s.Snapshot()
	if err != nil {
		return 0, err
	}
	return st.FreeBytes, nil
}

func (s openvinoDeviceMemorySource) Snapshot() (capacity.DeviceSnapshot, error) {
	d, err := ovsession.Device(s.device)
	if err != nil {
		return capacity.DeviceSnapshot{}, err
	}
	if d.MemoryFree == 0 || d.MemoryTotal == 0 {
		return capacity.DeviceSnapshot{}, fmt.Errorf("OpenVINO device %q reported no memory telemetry; set CONTENOX_OPENVINO_DEVICE=CPU or use a plugin exposing device memory", s.device)
	}
	kind := d.Type
	if kind == "" {
		kind = openvinoDeviceKind(d.Name)
	}
	return capacity.DeviceSnapshot{
		Kind:              kind,
		DeviceID:          d.Name,
		TotalBytes:        int64(d.MemoryTotal),
		FreeBytes:         int64(d.MemoryFree),
		SharedWithDisplay: d.SharedWithDisplay,
	}, nil
}

func openvinoDeviceUsesSystemRAM(device string) bool {
	base := openvinoDeviceBase(device)
	return base == "" || base == "CPU"
}

// HasAccelerator reports whether OpenVINO enumerates a non-CPU device on this
// host. modeld uses it for runtime backend selection.
func HasAccelerator() bool {
	info, err := ovsession.Runtime()
	if err != nil {
		return false
	}
	for _, d := range info.Devices {
		if !openvinoDeviceUsesSystemRAM(d.Name) {
			return true
		}
	}
	return false
}

func openvinoDeviceKind(device string) string {
	switch openvinoDeviceBase(device) {
	case "GPU":
		return "gpu"
	case "NPU":
		return "accel"
	case "CPU":
		return "cpu"
	default:
		return "unknown"
	}
}

func openvinoDeviceBase(device string) string {
	device = strings.ToUpper(strings.TrimSpace(device))
	if i := strings.IndexByte(device, '.'); i >= 0 {
		device = device[:i]
	}
	if i := strings.IndexByte(device, ':'); i >= 0 {
		device = device[:i]
	}
	return device
}

// resolveDevice selects the OpenVINO inference device. CONTENOX_OPENVINO_DEVICE
// is the explicit override (set it to CPU/GPU/NPU to pin a device); the test
// device hint and an AUTO default follow. AUTO is OpenVINO's virtual plugin that
// places work on the best available device and falls back to CPU.
func resolveDevice() string {
	if device := os.Getenv("CONTENOX_OPENVINO_DEVICE"); device != "" {
		return device
	}
	if device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE"); device != "" {
		return device
	}
	return "AUTO"
}

// effectiveDevice resolves a device selector to the concrete device that
// capacity planning should budget against. AUTO does not expose memory
// telemetry, so mirror its priority (GPU, then NPU, then CPU) against the
// enumerated devices and budget against the device inference will actually land
// on. Concrete selectors pass through unchanged; if enumeration fails we fall
// back to CPU so capacity uses system RAM rather than failing the request.
func effectiveDevice(device string) string {
	if openvinoDeviceBase(device) != "AUTO" {
		return device
	}
	info, err := ovsession.Runtime()
	if err != nil {
		return "CPU"
	}
	var gpu, npu string
	for _, d := range info.Devices {
		switch openvinoDeviceBase(d.Name) {
		case "GPU":
			if gpu == "" {
				gpu = d.Name
			}
		case "NPU":
			if npu == "" {
				npu = d.Name
			}
		}
	}
	switch {
	case gpu != "":
		return gpu
	case npu != "":
		return npu
	default:
		return "CPU"
	}
}
