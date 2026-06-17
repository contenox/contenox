package llama

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const profileFileName = "contenox-llama.json"

// modelProfile is the optional per-model runtime profile (<modelDir>/<name>/
// contenox-llama.json). Absent file = defaults. This is where explicit
// runtime config replaces the toy fixed constants.
type modelProfile struct {
	ProfileID       string         `json:"profile_id,omitempty"`
	ModelDigest     string         `json:"model_digest,omitempty"`
	ContextLength   int            `json:"context_length,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
	CanThink        bool             `json:"can_think,omitempty"`
	Prompt          promptProfile    `json:"prompt,omitempty"`
	Runtime         runtimeProfile   `json:"runtime,omitempty"`
	ToolCalls       toolCallsProfile `json:"tool_calls,omitempty"`
}

// toolCallsProfile declares how this model emits tool calls. The protocol must be
// a registered parser name (e.g. "hermes"); tool calls are rejected when unset, so
// the runtime never guesses a model's tool format.
type toolCallsProfile struct {
	Protocol string `json:"protocol,omitempty"`
}

type promptProfile struct {
	Format         string `json:"format,omitempty"`
	TemplateDigest string `json:"template_digest,omitempty"`
	AddBOS         *bool  `json:"add_bos,omitempty"`
}

type runtimeProfile struct {
	NumCtx         int       `json:"num_ctx,omitempty"`
	NumBatch       int       `json:"num_batch,omitempty"`
	NumThreads     int       `json:"num_threads,omitempty"`
	NumGpuLayers   int       `json:"num_gpu_layers,omitempty"`
	TensorSplit    []float32 `json:"tensor_split,omitempty"`
	FlashAttention bool      `json:"flash_attention,omitempty"`
	KVCacheType    string    `json:"kv_cache_type,omitempty"`
}

func loadModelProfile(profileDir string) (modelProfile, error) {
	path := filepath.Join(profileDir, profileFileName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return modelProfile{}, nil
		}
		return modelProfile{}, fmt.Errorf("llama profile open %s: %w", path, err)
	}
	defer f.Close()
	var p modelProfile
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return modelProfile{}, fmt.Errorf("llama profile decode %s: %w", path, err)
	}
	if _, err := rendererForFormat(p.Prompt.Format, p.Prompt.TemplateDigest); err != nil {
		return modelProfile{}, fmt.Errorf("llama profile prompt %s: %w", path, err)
	}
	if p.ToolCalls.Protocol != "" && !toolCallProtocolKnown(p.ToolCalls.Protocol) {
		return modelProfile{}, fmt.Errorf("llama profile %s: unknown tool_calls.protocol %q", path, p.ToolCalls.Protocol)
	}
	return p, nil
}

// config resolves the runtime Config, applying env overrides (handy for quick
// GPU-bench tuning) and defaults.
func (p modelProfile) config() Config {
	c := Config{
		NumCtx:               p.Runtime.NumCtx,
		NumBatch:             p.Runtime.NumBatch,
		NumThreads:           p.Runtime.NumThreads,
		NumGpuLayers:         p.Runtime.NumGpuLayers,
		TensorSplit:          p.Runtime.TensorSplit,
		FlashAttn:            p.Runtime.FlashAttention,
		KVCacheType:          p.Runtime.KVCacheType,
		PromptFormat:         p.Prompt.Format,
		PromptTemplateDigest: p.Prompt.TemplateDigest,
	}
	if p.Prompt.AddBOS != nil {
		c.DisableBOS = !*p.Prompt.AddBOS
	}
	if v := os.Getenv("CONTENOX_LLAMA_GPU_LAYERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.NumGpuLayers = n
		}
	}
	if v := os.Getenv("CONTENOX_LLAMA_CTX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.NumCtx = n
		}
	}
	return normalizeConfig(c)
}

func (p modelProfile) capabilityConfig() modelrepo.CapabilityConfig {
	cfg := p.config()
	contextLength := p.ContextLength
	if contextLength <= 0 {
		contextLength = cfg.NumCtx
	}
	return modelrepo.CapabilityConfig{
		ContextLength:   contextLength,
		MaxOutputTokens: p.MaxOutputTokens,
		CanChat:         SessionAvailable(),
		CanPrompt:       SessionAvailable(),
		CanStream:       SessionAvailable(),
		CanEmbed:        EmbedAvailable(),
		CanThink:        p.CanThink,
	}
}
