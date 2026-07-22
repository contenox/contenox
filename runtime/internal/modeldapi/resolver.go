package modeldapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/transport"
)

const (
	llamaProfileFileName    = "contenox-llama.json"
	openvinoProfileFileName = "contenox-openvino.json"
)

var openvinoEntrypointNames = []string{
	"openvino_model.xml",
	"openvino_language_model.xml",
}

// LocalModel is the browser-safe identity of a modeld-servable local model.
// It deliberately omits the backend BaseURL and resolved daemon filesystem path.
type LocalModel struct {
	ID              string `json:"id" example:"llama:qwen3-8b"`
	Model           string `json:"model" example:"qwen3-8b"`
	Name            string `json:"name,omitempty" example:"Qwen3 8B"`
	BackendID       string `json:"backendId,omitempty" example:"b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e"`
	BackendName     string `json:"backendName,omitempty" example:"llama"`
	BackendType     string `json:"backendType" example:"llama"`
	Digest          string `json:"digest,omitempty" example:"sha256:abcdef"`
	ContextLength   int    `json:"contextLength,omitempty" example:"8192"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty" example:"4096"`
	CanChat         bool   `json:"canChat"`
	CanEmbed        bool   `json:"canEmbed"`
	CanPrompt       bool   `json:"canPrompt"`
	CanStream       bool   `json:"canStream"`
	CanThink        bool   `json:"canThink,omitempty"`
	CanVision       bool   `json:"canVision,omitempty"`
}

type CapacityResponse struct {
	Model LocalModel   `json:"model" openapi_include_type:"modeldapi.LocalModel"`
	Info  CapacityInfo `json:"info" openapi_include_type:"modeldapi.CapacityInfo"`
}

type CapacityInfo struct {
	ModelMaxContext                     int              `json:"modelMaxContext"`
	EffectiveContext                    int              `json:"effectiveContext"`
	MemoryContextTokens                 int              `json:"memoryContextTokens,omitempty"`
	HotContextTokens                    int              `json:"hotContextTokens,omitempty"`
	PlannerEffectiveContext             int              `json:"plannerEffectiveContext,omitempty"`
	KVBytesPerToken                     int64            `json:"kvBytesPerToken,omitempty"`
	FreeBytes                           int64            `json:"freeBytes,omitempty"`
	WeightsBytes                        int64            `json:"weightsBytes,omitempty"`
	OverheadBytes                       int64            `json:"overheadBytes,omitempty"`
	ReservedBytes                       int64            `json:"reservedBytes,omitempty"`
	UserLimitBytes                      int64            `json:"userLimitBytes,omitempty"`
	MinFreeBytes                        int64            `json:"minFreeBytes,omitempty"`
	HostColdBudgetBytes                 int64            `json:"hostColdBudgetBytes,omitempty"`
	UsableBytes                         int64            `json:"usableBytes,omitempty"`
	RequiredBytes                       int64            `json:"requiredBytes,omitempty"`
	Clamped                             bool             `json:"clamped,omitempty"`
	Reason                              string           `json:"reason,omitempty"`
	DeviceKind                          string           `json:"deviceKind,omitempty"`
	DeviceID                            string           `json:"deviceId,omitempty"`
	DeviceTotalBytes                    int64            `json:"deviceTotalBytes,omitempty"`
	SharedWithDisplay                   bool             `json:"sharedWithDisplay,omitempty"`
	RequestedGpuLayers                  int              `json:"requestedGpuLayers,omitempty"`
	ResolvedGpuLayers                   int              `json:"resolvedGpuLayers,omitempty"`
	SparseAttention                     bool             `json:"sparseAttention,omitempty"`
	SlidingWindowAttentionTokens        int              `json:"slidingWindowAttentionTokens,omitempty"`
	ChatTemplateFormat                  string           `json:"chatTemplateFormat,omitempty"`
	ChatTemplateThinkingStartTag        string           `json:"chatTemplateThinkingStartTag,omitempty"`
	ChatTemplateReasoningFormat         string           `json:"chatTemplateReasoningFormat,omitempty"`
	ChatTemplateSupportsToolCalls       bool             `json:"chatTemplateSupportsToolCalls,omitempty"`
	ChatTemplateSupportsThinking        bool             `json:"chatTemplateSupportsThinking,omitempty"`
	ChatTemplateSupportsReasoningEffort bool             `json:"chatTemplateSupportsReasoningEffort,omitempty"`
	SupportsVision                      bool             `json:"supportsVision,omitempty"`
	RuntimeName                         string           `json:"runtimeName,omitempty"`
	RuntimeDigest                       string           `json:"runtimeDigest,omitempty"`
	RuntimeSystemInfo                   string           `json:"runtimeSystemInfo,omitempty"`
	SupportsGPUOffload                  bool             `json:"supportsGpuOffload,omitempty"`
	Devices                             []CapacityDevice `json:"devices,omitempty" openapi_include_type:"modeldapi.CapacityDevice"`
}

