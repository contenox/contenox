package llama

import (
	"encoding/json"

	"github.com/contenox/runtime/runtime/contextasm"
)

const backendName = "llamacpp"

type ManifestSegment = contextasm.ManifestSegment
type ContextManifest = contextasm.ContextManifest
type TokenizeFunc = contextasm.TokenizeFunc
type ManifestMismatchError = contextasm.ManifestMismatchError

var ErrManifestMismatch = contextasm.ErrManifestMismatch

func NewManifestMismatchError(reason string) error {
	return contextasm.NewManifestMismatchError(reason)
}

func runtimeDigest(cfg Config, adapters []AdapterSpec) string {
	cfg = normalizeConfig(cfg)
	type adapterIdentity struct {
		Digest string  `json:"digest,omitempty"`
		Scale  float32 `json:"scale,omitempty"`
	}
	type runtimeIdentity struct {
		NumCtx                  int               `json:"num_ctx"`
		PlannerEffectiveContext int               `json:"planner_effective_context,omitempty"`
		NumBatch                int               `json:"num_batch"`
		NumThreads              int               `json:"num_threads"`
		NumGpuLayers            int               `json:"num_gpu_layers"`
		TensorSplit             []float32         `json:"tensor_split,omitempty"`
		FlashAttn               bool              `json:"flash_attention"`
		KVCacheType             string            `json:"kv_cache_type,omitempty"`
		Reasoning               string            `json:"reasoning,omitempty"`
		Adapters                []adapterIdentity `json:"adapters,omitempty"`
	}
	// Adapter digests and scales in list order make a variant a distinct manifest
	// from its base — warm KV for base+A must not satisfy base+B's manifest gate.
	var ids []adapterIdentity
	for _, a := range adapters {
		ids = append(ids, adapterIdentity{Digest: a.Digest, Scale: a.Scale})
	}
	b, _ := json.Marshal(runtimeIdentity{
		NumCtx:                  cfg.NumCtx,
		PlannerEffectiveContext: cfg.PlannerEffectiveContext,
		NumBatch:                cfg.NumBatch,
		NumThreads:              cfg.NumThreads,
		NumGpuLayers:            cfg.NumGpuLayers,
		TensorSplit:             cfg.TensorSplit,
		FlashAttn:               cfg.FlashAttn,
		KVCacheType:             cfg.KVCacheType,
		Reasoning:               cfg.ReasoningFormat,
		Adapters:                ids,
	})
	return hashBytes(b)
}

func hashString(s string) string {
	return contextasm.HashString(s)
}

func hashBytes(b []byte) string {
	return contextasm.HashBytes(b)
}

func HashTokenIDs(tokens []int) string {
	return contextasm.HashTokenIDs(tokens)
}

func normalizeConfig(cfg Config) Config {
	if cfg.NumCtx <= 0 {
		cfg.NumCtx = 8192
	}
	if cfg.NumBatch <= 0 {
		cfg.NumBatch = 512
	}
	if cfg.PromptFormat == "" {
		cfg.PromptFormat = promptFormatChatML
	}
	if cfg.PromptTemplateDigest == "" {
		cfg.PromptTemplateDigest = promptTemplateDigest(cfg.PromptFormat)
	}
	return cfg
}

func backendVersion() string {
	return "llamacpp-direct"
}
