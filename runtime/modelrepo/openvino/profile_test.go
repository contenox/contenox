package openvino

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

func TestUnit_OpenVINOProfile_DefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()

	profile, err := loadModelProfile(dir)
	require.NoError(t, err)
	require.Equal(t, "f16", profile.GenAI.KVCachePrecision)
	require.Equal(t, 1, profile.GenAI.CacheSize)
	require.True(t, *profile.GenAI.DynamicSplitFuse)
	require.True(t, *profile.GenAI.EnablePrefixCaching)
	require.True(t, *profile.GenAI.UseSparseAttention)
	require.Equal(t, 10, profile.GenAI.NumLastDenseTokensInPrefill)
	require.Equal(t, float32(0.9), profile.GenAI.XAttentionThreshold)
	require.Equal(t, 128, profile.GenAI.XAttentionBlockSize)
	require.Equal(t, 16, profile.GenAI.XAttentionStride)
	require.Equal(t, ovsession.GenAIAvailable, profile.capabilityConfig().CanChat)
	require.Equal(t, ovsession.GenAIAvailable, profile.capabilityConfig().CanPrompt)
	require.Equal(t, ovsession.GenAIAvailable, profile.capabilityConfig().CanStream)
}

func TestUnit_OpenVINOProfile_LoadsStrictJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{
		"context_length": 65536,
		"max_output_tokens": 512,
		"can_think": true,
		"device": "CPU",
		"tool_calls": {"protocol": "openvino:llama3_json_tool_parser"},
		"reasoning": {"protocol": "openvino:deepseek_r1_reasoning_parser"},
		"genai": {
			"kv_cache_precision": "f32",
			"cache_size": 2,
			"dynamic_split_fuse": false,
			"enable_prefix_caching": true,
			"use_sparse_attention": false,
			"num_last_dense_tokens_in_prefill": 12,
			"xattention_threshold": 0.8,
			"xattention_block_size": 64,
			"xattention_stride": 8
		}
	}`), 0644))

	profile, err := loadModelProfile(dir)
	require.NoError(t, err)
	require.Equal(t, 65536, profile.ContextLength)
	require.Equal(t, 512, profile.MaxOutputTokens)
	require.True(t, profile.CanThink)
	require.Equal(t, "CPU", profile.Device)
	require.Equal(t, "openvino:llama3_json_tool_parser", profile.ToolCalls.Protocol)
	require.Equal(t, "openvino:deepseek_r1_reasoning_parser", profile.Reasoning.Protocol)
	require.Equal(t, "f32", profile.GenAI.KVCachePrecision)
	require.Equal(t, 2, profile.GenAI.CacheSize)
	require.False(t, *profile.GenAI.DynamicSplitFuse)
	require.True(t, *profile.GenAI.EnablePrefixCaching)
	require.False(t, *profile.GenAI.UseSparseAttention)
	require.Equal(t, 12, profile.GenAI.NumLastDenseTokensInPrefill)
	require.Equal(t, float32(0.8), profile.GenAI.XAttentionThreshold)
	require.Equal(t, 64, profile.GenAI.XAttentionBlockSize)
	require.Equal(t, 8, profile.GenAI.XAttentionStride)
}

func TestUnit_OpenVINOProfile_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{"genai":{"cache_siz":1}}`), 0644))

	_, err := loadModelProfile(dir)
	require.ErrorContains(t, err, "unknown field")
}

func TestUnit_OpenVINOProfile_RejectsUnsupportedProtocols(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, profileFileName), []byte(`{
		"tool_calls": {"protocol": "openvino:regex"}
	}`), 0644))

	_, err := loadModelProfile(dir)
	require.ErrorContains(t, err, `tool_calls.protocol: unsupported protocol "openvino:regex"`)
}
