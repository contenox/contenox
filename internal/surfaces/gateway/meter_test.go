package gateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblicense"
	"github.com/stretchr/testify/require"
)

func TestUnit_ChargeTurnMetrics_PopulatesTheMeter(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "h1", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	svc.chargeTurn(ctx, key, nil, turnUsage{model: "qwen3:8b", prompt: 100, completion: 40, durationMs: 900, finishReason: "stop"})
	svc.chargeTurn(ctx, key, nil, turnUsage{model: "qwen3:8b", prompt: 200, completion: 60, durationMs: 900, finishReason: "stop"})

	now := time.Now().UTC()
	snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 300, snapshot.PromptTokens)
	require.EqualValues(t, 100, snapshot.CompletionTokens)
	require.EqualValues(t, 400, snapshot.TotalTokens)

	lifetime, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowTotal,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 400, lifetime.TotalTokens)
}

func TestUnit_AuthorizeTurn_RefusesOnTheMeteredWeek(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "h1", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	claims := liblicense.NewClaims("lic-test", "issuer", "subject")
	claims.SetInt64(liblicense.OutputAllowanceKey("qwen3:8b"), 50)

	require.NoError(t, svc.AuthorizeTurn(ctx, key, &claims, "qwen3:8b"), "nothing metered yet")

	svc.chargeTurn(ctx, key, nil, turnUsage{model: "qwen3:8b", prompt: 100, completion: 40, durationMs: 900, finishReason: "stop"})
	require.NoError(t, svc.AuthorizeTurn(ctx, key, &claims, "qwen3:8b"), "40 output tokens are under the 50 cap")

	svc.chargeTurn(ctx, key, &claims, turnUsage{model: "qwen3:8b", prompt: 10, completion: 30, durationMs: 900, finishReason: "stop"})
	require.ErrorIs(t, svc.AuthorizeTurn(ctx, key, &claims, "qwen3:8b"), ErrAllowanceExhausted, "70 output tokens have crossed the 50 cap")

	other, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "llama3.1:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 0, other.TotalTokens, "a second model has its own meter")
}

func TestUnit_ChargeTurnMetrics_ChargesThePlansCacheDiscount(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "h2", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	planned := liblicense.NewClaims("lic-1", "issuer", "laptop")
	planned.SetInt64(liblicense.CacheDiscountMultiplierKey("qwen3:8b"), 2500)
	svc.chargeTurn(ctx, key, &planned, turnUsage{model: "qwen3:8b", prompt: 1000, completion: 10, cacheRead: 400, durationMs: 900, finishReason: "stop"})

	unplanned := liblicense.NewClaims("lic-2", "issuer", "laptop")
	svc.chargeTurn(ctx, key, &unplanned, turnUsage{model: "llama3.1:8b", prompt: 1000, completion: 10, cacheRead: 400, durationMs: 900, finishReason: "stop"})

	require.InDelta(t, 0.10, liblicense.CacheDiscountMultiplierFor(&unplanned, "llama3.1:8b"), 1e-9)

	now := time.Now().UTC()
	for _, tc := range []struct {
		model string
		want  int64
	}{
		{"qwen3:8b", 700},
		{"llama3.1:8b", 640},
	} {
		snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
			Model: tc.model, WindowKind: runtimetypes.UsageWindowWeek,
		}, now)
		require.NoError(t, err)
		require.EqualValues(t, tc.want, snapshot.EffectiveInput,
			"600 uncached plus 400 cached charged at the plan's rate for %s", tc.model)
	}
}

