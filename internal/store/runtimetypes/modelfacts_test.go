package runtimetypes_test

import (
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

func TestUnit_ModelFacts_SetGetResolve(t *testing.T) {
	ctx, st, exec := runtimetypes.SetupStoreExec(t)
	now := time.Now().UTC()
	_, err := exec.ExecContext(ctx, `
		INSERT INTO llm_backends (id, name, base_url, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, "b1", "local", "http://127.0.0.1:11434", "ollama", now, now)
	require.NoError(t, err)

	empty, err := st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Empty(t, empty, "a provider that states nothing returns an empty map, never an invented one")

	facts := map[string]runtimetypes.ModelFacts{
		"qwen3:8b": {
			ContextLength:   32768,
			MaxOutputTokens: 8192,
			Capabilities:    []string{runtimetypes.CapabilityCompletion, runtimetypes.CapabilityTools},
			Pricing: &runtimetypes.ModelPricing{
				InputPerMillion:  0.10,
				OutputPerMillion: 0.40,
			},
		},
	}
	require.NoError(t, st.SetLLMProviderModelFacts(ctx, "b1", facts))

	got, err := st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Equal(t, 32768, got["qwen3:8b"].ContextLength)
	require.Equal(t, []string{"completion", "tools"}, got["qwen3:8b"].Capabilities)
	require.InDelta(t, 0.40, got["qwen3:8b"].Pricing.OutputPerMillion, 1e-9)

	require.NoError(t, st.SetLLMProviderModelFact(ctx, "b1", "llama3.1:8b", runtimetypes.ModelFacts{
		MaxOutputTokens: 4096,
	}))
	got, err = st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Len(t, got, 2, "one model's facts leave the provider's other models declared")
	require.Equal(t, 8192, got["qwen3:8b"].MaxOutputTokens)
	require.Equal(t, 4096, got["llama3.1:8b"].MaxOutputTokens)

	require.NoError(t, st.DeleteLLMProviderModelFact(ctx, "b1", "llama3.1:8b"))
	got, err = st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.NoError(t, st.SetLLMProviderModelFact(ctx, "b1", "qwen3:8b", runtimetypes.ModelFacts{}))
	got, err = st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Empty(t, got, "facts that state nothing remove the entry, and the last one removes the row")

	require.NoError(t, st.SetLLMProviderModelFacts(ctx, "b1", facts))
	require.NoError(t, st.DeleteLLMProviderModelFacts(ctx, "b1"))
	got, err = st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Empty(t, got)

	require.NoError(t, st.SetLLMProviderModelFacts(ctx, "b1", facts))
	require.NoError(t, st.SetLLMProviderModelFacts(ctx, "b1", nil), "an empty map clears the declaration")
	got, err = st.GetLLMProviderModelFacts(ctx, "b1")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestUnit_ModelFacts_RejectsUnreadableDeclarations(t *testing.T) {
	ctx, st, _ := runtimetypes.SetupStoreExec(t)

	err := st.SetLLMProviderModelFacts(ctx, "b1", map[string]runtimetypes.ModelFacts{
		"m": {ContextLength: -1},
	})
	require.Error(t, err, "a negative context window is not a declaration any reader could honour")

	err = st.SetLLMProviderModelFacts(ctx, "b1", map[string]runtimetypes.ModelFacts{
		"m": {Capabilities: []string{"telepathy"}},
	})
	require.Error(t, err, "a capability outside the vocabulary would be silently ignored by every reader")

	err = st.SetLLMProviderModelFacts(ctx, "b1", map[string]runtimetypes.ModelFacts{
		"m": {Pricing: &runtimetypes.ModelPricing{OutputPerMillion: -0.1}},
	})
	require.Error(t, err, "negative pricing rates are refused")
}

func TestUnit_ModelPricing_CalculateCost(t *testing.T) {
	p := runtimetypes.ModelPricing{
		InputPerMillion:      1.00,
		CacheReadPerMillion:  0.10,
		CacheWritePerMillion: 1.25,
		OutputPerMillion:     2.00,
		PerImage:             0.01,
		PerAudioMebibyte:     0.50,
	}
	got := p.CalculateCost(1_000_000, 400_000, 100_000, 500_000, 0, 0)
	require.InDelta(t, 0.60+0.04+0.125+1.00, got, 1e-9)

	withImages := p.CalculateCost(1_000_000, 400_000, 100_000, 500_000, 3, 0)
	require.InDelta(t, 0.60+0.04+0.125+1.00+0.03, withImages, 1e-9,
		"an image is charged whether or not the provider counted its tokens")

	withAudio := p.CalculateCost(0, 0, 0, 0, 0, 2*runtimetypes.Mebibyte)
	require.InDelta(t, 1.00, withAudio, 1e-9, "audio is charged by the mebibyte of inline bytes")
}
