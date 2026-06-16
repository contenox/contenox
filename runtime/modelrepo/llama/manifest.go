package llama

import (
	"encoding/json"
	"runtime/debug"

	"github.com/contenox/runtime/runtime/modelrepo/contextasm"
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

func runtimeDigest(cfg Config) string {
	cfg = normalizeConfig(cfg)
	type runtimeIdentity struct {
		NumCtx       int       `json:"num_ctx"`
		NumBatch     int       `json:"num_batch"`
		NumThreads   int       `json:"num_threads"`
		NumGpuLayers int       `json:"num_gpu_layers"`
		TensorSplit  []float32 `json:"tensor_split,omitempty"`
		FlashAttn    bool      `json:"flash_attention"`
		KVCacheType  string    `json:"kv_cache_type,omitempty"`
	}
	b, _ := json.Marshal(runtimeIdentity{
		NumCtx:       cfg.NumCtx,
		NumBatch:     cfg.NumBatch,
		NumThreads:   cfg.NumThreads,
		NumGpuLayers: cfg.NumGpuLayers,
		TensorSplit:  cfg.TensorSplit,
		FlashAttn:    cfg.FlashAttn,
		KVCacheType:  cfg.KVCacheType,
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
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/ollama/ollama" {
			if dep.Replace != nil {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	return ""
}
