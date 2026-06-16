package openvino

import (
	"encoding/json"

	"github.com/contenox/runtime/runtime/modelrepo/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

func openvinoRuntimeDigest(cfg ovsession.GenAIConfig) string {
	cfg = normalizedPoolConfig(cfg)
	type runtimeIdentity struct {
		Device                      string  `json:"device"`
		KVCachePrecision            string  `json:"kv_cache_precision"`
		CacheSize                   int     `json:"cache_size"`
		DynamicSplitFuse            bool    `json:"dynamic_split_fuse"`
		EnablePrefixCaching         bool    `json:"enable_prefix_caching"`
		UseSparseAttention          bool    `json:"use_sparse_attention"`
		NumLastDenseTokensInPrefill int     `json:"num_last_dense_tokens_in_prefill"`
		XAttentionThreshold         float32 `json:"xattention_threshold"`
		XAttentionBlockSize         int     `json:"xattention_block_size"`
		XAttentionStride            int     `json:"xattention_stride"`
	}
	b, _ := json.Marshal(runtimeIdentity{
		Device:                      cfg.Device,
		KVCachePrecision:            cfg.KVCachePrecision,
		CacheSize:                   cfg.CacheSize,
		DynamicSplitFuse:            boolValue(cfg.DynamicSplitFuse, true),
		EnablePrefixCaching:         boolValue(cfg.EnablePrefixCaching, true),
		UseSparseAttention:          boolValue(cfg.UseSparseAttention, true),
		NumLastDenseTokensInPrefill: cfg.NumLastDenseTokensInPrefill,
		XAttentionThreshold:         cfg.XAttentionThreshold,
		XAttentionBlockSize:         cfg.XAttentionBlockSize,
		XAttentionStride:            cfg.XAttentionStride,
	})
	return contextasm.HashBytes(b)
}
