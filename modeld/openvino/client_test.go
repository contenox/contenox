package openvino

import (
	"testing"

	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/contextasm"
	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/stretchr/testify/require"
)

func TestUnit_OpenVINOClassifyChatContext_IgnoresVolatileUserTurn(t *testing.T) {
	base := []modeld.Message{
		{Role: "system", Content: "You are local."},
		{Role: "user", Content: "turn one"},
	}
	next := []modeld.Message{
		{Role: "system", Content: "You are local."},
		{Role: "user", Content: "turn two"},
	}
	changedStable := []modeld.Message{
		{Role: "system", Content: "You are local and strict."},
		{Role: "user", Content: "turn two"},
	}

	h1 := classifyChatContext(base, `[{"type":"function","function":{"name":"fs.read"}}]`).StablePrefixHash
	h2 := classifyChatContext(next, `[{"type":"function","function":{"name":"fs.read"}}]`).StablePrefixHash
	h3 := classifyChatContext(changedStable, `[{"type":"function","function":{"name":"fs.read"}}]`).StablePrefixHash
	h4 := classifyChatContext(base, `[{"type":"function","function":{"name":"fs.write"}}]`).StablePrefixHash

	require.Equal(t, h1, h2)
	require.NotEqual(t, h1, h3)
	require.NotEqual(t, h1, h4)
}

func TestUnit_OpenVINOClassifyChatContext_KeepsLateSystemVolatile(t *testing.T) {
	plan := classifyChatContext([]modeld.Message{
		{Role: "system", Content: "stable"},
		{Role: "user", Content: "first turn"},
		{Role: "system", Content: "late instruction"},
		{Role: "assistant", Content: "reply"},
	}, "")

	require.Len(t, plan.Messages, 4)
	require.Equal(t, "stable", plan.Messages[0].Content)
	require.Equal(t, "first turn", plan.Messages[1].Content)
	require.Equal(t, "late instruction", plan.Messages[2].Content)
	require.Equal(t, "reply", plan.Messages[3].Content)

	require.Len(t, plan.Segments, 4)
	require.Equal(t, KindSystem, plan.Segments[0].Kind)
	require.Equal(t, KindUserTurn, plan.Segments[1].Kind)
	require.Equal(t, KindUserTurn, plan.Segments[2].Kind)
	require.Equal(t, KindUserTurn, plan.Segments[3].Kind)
}

func TestUnit_OpenVINOCacheTelemetry_IncludesMetricsAndStableHash(t *testing.T) {
	manifest := contextasm.ContextManifest{
		Backend:        "openvino",
		RuntimeDigest:  "runtime",
		StableByteHash: "stable-hash",
	}
	data := genAICacheTelemetry(manifest, ovsession.PipelineMetrics{
		Requests:          3,
		ScheduledRequests: 2,
		CacheUsage:        0.25,
		MaxCacheUsage:     0.50,
		AvgCacheUsage:     0.125,
		InferenceDuration: 42,
		CacheSizeInBytes:  1024,
	}, 12, "prompt-token-hash")

	require.Equal(t, "stable-hash", data["stable_prefix_hash"])
	require.Equal(t, manifest.Digest(), data["manifest_digest"])
	require.Equal(t, uint64(3), data["requests"])
	require.Equal(t, uint64(2), data["scheduled_requests"])
	require.Equal(t, float32(0.25), data["cache_usage"])
	require.Equal(t, float32(0.50), data["max_cache_usage"])
	require.Equal(t, float32(0.125), data["avg_cache_usage"])
	require.Equal(t, float32(42), data["inference_duration"])
	require.Equal(t, uint64(1024), data["cache_size_bytes"])
	require.Equal(t, 12, data["prompt_tokens"])
	require.Equal(t, "prompt-token-hash", data["prompt_token_hash"])
}

func TestUnit_OpenVINORuntimeDigest_ChangesWithSchedulerConfig(t *testing.T) {
	a := openvinoRuntimeDigest(ovsession.GenAIConfig{Device: "CPU"})
	b := openvinoRuntimeDigest(ovsession.GenAIConfig{Device: "GPU"})
	require.NotEmpty(t, a)
	require.NotEqual(t, a, b)
}

func TestUnit_OpenVINOManifestReuseCompatibility_GatesRuntimeChanges(t *testing.T) {
	client := &genAIClient{}
	base := contextasm.ContextManifest{
		Backend:              "openvino",
		ModelDigest:          "model",
		PromptFormat:         "openvino_chat_template",
		PromptTemplateDigest: "template",
		RuntimeDigest:        "runtime-a",
		StableByteHash:       "stable-a",
	}
	client.rememberManifest(base)

	next := base
	next.StableByteHash = "stable-b"
	ok, reason := client.manifestReuseCompatibility(next)
	require.True(t, ok)
	require.Empty(t, reason)

	next = base
	next.RuntimeDigest = "runtime-b"
	ok, reason = client.manifestReuseCompatibility(next)
	require.False(t, ok)
	require.Contains(t, reason, "runtime_digest")
}