func TestUnit_ChargeTurnMetrics_ChargesThinkingAgainstOutputAndBurst(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "thinking", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	claims := liblicense.NewClaims("lic-thinking", "issuer", "laptop")
	claims.SetInt64(liblicense.ThinkingDiscountMultiplierKey("reasoner"), 2500)
	claims.SetInt64(liblicense.OutputAllowanceKey("reasoner"), 50)
	claims.SetInt64(liblicense.FiveHourAllowanceKey("reasoner"), 51)
	svc.chargeTurn(ctx, key, &claims, turnUsage{model: "reasoner", prompt: 10, completion: 100, thinking: 80})

	now := time.Now().UTC()
	snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "reasoner", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 100, snapshot.CompletionTokens)
	require.EqualValues(t, 80, snapshot.ThinkingTokens)
	require.EqualValues(t, 40, snapshot.EffectiveOutput)
	require.EqualValues(t, 50, snapshot.EffectiveTokens())
	require.NoError(t, svc.AuthorizeTurn(ctx, key, &claims, "reasoner"))

	claims.SetInt64(liblicense.FiveHourAllowanceKey("reasoner"), 50)
	require.ErrorIs(t, svc.AuthorizeTurn(ctx, key, &claims, "reasoner"), ErrAllowanceExhausted)
}

func TestUnit_ChargeTurnMetrics_PreservesAZeroThinkingMultiplier(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()
	key := &runtimetypes.ProxyKey{
		KeyHash: "free-thinking", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	claims := liblicense.NewClaims("lic-free-thinking", "issuer", "laptop")
	claims.SetInt64(liblicense.ThinkingDiscountMultiplierKey("reasoner"), 0)
	svc.chargeTurn(ctx, key, &claims, turnUsage{model: "reasoner", completion: 80, thinking: 80})

	snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "reasoner", WindowKind: runtimetypes.UsageWindowWeek,
	}, time.Now().UTC())
	require.NoError(t, err)
	require.EqualValues(t, 80, snapshot.CompletionTokens)
	require.EqualValues(t, 80, snapshot.ThinkingTokens)
	require.Zero(t, snapshot.EffectiveOutput)
}

func TestSystem_UsageIsRecordedOnceWhenTheConsumerRuns(t *testing.T) {
	svc := factsService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, svc.StartUsageConsumer(ctx))

	key := &runtimetypes.ProxyKey{
		KeyHash: "h3", ClientID: "laptop",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	_, err := svc.keys.RecordProxyKey(ctx, *key)
	require.NoError(t, err)

	svc.chargeTurn(ctx, key, nil, turnUsage{model: "qwen3:8b", prompt: 100, completion: 40, images: 2, durationMs: 900, finishReason: "stop"})

	require.Never(t, func() bool {
		snapshot, err := svc.usage.ReadUsage(ctx, runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
			Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
		}, time.Now().UTC())
		if err != nil {
			return false
		}
		return snapshot.PromptTokens != 100 || snapshot.ImageCount != 2
	}, 300*time.Millisecond, 50*time.Millisecond, "the turn is metered once, not once per process role")
}

func TestUnit_PulledModelOn_ReadsTheEndpointThatServesIt(t *testing.T) {
	svc := factsService(t)
	ctx := context.Background()

	urls := map[string]string{}
	for _, backend := range []struct {
		id       string
		perImage float64
	}{
		{"a", 1}, {"b", 2},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "script.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
  "model": "shared",
  "capabilities": {"chat": true},
  "turns": [{"text": "ok"}]
}`), 0o600))
		require.NoError(t, svc.keys.CreateBackend(ctx, &runtimetypes.Backend{
			ID: backend.id, Name: backend.id,
			Type: "scripted-test", BaseURL: path,
		}))
		require.NoError(t, svc.keys.SetLLMProviderModelFact(ctx, backend.id, "shared", runtimetypes.ModelFacts{
			Pricing: &runtimetypes.ModelPricing{PerImage: backend.perImage},
		}))
		urls[backend.id] = path
	}
	require.NoError(t, svc.runtime.RunBackendCycle(ctx))

	first := svc.pulledModelOn(ctx, urls["a"], "shared")
	second := svc.pulledModelOn(ctx, urls["b"], "shared")
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.InDelta(t, 1.0, first.Pricing.PerImage, 1e-9)
	require.InDelta(t, 2.0, second.Pricing.PerImage, 1e-9)

	unnamed := svc.findPulledModel(ctx, "shared")
	require.NotNil(t, unnamed)
	for i := 0; i < 20; i++ {
		again := svc.findPulledModel(ctx, "shared")
		require.Equal(t, unnamed.Pricing.PerImage, again.Pricing.PerImage,
			"a caller that names no endpoint gets the same answer every time")
	}
}