type CapacityDevice struct {
	Index            int    `json:"index"`
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Type             string `json:"type,omitempty"`
	MemoryFree       int64  `json:"memoryFree,omitempty"`
	MemoryTotal      int64  `json:"memoryTotal,omitempty"`
	MemoryFreeKnown  bool   `json:"memoryFreeKnown,omitempty"`
	MemoryTotalKnown bool   `json:"memoryTotalKnown,omitempty"`
}

type resolvedLocalModel struct {
	Model  LocalModel
	Ref    modeldconn.ModelRef
	Config transport.Config
}

type localCandidate struct {
	model   LocalModel
	state   statetype.BackendRuntimeState
	pulled  statetype.ModelPullStatus
	backend string
}

func (h *handler) listLocalModels(ctx context.Context) ([]LocalModel, error) {
	candidates, err := h.localCandidates(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]LocalModel, 0, len(candidates))
	for _, candidate := range candidates {
		models = append(models, candidate.model)
	}
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].BackendType != models[j].BackendType {
			return models[i].BackendType < models[j].BackendType
		}
		return models[i].Model < models[j].Model
	})
	return models, nil
}

func (h *handler) resolveLocalModel(ctx context.Context, query string) (resolvedLocalModel, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return resolvedLocalModel{}, apiframework.MissingParameter("model")
	}

	candidates, err := h.localCandidates(ctx)
	if err != nil {
		return resolvedLocalModel{}, err
	}

	var nameMatches []localCandidate
	for _, candidate := range candidates {
		if query == candidate.model.ID {
			return h.resolveCandidate(candidate)
		}
		if query == candidate.model.Model || (candidate.model.Name != "" && query == candidate.model.Name) {
			nameMatches = append(nameMatches, candidate)
		}
	}

	switch len(nameMatches) {
	case 0:
		return resolvedLocalModel{}, apiframework.NotFound(fmt.Sprintf("local model %q was not found", query))
	case 1:
		return h.resolveCandidate(nameMatches[0])
	default:
		ids := make([]string, 0, len(nameMatches))
		for _, match := range nameMatches {
			ids = append(ids, match.model.ID)
		}
		sort.Strings(ids)
		return resolvedLocalModel{}, apiframework.Conflict(fmt.Sprintf("local model %q is ambiguous; use one of: %s", query, strings.Join(ids, ", ")))
	}
}

