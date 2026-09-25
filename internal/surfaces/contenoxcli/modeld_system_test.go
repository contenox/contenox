package contenoxcli

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/backendservice"
	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelruntime"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

type meteredNativeService struct {
	*transport.MemoryService
	transport.NodeAdmin
}

func (s *meteredNativeService) ListModels(context.Context) ([]transport.NodeModel, error) {
	return []transport.NodeModel{{Name: "native-test", Type: "llama"}}, nil
}

func (s *meteredNativeService) Describe(context.Context, transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return transport.ModelInfo{ModelMaxContext: 32768, EffectiveContext: 4096, ChatTemplateSupportsThinking: true}, nil
}

func (s *meteredNativeService) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	req.Config.NumCtx = 4096
	sess, err := s.MemoryService.OpenSession(ctx, req)
	return &meteredNativeSession{Session: sess}, err
}

type meteredNativeSession struct {
	transport.Session
}

func (s *meteredNativeSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	out := make(chan transport.StreamChunk, 2)
	out <- transport.StreamChunk{Thinking: "check"}
	out <- transport.StreamChunk{Text: "native answer", FinishReason: "stop", Usage: &transport.TokenUsage{PromptTokens: 20, CompletionTokens: 8, ThinkingTokens: 3}}
	close(out)
	return out, nil
}

func TestSystem_Modeld_RuntimeGatewayMetering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	service := &meteredNativeService{MemoryService: transport.NewMemoryService()}
	go func() { _ = transportgrpc.Serve(ctx, listener, service, "native-owner", "llama") }()
	db, err := OpenDBAt(ctx, filepath.Join(t.TempDir(), "native.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, backendservice.New(db).Create(ctx, &runtimetypes.Backend{ID: "native-worker", Name: "native", Type: "modeld", BaseURL: listener.Addr().String()}))
	rt, err := modelruntime.Build(ctx, db, modelruntime.Config{DefaultModel: "native-test", DefaultProvider: "modeld"})
	require.NoError(t, err)
	defer rt.Stop()
	states := rt.State.Get(ctx)
	require.Empty(t, states["native-worker"].Error)
	require.Len(t, states["native-worker"].PulledModels, 1)
	require.Equal(t, 4096, states["native-worker"].PulledModels[0].ContextLength)
	require.False(t, states["native-worker"].PulledModels[0].CanVision)
	repo, err := gateway.NewLocalRepo(db, rt.Models, rt.State, libtracker.NoopTracker{})
	require.NoError(t, err)
	requestCtx := llmrepo.WithUsageSession(ctx, "native-session")
	result, _, err := repo.Chat(requestCtx, llmrepo.Request{ModelNames: []string{"native-test"}, ProviderTypes: []string{"modeld"}}, []modelrepo.Message{{Role: "user", Content: "hello"}}, modelrepo.WithMaxTokens(32))
	require.NoError(t, err)
	require.Equal(t, "native answer", result.Message.Content)
	require.Equal(t, "check", result.Message.Thinking)
	require.NotNil(t, result.Usage)
	require.Equal(t, 3, result.Usage.ThinkingTokens)
	usage, err := runtimetypes.NewUsageStore(db).UsageByScope(ctx, runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(gateway.LocalClientID, "native-session"))
	require.NoError(t, err)
	require.Len(t, usage, 1)
	require.EqualValues(t, 28, usage[0].Snapshot.TotalTokens)
	require.EqualValues(t, 3, usage[0].Snapshot.ThinkingTokens)
}
