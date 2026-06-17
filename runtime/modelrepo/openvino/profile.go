package openvino

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

const profileFileName = "contenox-openvino.json"

type modelProfile struct {
	ContextLength   int          `json:"context_length,omitempty"`
	MaxOutputTokens int          `json:"max_output_tokens,omitempty"`
	CanThink        bool         `json:"can_think,omitempty"`
	Device          string       `json:"device,omitempty"`
	GenAI           genAIProfile `json:"genai,omitempty"`
	ToolCalls       protocolRef  `json:"tool_calls,omitempty"`
	Reasoning       protocolRef  `json:"reasoning,omitempty"`
}

type genAIProfile struct {
	KVCachePrecision            string  `json:"kv_cache_precision,omitempty"`
	CacheSize                   int     `json:"cache_size,omitempty"`
	DynamicSplitFuse            *bool   `json:"dynamic_split_fuse,omitempty"`
	EnablePrefixCaching         *bool   `json:"enable_prefix_caching,omitempty"`
	UseSparseAttention          *bool   `json:"use_sparse_attention,omitempty"`
	NumLastDenseTokensInPrefill int     `json:"num_last_dense_tokens_in_prefill,omitempty"`
	XAttentionThreshold         float32 `json:"xattention_threshold,omitempty"`
	XAttentionBlockSize         int     `json:"xattention_block_size,omitempty"`
	XAttentionStride            int     `json:"xattention_stride,omitempty"`
}

type protocolRef struct {
	Protocol string `json:"protocol,omitempty"`
}

func loadModelProfile(modelPath string) (modelProfile, error) {
	path := filepath.Join(modelPath, profileFileName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultModelProfile(), nil
		}
		return modelProfile{}, fmt.Errorf("openvino profile open %s: %w", path, err)
	}
	defer f.Close()

	var profile modelProfile
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&profile); err != nil {
		return modelProfile{}, fmt.Errorf("openvino profile decode %s: %w", path, err)
	}
	if err := profile.validate(path); err != nil {
		return modelProfile{}, err
	}
	return profile.withDefaults(), nil
}

func defaultModelProfile() modelProfile {
	return modelProfile{
		GenAI: genAIProfile{
			KVCachePrecision:            "f16",
			CacheSize:                   1,
			DynamicSplitFuse:            boolPtr(true),
			EnablePrefixCaching:         boolPtr(true),
			UseSparseAttention:          boolPtr(true),
			NumLastDenseTokensInPrefill: 10,
			XAttentionThreshold:         0.9,
			XAttentionBlockSize:         128,
			XAttentionStride:            16,
		},
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func (p modelProfile) withDefaults() modelProfile {
	def := defaultModelProfile()
	if p.GenAI.KVCachePrecision == "" {
		p.GenAI.KVCachePrecision = def.GenAI.KVCachePrecision
	}
	if p.GenAI.CacheSize <= 0 {
		p.GenAI.CacheSize = def.GenAI.CacheSize
	}
	if p.GenAI.DynamicSplitFuse == nil {
		p.GenAI.DynamicSplitFuse = def.GenAI.DynamicSplitFuse
	}
	if p.GenAI.EnablePrefixCaching == nil {
		p.GenAI.EnablePrefixCaching = def.GenAI.EnablePrefixCaching
	}
	if p.GenAI.UseSparseAttention == nil {
		p.GenAI.UseSparseAttention = def.GenAI.UseSparseAttention
	}
	if p.GenAI.NumLastDenseTokensInPrefill <= 0 {
		p.GenAI.NumLastDenseTokensInPrefill = def.GenAI.NumLastDenseTokensInPrefill
	}
	if p.GenAI.XAttentionThreshold <= 0 {
		p.GenAI.XAttentionThreshold = def.GenAI.XAttentionThreshold
	}
	if p.GenAI.XAttentionBlockSize <= 0 {
		p.GenAI.XAttentionBlockSize = def.GenAI.XAttentionBlockSize
	}
	if p.GenAI.XAttentionStride <= 0 {
		p.GenAI.XAttentionStride = def.GenAI.XAttentionStride
	}
	return p
}

func (p modelProfile) validate(path string) error {
	if p.ContextLength < 0 {
		return fmt.Errorf("openvino profile %s: context_length must be non-negative", path)
	}
	if p.MaxOutputTokens < 0 {
		return fmt.Errorf("openvino profile %s: max_output_tokens must be non-negative", path)
	}
	if p.GenAI.CacheSize < 0 {
		return fmt.Errorf("openvino profile %s: genai.cache_size must be non-negative", path)
	}
	if p.GenAI.NumLastDenseTokensInPrefill < 0 {
		return fmt.Errorf("openvino profile %s: genai.num_last_dense_tokens_in_prefill must be non-negative", path)
	}
	if p.GenAI.XAttentionThreshold < 0 {
		return fmt.Errorf("openvino profile %s: genai.xattention_threshold must be non-negative", path)
	}
	if p.GenAI.XAttentionBlockSize < 0 {
		return fmt.Errorf("openvino profile %s: genai.xattention_block_size must be non-negative", path)
	}
	if p.GenAI.XAttentionStride < 0 {
		return fmt.Errorf("openvino profile %s: genai.xattention_stride must be non-negative", path)
	}
	if err := validateToolProtocol(p.ToolCalls.Protocol); err != nil {
		return fmt.Errorf("openvino profile %s: tool_calls.protocol: %w", path, err)
	}
	if err := validateReasoningProtocol(p.Reasoning.Protocol); err != nil {
		return fmt.Errorf("openvino profile %s: reasoning.protocol: %w", path, err)
	}
	return nil
}

func (p modelProfile) capabilityConfig() modelrepo.CapabilityConfig {
	return modelrepo.CapabilityConfig{
		ContextLength:   p.ContextLength,
		MaxOutputTokens: p.MaxOutputTokens,
		CanChat:         ovsession.GenAIAvailable,
		CanPrompt:       ovsession.GenAIAvailable,
		CanStream:       ovsession.GenAIAvailable,
		CanThink:        p.CanThink,
	}
}

func (p modelProfile) sessionConfig(device string) ovsession.GenAIConfig {
	return ovsession.GenAIConfig{
		Device:                      device,
		KVCachePrecision:            p.GenAI.KVCachePrecision,
		CacheSize:                   p.GenAI.CacheSize,
		DynamicSplitFuse:            p.GenAI.DynamicSplitFuse,
		EnablePrefixCaching:         p.GenAI.EnablePrefixCaching,
		UseSparseAttention:          p.GenAI.UseSparseAttention,
		NumLastDenseTokensInPrefill: p.GenAI.NumLastDenseTokensInPrefill,
		XAttentionThreshold:         p.GenAI.XAttentionThreshold,
		XAttentionBlockSize:         p.GenAI.XAttentionBlockSize,
		XAttentionStride:            p.GenAI.XAttentionStride,
	}
}

func mergeCapabilities(base, profile modelrepo.CapabilityConfig) modelrepo.CapabilityConfig {
	if base.ContextLength == 0 {
		base.ContextLength = profile.ContextLength
	}
	if base.MaxOutputTokens == 0 {
		base.MaxOutputTokens = profile.MaxOutputTokens
	}
	base.CanChat = profile.CanChat
	base.CanPrompt = profile.CanPrompt
	base.CanEmbed = false
	base.CanStream = profile.CanStream
	if !base.CanThink {
		base.CanThink = profile.CanThink
	}
	return base
}