func (h *handler) localCandidates(ctx context.Context) ([]localCandidate, error) {
	states, err := h.state.Get(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []localCandidate
	for _, state := range states {
		backend := modelrepo.CanonicalBackendType(state.Backend.Type)
		if backend != "llama" && backend != "openvino" {
			continue
		}
		for _, pulled := range state.PulledModels {
			name := pulledModelName(pulled)
			if name == "" {
				continue
			}
			model := LocalModel{
				ID:              backend + ":" + name,
				Model:           name,
				Name:            strings.TrimSpace(pulled.Name),
				BackendID:       state.Backend.ID,
				BackendName:     state.Backend.Name,
				BackendType:     backend,
				Digest:          pulled.Digest,
				ContextLength:   pulled.ContextLength,
				MaxOutputTokens: pulled.MaxOutputTokens,
				CanChat:         pulled.CanChat,
				CanEmbed:        pulled.CanEmbed,
				CanPrompt:       pulled.CanPrompt,
				CanStream:       pulled.CanStream,
				CanThink:        pulled.CanThink,
				CanVision:       pulled.CanVision,
			}
			candidates = append(candidates, localCandidate{
				model:   model,
				state:   state,
				pulled:  pulled,
				backend: backend,
			})
		}
	}
	return candidates, nil
}

func (h *handler) resolveCandidate(candidate localCandidate) (resolvedLocalModel, error) {
	root := strings.TrimSpace(candidate.state.Backend.BaseURL)
	if root == "" {
		return resolvedLocalModel{}, apiframework.BadRequest(fmt.Sprintf("local backend %q has no model directory", candidate.state.Backend.Name))
	}
	switch candidate.backend {
	case "llama":
		ref, cfg, digest, err := resolveLlamaModel(root, candidate.model.Model, candidate.pulled)
		if err != nil {
			return resolvedLocalModel{}, err
		}
		candidate.model.Digest = firstNonEmptyString(candidate.model.Digest, digest)
		return resolvedLocalModel{Model: candidate.model, Ref: ref, Config: cfg}, nil
	case "openvino":
		ref, cfg, digest, err := resolveOpenVINOModel(root, candidate.model.Model, candidate.pulled)
		if err != nil {
			return resolvedLocalModel{}, err
		}
		candidate.model.Digest = firstNonEmptyString(candidate.model.Digest, digest)
		return resolvedLocalModel{Model: candidate.model, Ref: ref, Config: cfg}, nil
	default:
		return resolvedLocalModel{}, apiframework.BadRequest(fmt.Sprintf("unsupported local backend %q", candidate.backend))
	}
}

func pulledModelName(model statetype.ModelPullStatus) string {
	name := strings.TrimSpace(model.Model)
	if name == "" {
		name = strings.TrimSpace(model.Name)
	}
	return name
}

func safeModelDir(root, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" || model == "." || model == ".." || strings.ContainsAny(model, `/\`) {
		return "", apiframework.BadRequest("invalid local model name")
	}
	return filepath.Join(root, model), nil
}

type llamaProfile struct {
	ModelDigest   string           `json:"model_digest,omitempty"`
	ContextLength int              `json:"context_length,omitempty"`
	Adapters      []adapterProfile `json:"adapters,omitempty"`
	Prompt        struct {
		Format         string `json:"format,omitempty"`
		TemplateDigest string `json:"template_digest,omitempty"`
		AddBOS         *bool  `json:"add_bos,omitempty"`
	} `json:"prompt,omitempty"`
	Runtime struct {
		NumCtx         int       `json:"num_ctx,omitempty"`
		NumBatch       int       `json:"num_batch,omitempty"`
		NumThreads     int       `json:"num_threads,omitempty"`
		NumGpuLayers   int       `json:"num_gpu_layers,omitempty"`
		TensorSplit    []float32 `json:"tensor_split,omitempty"`
		FlashAttention bool      `json:"flash_attention,omitempty"`
		KVCacheType    string    `json:"kv_cache_type,omitempty"`
	} `json:"runtime,omitempty"`
	Reasoning struct {
		Format string `json:"format,omitempty"`
	} `json:"reasoning,omitempty"`
}

type adapterProfile struct {
	Name   string   `json:"name,omitempty"`
	Path   string   `json:"path,omitempty"`
	Digest string   `json:"digest,omitempty"`
	Scale  *float32 `json:"scale,omitempty"`
}

func resolveLlamaModel(root, name string, pulled statetype.ModelPullStatus) (modeldconn.ModelRef, transport.Config, string, error) {
	dir, err := safeModelDir(root, name)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	modelPath := filepath.Join(dir, "model.gguf")
	if _, err := os.Stat(modelPath); err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", apiframework.NotFound(fmt.Sprintf("llama model %q is not available", name))
	}

	var profile llamaProfile
	if err := decodeOptionalJSON(filepath.Join(dir, llamaProfileFileName), &profile); err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	cfg := llamaTransportConfig(profile, pulled)
	digest := strings.TrimSpace(profile.ModelDigest)
	if digest == "" {
		digest, err = fileSHA256(modelPath)
		if err != nil {
			return modeldconn.ModelRef{}, transport.Config{}, "", err
		}
	}
	adapters, err := resolveAdapterProfiles(dir, profile.Adapters)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	return modeldconn.ModelRef{Name: name, Type: "llama", Digest: digest, Path: modelPath, Adapters: adapters}, cfg, digest, nil
}

