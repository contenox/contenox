package openvino

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

type recordingService struct {
	base *transport.MemoryService

	mu      sync.Mutex
	request transport.OpenSessionRequest
	info    transport.ModelInfo
}

func (s *recordingService) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	return s.base.OpenSession(ctx, req)
}

func (s *recordingService) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.request = req
	return s.info, nil
}

func (s *recordingService) lastRequest() transport.OpenSessionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}

func TestUnit_OpenVINOProvider_UsesDescribeResolvedContextAndRuntime(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "config.json"), []byte(`{"max_position_embeddings":32768}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "tokenizer_config.json"), []byte(`{"chat_template":"{{ messages }}"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &recordingService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:  32768,
			EffectiveContext: 24576,
			RuntimeName:      "OpenVINO GenAI",
			RuntimeDigest:    "2026.2-test",
		},
	}
	serveModeldForProviderTest(t, svc)

	p := &openvinoProvider{name: "coder", modelDir: root, caps: modelrepo.CapabilityConfig{}}
	got, err := p.GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.cfg.NumCtx != 24576 {
		t.Fatalf("NumCtx = %d, want Describe effective context", c.cfg.NumCtx)
	}
	if c.backendVersion != "OpenVINO GenAI@2026.2-test" {
		t.Fatalf("backendVersion = %q", c.backendVersion)
	}
	req := svc.lastRequest()
	if req.Config.NumCtx != 0 {
		t.Fatalf("Describe request NumCtx = %d, want unset so modeld can choose max", req.Config.NumCtx)
	}
	if req.Type != "openvino" || req.Digest == "" {
		t.Fatalf("Describe request missing model identity: %+v", req)
	}
}

func TestUnit_OpenVINOProvider_CuratedQwenUsesToolProtocolFallback(t *testing.T) {
	got := curatedToolProtocol(context.Background(), "qwen3-8b-ov", "openvino")
	if got != "qwen" {
		t.Fatalf("curated tool protocol = %q, want qwen", got)
	}
	if got := curatedToolProtocol(context.Background(), "gemma3-4b-ov", "openvino"); got != "" {
		t.Fatalf("gemma should not declare a qwen tool protocol, got %q", got)
	}
	if got := curatedToolProtocol(context.Background(), "qwen3-8b", "openvino"); got != "" {
		t.Fatalf("backend mismatch should not return a protocol, got %q", got)
	}
}

func TestUnit_OpenVINOProvider_ModeldClampLeavesCapacitySafetyMargin(t *testing.T) {
	cfg := clampContextForModeld(Config{NumCtx: 8192}, 4339)
	if cfg.NumCtx != 4275 {
		t.Fatalf("NumCtx = %d, want 4275", cfg.NumCtx)
	}
	if cfg.PromptFormat != "openvino-chat-template" {
		t.Fatalf("PromptFormat = %q", cfg.PromptFormat)
	}

	cfg = clampContextForModeld(Config{NumCtx: 100}, 50)
	if cfg.NumCtx != 50 {
		t.Fatalf("small cap should not subtract safety margin, got %d", cfg.NumCtx)
	}
}

func serveModeldForProviderTest(t *testing.T, svc transport.Service) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()

	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 30*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint, "backend": "openvino"}))
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		t.Fatalf("inspect lease: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, rec.InstanceID, "openvino") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
}
