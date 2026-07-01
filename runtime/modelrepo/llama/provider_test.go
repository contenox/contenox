package llama

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/transport"
)

func TestUnit_LocalNodeProvider_DefaultBuildReportsNotWired(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })
	isolateModeldDataRoot(t)

	p := newProvider("coder", "/models", modelrepo.CapabilityConfig{ContextLength: 8192})

	if p.CanChat() || p.CanPrompt() || p.CanStream() {
		t.Fatal("provider should not advertise generation without a compiled session backend")
	}
	if p.CanEmbed() {
		t.Fatal("llama should not advertise embeddings")
	}
	if p.GetType() != "llama" || p.GetID() != "llama:coder" {
		t.Fatalf("unexpected provider identity: type=%s id=%s", p.GetType(), p.GetID())
	}
	_, err := p.GetChatConnection(context.Background(), "llama")
	if !errors.Is(err, ErrSessionUnavailable) || !strings.Contains(err.Error(), "running modeld serving the llama backend") {
		t.Fatalf("expected not-wired error, got: %v", err)
	}
}

func TestUnit_LlamaProvider_CuratedModelsUseCommonChatToolProtocol(t *testing.T) {
	got := curatedToolProtocol(context.Background(), "qwen3-8b", "llama")
	if got != toolParserProtocolCommonChat {
		t.Fatalf("curated tool protocol = %q, want %q", got, toolParserProtocolCommonChat)
	}
	if got := curatedToolProtocol(context.Background(), "gemma4-e4b", "llama"); got != toolParserProtocolCommonChat {
		t.Fatalf("curated gemma tool protocol = %q, want %q", got, toolParserProtocolCommonChat)
	}
	if got := curatedToolProtocol(context.Background(), "qwen3-8b-ov", "llama"); got != "" {
		t.Fatalf("backend mismatch should not return a protocol, got %q", got)
	}
}

func TestUnit_LlamaProvider_CuratedReasoningProtocolFallback(t *testing.T) {
	if got := curatedReasoningProtocol(context.Background(), "qwen3-8b", "llama"); got != reasoningProtocolCommonChat {
		t.Fatalf("curated qwen reasoning protocol = %q, want %q", got, reasoningProtocolCommonChat)
	}
	if got := curatedReasoningFormat(context.Background(), "qwen3-8b", "llama"); got != "deepseek" {
		t.Fatalf("curated qwen reasoning format = %q, want deepseek", got)
	}
	if got := curatedReasoningProtocol(context.Background(), "deepseek-r1-distill-qwen-7b", "llama"); got != reasoningProtocolCommonChat {
		t.Fatalf("curated deepseek reasoning protocol = %q, want %q", got, reasoningProtocolCommonChat)
	}
	if got := curatedReasoningProtocol(context.Background(), "qwen3-coder-30b-a3b", "llama"); got != "" {
		t.Fatalf("coder should not declare reasoning parser, got %q", got)
	}
	if got := curatedReasoningProtocol(context.Background(), "qwen3-8b-ov", "llama"); got != "" {
		t.Fatalf("backend mismatch should not return a reasoning protocol, got %q", got)
	}
}

func TestUnit_LlamaProvider_ModeldClampLeavesCapacitySafetyMargin(t *testing.T) {
	cfg := clampContextForModeld(Config{NumCtx: 8192}, 4339)
	if cfg.NumCtx != 4275 {
		t.Fatalf("NumCtx = %d, want 4275", cfg.NumCtx)
	}

	cfg = clampContextForModeld(Config{NumCtx: 100}, 50)
	if cfg.NumCtx != 50 {
		t.Fatalf("small cap should not subtract safety margin, got %d", cfg.NumCtx)
	}
}

func TestUnit_LlamaProvider_UsesResolvedAutoGpuLayersFromModeld(t *testing.T) {
	cfg := applyModeldInfoToConfig(Config{NumCtx: 8192}, transport.ModelInfo{
		ModelMaxContext:         32768,
		EffectiveContext:        8192,
		PlannerEffectiveContext: 16384,
		RequestedGpuLayers:      0,
		ResolvedGpuLayers:       27,
	})
	if cfg.NumGpuLayers != 27 {
		t.Fatalf("NumGpuLayers = %d, want resolved auto offload count", cfg.NumGpuLayers)
	}
	if cfg.PlannerEffectiveContext != 16384 {
		t.Fatalf("PlannerEffectiveContext = %d, want modeld planner context", cfg.PlannerEffectiveContext)
	}
}

func TestUnit_LlamaProvider_UsesDescribeResolvedContextWhenProfileOmitsNumCtx(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &recordingEmbedService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:         32768,
			EffectiveContext:        24576,
			PlannerEffectiveContext: 32768,
			RuntimeName:             "llamacpp",
			RuntimeDigest:           "test",
		},
	}
	serveLlamaModeldForTest(t, svc)

	got, err := newProvider("coder", root, modelrepo.CapabilityConfig{}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.cfg.NumCtx != 24576-modeldCapacitySafetyTokens {
		t.Fatalf("NumCtx = %d, want Describe effective context minus modeld safety margin", c.cfg.NumCtx)
	}
	if c.cfg.PlannerEffectiveContext != 32768 {
		t.Fatalf("PlannerEffectiveContext = %d, want modeld planner context", c.cfg.PlannerEffectiveContext)
	}
	req := svc.lastDescribeRequest()
	if req.Config.NumCtx != 0 {
		t.Fatalf("Describe request NumCtx = %d, want unset so modeld can discover max", req.Config.NumCtx)
	}
}

func TestUnit_LlamaProvider_ExplicitProfileContextStillCapsDescribe(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, profileFileName), []byte(`{"runtime":{"num_ctx":4096}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &recordingEmbedService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:         32768,
			EffectiveContext:        24576,
			PlannerEffectiveContext: 32768,
		},
	}
	serveLlamaModeldForTest(t, svc)

	got, err := newProvider("coder", root, modelrepo.CapabilityConfig{ContextLength: 32768}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.cfg.NumCtx != 4096 {
		t.Fatalf("NumCtx = %d, want explicit profile context", c.cfg.NumCtx)
	}
	if c.cfg.PlannerEffectiveContext != 32768 {
		t.Fatalf("PlannerEffectiveContext = %d, want modeld planner context", c.cfg.PlannerEffectiveContext)
	}
	req := svc.lastDescribeRequest()
	if req.Config.NumCtx != 4096 {
		t.Fatalf("Describe request NumCtx = %d, want explicit profile context", req.Config.NumCtx)
	}
}

func TestUnit_LlamaProvider_ProfileAdaptersReachModelRef(t *testing.T) {
	withSessionFactory(t, func(string, Config) (Session, error) { return nil, nil })

	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterBytes := []byte("fake adapter")
	adapterPath := filepath.Join(modelDir, "style.gguf")
	if err := os.WriteFile(adapterPath, adapterBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, profileFileName), []byte(`{
		"adapters":[{"name":"style","path":"style.gguf"}],
		"runtime":{"num_ctx":4096}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(adapterBytes)
	wantDigest := hex.EncodeToString(sum[:])

	got, err := newProvider("coder", root, modelrepo.CapabilityConfig{ContextLength: 4096}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	ref := c.ref()
	if len(ref.Adapters) != 1 {
		t.Fatalf("adapters = %#v, want one", ref.Adapters)
	}
	adapter := ref.Adapters[0]
	if adapter.Name != "style" || adapter.Path != adapterPath || adapter.Digest != wantDigest || adapter.Scale != 1 {
		t.Fatalf("unexpected adapter: %#v", adapter)
	}
}