func llamaTransportConfig(profile llamaProfile, pulled statetype.ModelPullStatus) transport.Config {
	cfg := transport.Config{
		NumCtx:               profile.Runtime.NumCtx,
		NumBatch:             profile.Runtime.NumBatch,
		NumThreads:           profile.Runtime.NumThreads,
		NumGpuLayers:         profile.Runtime.NumGpuLayers,
		TensorSplit:          profile.Runtime.TensorSplit,
		FlashAttn:            profile.Runtime.FlashAttention,
		KVCacheType:          profile.Runtime.KVCacheType,
		PromptFormat:         profile.Prompt.Format,
		PromptTemplateDigest: profile.Prompt.TemplateDigest,
		ReasoningFormat:      profile.Reasoning.Format,
	}
	if profile.Prompt.AddBOS != nil {
		cfg.DisableBOS = !*profile.Prompt.AddBOS
	}
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
	// No fallback when nothing explicit is set: NumCtx=0 means "auto" and lets
	// modeld resolve the window fresh from live memory for both LoadModel and
	// Describe. A concrete placeholder here (the old firstPositive(..., 8192))
	// silently converted every auto request into a static-ctx request, keeping
	// modeld's dynamic capacity path permanently disengaged.
	if cfg.NumBatch <= 0 {
		cfg.NumBatch = 512
	}
	return cfg
}

type openvinoProfile struct {
	ContextLength int              `json:"context_length,omitempty"`
	Adapters      []adapterProfile `json:"adapters,omitempty"`
}

func resolveOpenVINOModel(root, name string, pulled statetype.ModelPullStatus) (modeldconn.ModelRef, transport.Config, string, error) {
	dir, err := safeModelDir(root, name)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	if !hasOpenVINOEntrypoint(dir) {
		return modeldconn.ModelRef{}, transport.Config{}, "", apiframework.NotFound(fmt.Sprintf("OpenVINO model %q is not available", name))
	}

	var profile openvinoProfile
	if err := decodeOptionalJSON(filepath.Join(dir, openvinoProfileFileName), &profile); err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	digest, templateDigest := openvinoIdentity(dir)
	adapters, err := resolveAdapterProfiles(dir, profile.Adapters)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, "", err
	}
	// NumCtx stays 0 (auto) unless the model profile declares an explicit
	// context: modeld resolves the window fresh from live memory, and derives
	// the trained ceiling from the model files itself.
	cfg := transport.Config{
		NumCtx:               profile.ContextLength,
		PromptFormat:         "openvino-chat-template",
		PromptTemplateDigest: templateDigest,
	}
	return modeldconn.ModelRef{Name: name, Type: "openvino", Digest: digest, Path: dir, Adapters: adapters}, cfg, digest, nil
}

func resolveAdapterProfiles(profileDir string, adapters []adapterProfile) ([]transport.AdapterSpec, error) {
	if len(adapters) == 0 {
		return nil, nil
	}
	out := make([]transport.AdapterSpec, 0, len(adapters))
	for i, adapter := range adapters {
		path := strings.TrimSpace(adapter.Path)
		if path == "" {
			return nil, apiframework.BadRequest(fmt.Sprintf("local model profile adapter[%d] is missing path", i))
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(profileDir, path)
		}
		digest := strings.TrimSpace(adapter.Digest)
		if digest == "" {
			var err error
			digest, err = fileSHA256(path)
			if err != nil {
				return nil, err
			}
		}
		scale := float32(1)
		if adapter.Scale != nil {
			scale = *adapter.Scale
		}
		out = append(out, transport.AdapterSpec{
			Name:   strings.TrimSpace(adapter.Name),
			Path:   path,
			Digest: digest,
			Scale:  scale,
		})
	}
	return out, nil
}

