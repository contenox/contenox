package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

type localModels struct {
	llmrepo.ModelRepo
	result   modelrepo.ChatResult
	meta     llmrepo.Meta
	err      error
	stream   chan *modelrepo.StreamParcel
	request  llmrepo.Request
	messages []modelrepo.Message
	options  []modelrepo.ChatArgument
}

func (m *localModels) Chat(_ context.Context, req llmrepo.Request, messages []modelrepo.Message, opts ...modelrepo.ChatArgument) (modelrepo.ChatResult, llmrepo.Meta, error) {
	m.request, m.messages, m.options = req, messages, opts
	return m.result, m.meta, m.err
}

func (m *localModels) Stream(_ context.Context, req llmrepo.Request, messages []modelrepo.Message, opts ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, llmrepo.Meta, error) {
	m.request, m.messages, m.options = req, messages, opts
	return m.stream, m.meta, m.err
}

func (m *localModels) PromptExecute(context.Context, llmrepo.Request, string, float32, string) (string, llmrepo.Meta, error) {
	return "partial", m.meta, m.err
}

func TestUnit_LocalRepo_RecordsResolvedModelAndSessionWithoutCredentials(t *testing.T) {
	svc := factsService(t)
	upstreamErr := errors.New("provider interrupted")
	model := &localModels{
		meta: llmrepo.Meta{ModelName: "resolved"}, err: upstreamErr,
		result: modelrepo.ChatResult{Usage: &modelrepo.TokenUsage{PromptTokens: 100, CompletionTokens: 40, ThinkingTokens: 15, CacheReadTokens: 80}},
	}
	repo, err := NewLocalRepo(svc.db, model, svc.runtime, svc.tracker)
	require.NoError(t, err)
	ctx := llmrepo.WithUsageSession(context.Background(), "session-a")
	messages := []modelrepo.Message{{Role: "user", Content: "hi"}}
	req := llmrepo.Request{SessionKey: "cache-session"}
	result, meta, err := repo.Chat(ctx, req, messages)
	require.ErrorIs(t, err, upstreamErr)
	require.Equal(t, model.result, result)
	require.Equal(t, model.meta, meta)
	require.Equal(t, req, model.request)
	require.Equal(t, messages, model.messages)
	totals, err := svc.usage.UsageByScope(ctx, runtimetypes.UsageScopeClient, LocalClientID)
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.Equal(t, "resolved", totals[0].Model)
	require.EqualValues(t, 100, totals[0].Snapshot.EffectiveInput)
	require.EqualValues(t, 40, totals[0].Snapshot.EffectiveOutput)
	require.EqualValues(t, 15, totals[0].Snapshot.ThinkingTokens)
	session, err := svc.usage.UsageByScope(ctx, runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(LocalClientID, "session-a"))
	require.NoError(t, err)
	require.Equal(t, totals, session)
	other, err := svc.usage.UsageByScope(ctx, runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(LocalClientID, "session-b"))
	require.NoError(t, err)
	require.Empty(t, other)
	global, err := svc.usage.UsageByModel(ctx)
	require.NoError(t, err)
	require.Equal(t, totals, global)
}

func TestUnit_LocalRepo_StreamPreservesParcelsAndMetersOnce(t *testing.T) {
	svc := factsService(t)
	parcels := []*modelrepo.StreamParcel{
		{Usage: &modelrepo.TokenUsage{PromptTokens: 100, CacheReadTokens: 80}},
		{Thinking: "reasoning"},
		{Data: "answer"},
		{ToolCall: &modelrepo.ToolCallDelta{Index: 0, ID: "call-1", Name: "search", ArgsFragment: "{}", ProviderMeta: map[string]string{"signature": "opaque"}}},
		{Terminal: &modelrepo.StreamTerminal{FinishReason: "tool_calls", Usage: &modelrepo.TokenUsage{CompletionTokens: 40, ThinkingTokens: 15}}},
	}
	model := &localModels{meta: llmrepo.Meta{ModelName: "resolved"}, stream: make(chan *modelrepo.StreamParcel, len(parcels))}
	for _, parcel := range parcels {
		model.stream <- parcel
	}
	close(model.stream)
	repo, err := NewLocalRepo(svc.db, model, svc.runtime, svc.tracker)
	require.NoError(t, err)
	stream, _, err := repo.Stream(context.Background(), llmrepo.Request{}, nil)
	require.NoError(t, err)
	i := 0
	for parcel := range stream {
		require.Same(t, parcels[i], parcel)
		i++
	}
	require.Equal(t, len(parcels), i)
	totals, err := svc.usage.UsageByScope(context.Background(), runtimetypes.UsageScopeClient, LocalClientID)
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.EqualValues(t, 140, totals[0].Snapshot.TotalTokens)
	require.EqualValues(t, 80, totals[0].Snapshot.CacheReadTokens)
	require.EqualValues(t, 15, totals[0].Snapshot.ThinkingTokens)
}

func TestUnit_LocalRepo_CancelPersistsReportedUsage(t *testing.T) {
	svc := factsService(t)
	model := &localModels{meta: llmrepo.Meta{ModelName: "resolved"}, stream: make(chan *modelrepo.StreamParcel, 1)}
	model.stream <- &modelrepo.StreamParcel{Usage: &modelrepo.TokenUsage{PromptTokens: 100, CompletionTokens: 2}}
	repo, err := NewLocalRepo(svc.db, model, svc.runtime, svc.tracker)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, _, err := repo.Stream(ctx, llmrepo.Request{}, nil)
	require.NoError(t, err)
	<-stream
	cancel()
	select {
	case _, ok := <-stream:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("meter waited for an upstream that did not close")
	}
	totals, err := svc.usage.UsageByModel(context.Background())
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.EqualValues(t, 102, totals[0].Snapshot.TotalTokens)
}

func TestUnit_LocalRepo_PromptKeepsUsageOnError(t *testing.T) {
	svc := factsService(t)
	model := &localModels{meta: llmrepo.Meta{ModelName: "resolved", Usage: &modelrepo.TokenUsage{PromptTokens: 5, CompletionTokens: 2}}, err: errors.New("interrupted")}
	repo, err := NewLocalRepo(svc.db, model, svc.runtime, svc.tracker)
	require.NoError(t, err)
	result, _, err := repo.PromptExecute(context.Background(), llmrepo.Request{}, "", 0, "hi")
	require.Equal(t, "partial", result)
	require.ErrorIs(t, err, model.err)
	totals, err := svc.usage.UsageByModel(context.Background())
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.EqualValues(t, 7, totals[0].Snapshot.TotalTokens)
}
