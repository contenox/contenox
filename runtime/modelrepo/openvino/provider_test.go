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
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

type recordingService struct {
	base *transport.MemoryService

	mu      sync.Mutex
	request transport.OpenSessionRequest
	embed   transport.EmbedRequest
	info    transport.ModelInfo
	vector  []float32
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

func (s *recordingService) Embed(_ context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embed = req
	return transport.EmbedResult{Vector: s.vector}, nil
}

func (s *recordingService) lastRequest() transport.OpenSessionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}

func (s *recordingService) lastEmbedRequest() transport.EmbedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.embed
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

	profile, err := loadModelProfile(modelDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	p := &openvinoProvider{name: "coder", modelDir: root, caps: profile.capabilityConfig()}
	got, err := p.GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.cfg.NumCtx != 24576-modeldCapacitySafetyTokens {
		t.Fatalf("NumCtx = %d, want Describe effective context minus modeld safety margin", c.cfg.NumCtx)
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

func TestUnit_OpenVINOProvider_EmbeddingUsesModeldTransport(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "embedder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "contenox-openvino.json"), []byte(`{"can_chat":false,"can_embed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &recordingService{
		base:   transport.NewMemoryService(),
		vector: []float32{1.25, -0.5, 3},
	}
	serveModeldForProviderTest(t, svc)

	profile, err := loadModelProfile(modelDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	p := &openvinoProvider{name: "embedder", modelDir: root, caps: profile.capabilityConfig()}
	if p.CanChat() {
		t.Fatal("embedding-only profile should not advertise chat")
	}
	if !p.CanEmbed() {
		t.Fatal("embedding profile should advertise embeddings when modeld is available")
	}

	conn, err := p.GetEmbedConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetEmbedConnection: %v", err)
	}
	got, err := conn.Embed(context.Background(), "hello embeddings")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	want := []float64{1.25, -0.5, 3}
	if len(got) != len(want) {
		t.Fatalf("embedding length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("embedding[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	req := svc.lastEmbedRequest()
	if req.Type != "openvino" || req.ModelName != "embedder" || req.Path != modelDir || req.Digest == "" {
		t.Fatalf("Embed request missing model identity: %+v", req)
	}
	if req.Text != "hello embeddings" {
		t.Fatalf("Embed text = %q", req.Text)
	}
}

func TestUnit_OpenVINOProvider_CuratedQwenUsesNativeOpenVINOProtocol(t *testing.T) {
	got := curatedToolProtocol(context.Background(), "qwen3-8b-ov", "openvino")
	if got != "openvino:json_schema_tool_calls" {
		t.Fatalf("curated OpenVINO qwen protocol = %q, want native JSON schema protocol", got)
	}
	if got := curatedToolProtocol(context.Background(), "qwen2.5-coder-0.5b-ov", "openvino"); got != "openvino:json_schema_tool_calls" {
		t.Fatalf("curated OpenVINO qwen2.5 protocol = %q, want native JSON schema protocol", got)
	}
	if got := curatedToolProtocol(context.Background(), "gemma3-4b-ov", "openvino"); got != "" {
		t.Fatalf("gemma should not declare a tool protocol, got %q", got)
	}
	if got := curatedToolProtocol(context.Background(), "qwen3-8b", "openvino"); got != "" {
		t.Fatalf("backend mismatch should not return a protocol, got %q", got)
	}
}

func TestUnit_OpenVINOProfile_ReasoningProtocolDerivesStreamParserAndCapability(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "deepseek")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "contenox-openvino.json"), []byte(`{"reasoning":{"protocol":"openvino:deepseek_r1_reasoning_parser"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	profile, err := loadModelProfile(modelDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if !profile.capabilityConfig().CanThink {
		t.Fatal("reasoning protocol should advertise CanThink")
	}
	_, stream := profile.Reasoning.protocols()
	if stream != "openvino:deepseek_r1_reasoning_incremental_parser" {
		t.Fatalf("stream protocol = %q", stream)
	}
}

func TestUnit_OpenVINOProfile_ToolProtocolMustBeNativeOpenVINOProtocol(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "llama3")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(modelDir, "contenox-openvino.json")
	if err := os.WriteFile(path, []byte(`{"tool_calls":{"protocol":"openvino:llama3_json_tool_parser"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelProfile(modelDir); err != nil {
		t.Fatalf("native OpenVINO parser should be accepted: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"tool_calls":{"protocol":"openvino:json_schema_tool_calls"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelProfile(modelDir); err != nil {
		t.Fatalf("OpenVINO JSON-schema tool-call protocol should be accepted: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"tool_calls":{"protocol":"openvino:qwen_xml_parameters"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelProfile(modelDir); err == nil {
		t.Fatal("OpenVINO qwen XML parameters protocol should not be accepted")
	}

	if err := os.WriteFile(path, []byte(`{"tool_calls":{"protocol":"qwen"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelProfile(modelDir); err == nil {
		t.Fatal("qwen should not be accepted as an OpenVINO parser protocol")
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
