package contenoxcli

import (
	"context"
	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/acpsvc"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
	libacp "github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newNativeHarness(t *testing.T, endpoint string) *inProcessRuntime {
	t.Helper()
	ctx := context.Background()
	t.Setenv("HOME", t.TempDir())

	workspace := workspaceDir(t, "saas-workspace")
	contenoxDir := filepath.Join(workspace, ".contenox")
	require.NoError(t, os.MkdirAll(contenoxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contenoxDir, "hitl-policy-default.json"), []byte(`{"default_action":"allow"}`), 0o644))
	db := systemTestDB(t)
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID:      "native-worker",
		Name:    "native",
		Type:    "modeld",
		BaseURL: endpoint,
	}))

	var transport *acpsvc.Transport
	transportFn := func() *acpsvc.Transport { return transport }
	hitl := newHITLService(ctx, contenoxDir, runtimetypes.New(db.WithoutTransaction()), libtracker.NoopTracker{}, "")
	router := acpsvc.NewSessionRouter()

	engine, err := BuildEngine(ctx, db, chatOpts{
		EffectiveDefaultModel:    "native-test",
		EffectiveDefaultProvider: "modeld",
		EffectiveContext:         harnessContextTokens,
		ContenoxDir:              contenoxDir,
		EffectiveHITL:            false,
		EffectiveHITLService:     hitl,
		EffectiveAskApproval:     routedAskApproval(router, transportFn),
		EffectiveExtraTools: map[string]taskengine.ToolsRepo{
			localtools.GitToolsName: localtools.NewGitToolsWith("", localtools.GitToolsName,
				acpsvc.NewACPCwdResolver(func(context.Context) *acpsvc.Transport { return transport })),
		},
	})
	require.NoError(t, err)
	t.Cleanup(engine.Stop)

	t.Setenv("CONTENOX_ACP_CHAIN_PATH", generatedACPChain(t))
	chains, err := acpsvc.LoadChainRegistryFrom("chain-agent-acp.json", "CONTENOX_ACP_CHAIN_PATH")
	require.NoError(t, err)

	factory, err := buildWorkspaceFactory(workspace)
	require.NoError(t, err)
	workspaceID := ResolveWorkspaceID(contenoxDir)

	streamR, streamW := io.Pipe()
	sink := &notificationSink{}
	drained := make(chan struct{})
	go drainStream(streamR, sink, drained)

	inboundR, inboundW := io.Pipe()
	_ = inboundR.Close()

	build := acpsvc.New(acpsvc.Deps{
		Engine:               engine,
		DB:                   db,
		ChainRegistry:        chains,
		DefaultContextTokens: &harnessDefaultContextTokens,
		DefaultModel:         "native-test",
		DefaultProvider:      "modeld",
		WorkspaceID:          workspaceID,
		ContenoxDir:          contenoxDir,
		WorkspaceRoots:       factory,
		SessionRouter:        router,
		Asks:                 hitl,
	})
	conn := libacp.NewAgentSideConnection(&duplexPipe{r: inboundR, w: streamW}, func(c *libacp.AgentSideConnection) libacp.Agent {
		agent := build(c)
		transport = agent.(*acpsvc.Transport)
		return agent
	})
	require.NotNil(t, conn)
	t.Cleanup(func() {
		_ = inboundW.Close()
		_ = streamW.Close()
		<-drained
	})

	return &inProcessRuntime{
		t:             t,
		db:            db,
		transport:     transport,
		contenoxDir:   contenoxDir,
		workspace:     workspace,
		workspaceID:   workspaceID,
		notifications: sink,
	}
}

type nativeHarnessService struct {
	*meteredNativeService
	mu      sync.Mutex
	calls   int
	sawTool bool
}

// Describe reports a window that can hold the shipped tool surface. The shared
// stub reports 4096, which cannot hold local_fs alone now that the content and
// browse tools share one namespace, and no real agentic deployment runs that
// small.
func (s *nativeHarnessService) Describe(context.Context, transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return transport.ModelInfo{
		ModelMaxContext:              harnessContextTokens,
		EffectiveContext:             harnessContextTokens,
		HotContextTokens:             harnessContextTokens,
		ChatTemplateSupportsThinking: true,
	}, nil
}

