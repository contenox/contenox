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

func TestUnit_LlamaProvider_NeverBakesGpuLayersFromModeld(t *testing.T) {
	// GPU offload is modeld's to resolve live from hardware telemetry. Baking a
	// Describe-time ResolvedGpuLayers into the open request pins the session to a
	// stale, encumbered snapshot (see capacity.HardContextLimit) and — because it
	// implies an explicit request — bypasses modeld's usable-context floor. Even
	// for a genuine pinned num_ctx the runtime must leave GPU layers unset.
	cfg := applyModeldInfoToConfig(Config{NumCtx: 8192}, transport.ModelInfo{
		ModelMaxContext:         32768,
		EffectiveContext:        8192,
		PlannerEffectiveContext: 16384,
		RequestedGpuLayers:      0,
		ResolvedGpuLayers:       27,
	})
	if cfg.NumGpuLayers != 0 {
		t.Fatalf("NumGpuLayers = %d, want 0 (modeld resolves GPU offload; the runtime never bakes it)", cfg.NumGpuLayers)
	}
	if cfg.PlannerEffectiveContext != 16384 {
		t.Fatalf("PlannerEffectiveContext = %d, want modeld planner context", cfg.PlannerEffectiveContext)
	}
}

func TestUnit_LlamaProvider_AutoContextStaysZeroWhenProfileOmitsNumCtx(t *testing.T) {
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
	// Describe's answer is a snapshot that may be stale by open time (e.g. taken
	// while another session was still resident). Baking it into cfg would make
	// it a hard Request ceiling modeld can never raise — the real session must
	// go out with NumCtx=0 so modeld resolves the window fresh, post-eviction.
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
	if c.describedModelMaxContext != 32768 {
		t.Fatalf("describedModelMaxContext = %d, want Describe answer kept informationally", c.describedModelMaxContext)
	}
	req := svc.lastDescribeRequest()
	if req.Config.NumCtx != 0 {
		t.Fatalf("Describe request NumCtx = %d, want unset so modeld can discover max", req.Config.NumCtx)
	}
}

func TestUnit_LlamaProvider_RequestedContextDoesNotPinAutoWindow(t *testing.T) {
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
		},
	}
	serveLlamaModeldForTest(t, svc)

	// A per-request prompt-size requirement is NOT a user pin. It must not turn the
	// auto request into a concrete num_ctx — the window stays modeld's to resolve
	// live at open, so the real session (and the Describe probe) go out with
	// NumCtx=0. Pinning it here reintroduces the stale-window / floor-bypass bug.
	ctx := modelrepo.WithRequestedContextLength(context.Background(), 8192)
	got, err := newProvider("coder", root, modelrepo.CapabilityConfig{ContextLength: 32768}).GetChatConnection(ctx, "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.cfg.NumCtx != 0 {
		t.Fatalf("NumCtx = %d, want 0 (a prompt requirement must not pin the auto window)", c.cfg.NumCtx)
	}
	req := svc.lastDescribeRequest()
	if req.Config.NumCtx != 0 {
		t.Fatalf("Describe request NumCtx = %d, want 0 (auto)", req.Config.NumCtx)
	}
}

// TestUnit_LlamaProvider_AutoContextKeepsWarmCacheKeyStableAcrossJitter is the
// regression test for the shrink-then-freeze bug: with no explicit context,
// two client constructions whose Describe answers differ (live free-VRAM
// jitter) must still produce the identical session cache key, so the warm
// session is reused instead of being evicted and reopened with a stale,
// smaller ceiling every turn.
func TestUnit_LlamaProvider_AutoContextKeepsWarmCacheKeyStableAcrossJitter(t *testing.T) {
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
			EffectiveContext:        17198,
			PlannerEffectiveContext: 24576,
		},
	}
	serveLlamaModeldForTest(t, svc)

	first, err := newProvider("coder", root, modelrepo.CapabilityConfig{}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	// Simulate memory jitter: the next Describe reports a much smaller window
	// (e.g. taken while the previous session is still resident).
	svc.info.EffectiveContext = 366
	svc.info.PlannerEffectiveContext = 14102
	second, err := newProvider("coder", root, modelrepo.CapabilityConfig{}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}

	a, b := first.(*client), second.(*client)
	keyA := sessionCacheKey(a.ref(), a.cfg)
	keyB := sessionCacheKey(b.ref(), b.cfg)
	if keyA != keyB {
		t.Fatalf("session cache keys differ across Describe jitter:\n  a=%q\n  b=%q\nwarm reuse would evict and reopen every turn", keyA, keyB)
	}
	if b.cfg.NumCtx != 0 {
		t.Fatalf("second client NumCtx = %d, want 0 (jittery Describe answer must not become a request ceiling)", b.cfg.NumCtx)
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

func TestUnit_LlamaProvider_UsesModeldDetectedTemplateCapabilities(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	root := t.TempDir()
	modelDir := filepath.Join(root, "uncurated")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &recordingEmbedService{
		base: transport.NewMemoryService(),
		info: transport.ModelInfo{
			ModelMaxContext:               32768,
			EffectiveContext:              8192,
			ChatTemplateSupportsToolCalls: true,
			ChatTemplateReasoningFormat:   "auto",
			ChatTemplateSupportsThinking:  true,
		},
	}
	serveLlamaModeldForTest(t, svc)

	got, err := newProvider("uncurated", root, modelrepo.CapabilityConfig{}).GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := got.(*client)
	if c.toolProtocol != toolParserProtocolCommonChat {
		t.Fatalf("tool protocol = %q, want modeld-detected common_chat", c.toolProtocol)
	}
	if c.reasoningProtocol != reasoningProtocolCommonChat {
		t.Fatalf("reasoning protocol = %q, want modeld-detected common_chat", c.reasoningProtocol)
	}
	if c.cfg.ReasoningFormat != "auto" {
		t.Fatalf("reasoning format = %q, want modeld-detected auto", c.cfg.ReasoningFormat)
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