func hasOpenVINOEntrypoint(dir string) bool {
	for _, name := range openvinoEntrypointNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func openvinoIdentity(modelDir string) (modelDigest, templateDigest string) {
	h := sha256.New()
	for _, name := range []string{"config.json", "tokenizer_config.json", "generation_config.json"} {
		if b, err := os.ReadFile(filepath.Join(modelDir, name)); err == nil {
			h.Write(b)
		}
	}
	modelDigest = hex.EncodeToString(h.Sum(nil))

	if b, err := os.ReadFile(filepath.Join(modelDir, "tokenizer_config.json")); err == nil {
		var cfg struct {
			ChatTemplate json.RawMessage `json:"chat_template"`
		}
		if json.Unmarshal(b, &cfg) == nil && len(cfg.ChatTemplate) > 0 {
			templateDigest = contextasm.HashString(string(cfg.ChatTemplate))
		}
	}
	return modelDigest, templateDigest
}

func decodeOptionalJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return apiframework.BadRequest(fmt.Sprintf("could not read local model profile %s", filepath.Base(path)))
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(dst); err != nil {
		return apiframework.BadRequest(fmt.Sprintf("invalid local model profile %s: %v", filepath.Base(path), err))
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", apiframework.InternalServerError("could not hash local model")
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", apiframework.InternalServerError("could not hash local model")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func capacityInfoFromTransport(info transport.ModelInfo) CapacityInfo {
	devices := make([]CapacityDevice, 0, len(info.Devices))
	for _, device := range info.Devices {
		devices = append(devices, CapacityDevice{
			Index:            device.Index,
			Name:             device.Name,
			Description:      device.Description,
			Type:             device.Type,
			MemoryFree:       device.MemoryFree,
			MemoryTotal:      device.MemoryTotal,
			MemoryFreeKnown:  device.MemoryFreeKnown,
			MemoryTotalKnown: device.MemoryTotalKnown,
		})
	}
	return CapacityInfo{
		ModelMaxContext:                     info.ModelMaxContext,
		EffectiveContext:                    info.EffectiveContext,
		MemoryContextTokens:                 info.MemoryContextTokens,
		HotContextTokens:                    info.HotContextTokens,
		PlannerEffectiveContext:             info.PlannerEffectiveContext,
		KVBytesPerToken:                     info.KVBytesPerToken,
		FreeBytes:                           info.FreeBytes,
		WeightsBytes:                        info.WeightsBytes,
		OverheadBytes:                       info.OverheadBytes,
		ReservedBytes:                       info.ReservedBytes,
		UserLimitBytes:                      info.UserLimitBytes,
		MinFreeBytes:                        info.MinFreeBytes,
		HostColdBudgetBytes:                 info.HostColdBudgetBytes,
		UsableBytes:                         info.UsableBytes,
		RequiredBytes:                       info.RequiredBytes,
		Clamped:                             info.Clamped,
		Reason:                              info.Reason,
		DeviceKind:                          info.DeviceKind,
		DeviceID:                            info.DeviceID,
		DeviceTotalBytes:                    info.DeviceTotalBytes,
		SharedWithDisplay:                   info.SharedWithDisplay,
		RequestedGpuLayers:                  info.RequestedGpuLayers,
		ResolvedGpuLayers:                   info.ResolvedGpuLayers,
		SparseAttention:                     info.SparseAttention,
		SlidingWindowAttentionTokens:        info.SlidingWindowAttentionTokens,
		ChatTemplateFormat:                  info.ChatTemplateFormat,
		ChatTemplateThinkingStartTag:        info.ChatTemplateThinkingStartTag,
		ChatTemplateReasoningFormat:         info.ChatTemplateReasoningFormat,
		ChatTemplateSupportsToolCalls:       info.ChatTemplateSupportsToolCalls,
		ChatTemplateSupportsThinking:        info.ChatTemplateSupportsThinking,
		ChatTemplateSupportsReasoningEffort: info.ChatTemplateSupportsReasoningEffort,
		SupportsVision:                      info.SupportsVision,
		RuntimeName:                         info.RuntimeName,
		RuntimeDigest:                       info.RuntimeDigest,
		RuntimeSystemInfo:                   info.RuntimeSystemInfo,
		SupportsGPUOffload:                  info.SupportsGPUOffload,
		Devices:                             devices,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