func (s *nativeHarnessService) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	req.Config.NumCtx = harnessContextTokens
	base, err := s.meteredNativeService.MemoryService.OpenSession(ctx, req)
	if err != nil {
		return nil, err
	}
	return &nativeHarnessSession{Session: base, service: s}, nil
}

type nativeHarnessSession struct {
	transport.Session
	service *nativeHarnessService
	suffix  transport.SuffixInput
}

func (s *nativeHarnessSession) PrefillSuffix(ctx context.Context, input transport.SuffixInput) (transport.SuffixStatus, error) {
	s.suffix = input
	return s.Session.PrefillSuffix(ctx, input)
}
func (s *nativeHarnessSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	s.service.mu.Lock()
	defer s.service.mu.Unlock()
	s.service.calls++
	out := make(chan transport.StreamChunk, 1)
	chunk := transport.StreamChunk{FinishReason: "stop", Usage: &transport.TokenUsage{PromptTokens: 100, CompletionTokens: 10, ThinkingTokens: 2}}
	switch s.service.calls {
	case 1:
		chunk.Text = "general"
	case 2:
		call := transport.ToolCall{ID: "native-git-call", Type: "function"}
		call.Function.Name = "git_status"
		call.Function.Arguments = `{}`
		chunk.ToolCalls = []transport.ToolCall{call}
	case 3:
		for _, segment := range s.suffix.Manifest.Segments {
			if segment.Kind == "tool" && segment.ToolCallID == "native-git-call" && segment.ByteEnd > segment.ByteStart {
				s.service.sawTool = true
			}
		}
		chunk.Text = "Native tool roundtrip complete."
	default:
		chunk.Text = "Remember the repository status."
	}
	out <- chunk
	close(out)
	return out, nil
}

// A harness window that can hold the shipped tool surface: local_fs alone is ten
// tools, and a 4096-token stub cannot hold a toolset of that size plus a turn.
const harnessContextTokens = 32768

var harnessDefaultContextTokens = harnessContextTokens

func TestSystem_Modeld_HarnessToolRoundTripAndCompact(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	service := &nativeHarnessService{meteredNativeService: &meteredNativeService{MemoryService: transport.NewMemoryService()}}
	done := make(chan error, 1)
	go func() { done <- transportgrpc.Serve(ctx, listener, service, "harness-owner", "llama") }()
	t.Cleanup(func() { cancel(); <-done })
	r := newNativeHarness(t, listener.Addr().String())
	sid := r.newSession(t, r.workspace)
	r.prompt(t, sid, "Inspect the repository status.")
	require.Contains(t, r.notifications.text(), "Native tool roundtrip complete.")

	service.mu.Lock()
	calls, sawTool := service.calls, service.sawTool
	service.mu.Unlock()
	require.Equal(t, 3, calls)
	require.True(t, sawTool, "tool result and call ID must reach the native model")
	internalID := r.internalID(t, sid)
	usage, err := runtimetypes.NewUsageStore(r.db).UsageByScope(ctx, runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(gateway.LocalClientID, internalID))
	require.NoError(t, err)
	require.Len(t, usage, 1)
	require.EqualValues(t, 330, usage[0].Snapshot.TotalTokens)
	require.EqualValues(t, 6, usage[0].Snapshot.ThinkingTokens)
	require.NoError(t, os.MkdirAll(systemDir(r.contenoxDir), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(systemDir(r.contenoxDir), chainCompactDefaultFilename), []byte(initCompactChain), 0600))
	r.prompt(t, sid, "/compact 0")
	require.Contains(t, r.notifications.text(), "Compacted")
	stored := r.storedMessages(t, internalID)
	found := false
	for _, msg := range stored {
		if msg.Content == "<compact-summary>\nRemember the repository status.\n</compact-summary>" {
			found = true
		}
	}
	require.True(t, found, "native compaction summary must persist")
	usage, err = runtimetypes.NewUsageStore(r.db).UsageByScope(ctx, runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(gateway.LocalClientID, internalID))
	require.NoError(t, err)
	require.EqualValues(t, 440, usage[0].Snapshot.TotalTokens)
}
