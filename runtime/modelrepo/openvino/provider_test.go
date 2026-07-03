package openvino

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestUnit_OpenVINOProvider_AutoContextStaysZeroAndRuntimeIdentityKept(t *testing.T) {
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
			ModelMaxContext:         32768,
			EffectiveContext:        24576,
			PlannerEffectiveContext: 32768,
			RuntimeName:             "OpenVINO GenAI",
			RuntimeDigest:           "2026.2-test",
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
	// Describe's answer stays informational: baking it into cfg would make a
	// possibly-stale snapshot a hard Request ceiling. The real session goes out
	// with NumCtx=0 so modeld resolves the window fresh, post-eviction.
	if c.cfg.NumCtx != 0 {
		t.Fatalf("NumCtx = %d, want 0 so modeld resolves the window fresh at open", c.cfg.NumCtx)
	}
	if c.cfg.PlannerEffectiveContext != 0 {
		t.Fatalf("PlannerEffectiveContext = %d, want 0 (auto)", c.cfg.PlannerEffectiveContext)
	}
	if c.describedEffectiveContext != 24576 {
		t.Fatalf("describedEffectiveContext = %d, want Describe answer kept informationally", c.describedEffectiveContext)
	}
	if c.describedPlannerContext != 32768 {
		t.Fatalf("describedPlannerContext = %d, want Describe answer kept informationally", c.describedPlannerContext)
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

func TestUnit_OpenVINOProvider_UsesModeldDetectedTemplateThinkingControls(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "uncurated")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "tokenizer_config.json"), []byte(`{"chat_template":"{{ messages }}"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &recordingService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:                     32768,
			EffectiveContext:                    8192,
			ChatTemplateSupportsThinking:        true,
			ChatTemplateSupportsReasoningEffort: true,
			ChatTemplateReasoningFormat:         "auto",
		},
	}
	serveModeldForProviderTest(t, svc)

	profile, err := loadModelProfile(modelDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	got, err := (&openvinoProvider{name: "uncurated", modelDir: root, caps: profile.capabilityConfig()}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if !c.supportsThinking {
		t.Fatal("client supportsThinking = false, want modeld-detected template thinking controls")
	}
	if c.toolProtocol != "" {
		t.Fatalf("tool protocol = %q, want no inferred tool parser without profile/catalog protocol", c.toolProtocol)
	}
}

func TestUnit_OpenVINOCatalog_UsesModeldDetectedTemplateThinkingControls(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "uncurated")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "tokenizer_config.json"), []byte(`{"chat_template":"{{ messages }}"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &recordingService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:                     32768,
			EffectiveContext:                    8192,
			PlannerEffectiveContext:             16384,
			ChatTemplateSupportsReasoningEffort: true,
		},
	}
	serveModeldForProviderTest(t, svc)

	models, err := (&catalogProvider{dir: root}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want one", len(models))
	}
	if !models[0].CapabilityConfig.CanThink {
		t.Fatalf("CanThink = false, want modeld-detected template thinking controls: %+v", models[0].CapabilityConfig)
	}
	if models[0].ContextLength != 16384 {
		t.Fatalf("ContextLength = %d, want planner context from Describe", models[0].ContextLength)
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

func TestUnit_OpenVINOProvider_ProfileAdaptersReachModelRef(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "tokenizer_config.json"), []byte(`{"chat_template":"{{ messages }}"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterBytes := []byte("fake adapter")
	adapterPath := filepath.Join(modelDir, "style.safetensors")
	if err := os.WriteFile(adapterPath, adapterBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, profileFileName), []byte(`{
		"adapters":[{"name":"style","path":"style.safetensors","scale":3}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(adapterBytes)
	wantDigest := hex.EncodeToString(sum[:])

	oldFactory := sessionFactory
	sessionFactory = func(modeldconn.ModelRef, Config) (Session, error) { return nil, nil }
	t.Cleanup(func() { sessionFactory = oldFactory })

	profile, err := loadModelProfile(modelDir)
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	got, err := (&openvinoProvider{name: "coder", modelDir: root, caps: profile.capabilityConfig()}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	ref := c.ref()
	if len(ref.Adapters) != 1 {
		t.Fatalf("adapters = %#v, want one", ref.Adapters)
	}
	adapter := ref.Adapters[0]
	if adapter.Name != "style" || adapter.Path != adapterPath || adapter.Digest != wantDigest || adapter.Scale != 3 {
		t.Fatalf("unexpected adapter: %#v", adapter)
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
