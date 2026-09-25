package runtimetypes_test

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

func TestUnit_Usage_TotalsAreCumulative(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	recordTurn(t, ctx, usage, now.Add(-2*time.Minute), "qwen3:8b", 100, 40, 250)
	recordTurn(t, ctx, usage, now.Add(-time.Minute), "qwen3:8b", 200, 60, 750)

	week := runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}
	snapshot, err := usage.ReadUsage(ctx, week, now)
	require.NoError(t, err)
	require.EqualValues(t, 300, snapshot.PromptTokens)
	require.EqualValues(t, 100, snapshot.CompletionTokens)
	require.EqualValues(t, 400, snapshot.TotalTokens)
	require.EqualValues(t, 1000, snapshot.CostMicrodollars)

	total := week
	total.WindowKind = runtimetypes.UsageWindowTotal
	snapshot, err = usage.ReadUsage(ctx, total, now)
	require.NoError(t, err)
	require.EqualValues(t, 400, snapshot.TotalTokens, "the lifetime scope saw both turns")
}

func TestUnit_Usage_ScopesAreIndependent(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	recordTurn(t, ctx, usage, now, "qwen3:8b", 100, 40, 250)
	recordTurn(t, ctx, usage, now, "llama3.1:8b", 500, 10, 100)

	other, err := usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "llama3.1:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 10, other.CompletionTokens, "a second model is a second meter")

	foreign, err := usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "someone-else",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 0, foreign.TotalTokens, "a second client is a second meter")

	global, err := usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeGlobal, ScopeID: "",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.EqualValues(t, 140, global.TotalTokens, "the deployment scope counts every client's turns")
}

func TestUnit_Usage_WindowRollIsReadAsZero(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)

	recorded := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	recordTurn(t, ctx, usage, recorded, "qwen3:8b", 100, 40, 250)

	scope := runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen3:8b", WindowKind: runtimetypes.UsageWindowWeek,
	}
	sameWeek, err := usage.ReadUsage(ctx, scope, recorded.Add(2*time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 140, sameWeek.TotalTokens)

	nextWeek, err := usage.ReadUsage(ctx, scope, recorded.AddDate(0, 0, 8))
	require.NoError(t, err)
	require.EqualValues(t, 0, nextWeek.TotalTokens, "the window rolled, so nothing is counted in the new one")
	require.True(t, nextWeek.WindowStart.After(sameWeek.WindowStart))
}

func TestUnit_UsageByModel_NewestPerModel(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	recordTurn(t, ctx, usage, now.Add(-2*time.Minute), "qwen3:8b", 100, 40, 250)
	recordTurn(t, ctx, usage, now.Add(-time.Minute), "qwen3:8b", 100, 40, 250)
	recordTurn(t, ctx, usage, now, "llama3.1:8b", 500, 10, 100)

	byModel, err := usage.UsageByModel(ctx)
	require.NoError(t, err)
	require.Len(t, byModel, 2)
	require.Equal(t, "llama3.1:8b", byModel[0].Model)
	require.EqualValues(t, 510, byModel[0].Snapshot.TotalTokens)
	require.Equal(t, "qwen3:8b", byModel[1].Model)
	require.EqualValues(t, 280, byModel[1].Snapshot.TotalTokens)
}

func TestUnit_UsageWindowStart_Buckets(t *testing.T) {
	five := time.Date(2026, 9, 20, 13, 37, 0, 0, time.UTC)

	require.Equal(t, time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC),
		runtimetypes.UsageWindowStart(runtimetypes.UsageWindowFiveHour, five))
	require.Equal(t, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		runtimetypes.UsageWindowStart(runtimetypes.UsageWindowWeek, five), "a week starts on Monday")
	require.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		runtimetypes.UsageWindowStart(runtimetypes.UsageWindowMonth, five))
	require.True(t, runtimetypes.UsageWindowStart(runtimetypes.UsageWindowTotal, five).IsZero())
}

func TestUnit_Usage_RequiresKeyClientAndModel(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	require.Error(t, usage.RecordUsage(ctx, runtimetypes.ProxyUsage{ClientID: "laptop", Model: "m", RecordedAt: now}))
	require.Error(t, usage.RecordUsage(ctx, runtimetypes.ProxyUsage{KeyHash: "h1", ClientID: "laptop", RecordedAt: now}))
}

func recordTurn(t *testing.T, ctx context.Context, usage runtimetypes.UsageStore, at time.Time, model string, prompt, completion, microdollars int64) {
	t.Helper()
	require.NoError(t, usage.RecordUsage(ctx, runtimetypes.ProxyUsage{
		KeyHash: "h1", ClientID: "laptop", Model: model,
		PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion,
		EffectiveInput: prompt, EffectiveOutput: completion, CostMicrodollars: microdollars,
		RecordedAt: at,
	}))
}

func TestUnit_Usage_ImageCountIsCumulativePerScope(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	for i, images := range []int64{2, 3} {
		require.NoError(t, usage.RecordUsage(ctx, runtimetypes.ProxyUsage{
			KeyHash: "h1", ClientID: "laptop", Model: "qwen2.5vl",
			PromptTokens: 100, CompletionTokens: 5, TotalTokens: 105,
			EffectiveInput: 100, EffectiveOutput: 5, ImageCount: images,
			RecordedAt: now.Add(time.Duration(i) * time.Minute),
		}))
	}

	scope := runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "qwen2.5vl", WindowKind: runtimetypes.UsageWindowWeek,
	}
	snapshot, err := usage.ReadUsage(ctx, scope, now.Add(time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 5, snapshot.ImageCount)

	other, err := usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "someone-else",
		Model: "qwen2.5vl", WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	require.NoError(t, err)
	require.Zero(t, other.ImageCount, "a second client is a second meter for images too")
}

func TestUnit_Usage_AudioBytesAreCumulativePerScope(t *testing.T) {
	ctx, db := runtimetypes.SetupDBManager(t)
	usage := runtimetypes.NewUsageStore(db)
	now := time.Now().UTC()

	for i, audioBytes := range []int64{runtimetypes.Mebibyte, 2 * runtimetypes.Mebibyte} {
		require.NoError(t, usage.RecordUsage(ctx, runtimetypes.ProxyUsage{
			KeyHash: "h1", ClientID: "laptop", Model: "gemma3n",
			PromptTokens: 50, CompletionTokens: 4, TotalTokens: 54,
			EffectiveInput: 50, EffectiveOutput: 4, AudioBytes: audioBytes,
			RecordedAt: now.Add(time.Duration(i) * time.Minute),
		}))
	}

	snapshot, err := usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: "laptop",
		Model: "gemma3n", WindowKind: runtimetypes.UsageWindowWeek,
	}, now.Add(time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 3*runtimetypes.Mebibyte, snapshot.AudioBytes)
}
