package llama

import (
	"context"
	"errors"
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

func TestUnit_LlamaProvider_CuratedQwenUsesCommonChatToolProtocol(t *testing.T) {
	got := curatedToolProtocol(context.Background(), "qwen3-8b", "llama")
	if got != toolParserProtocolCommonChat {
		t.Fatalf("curated tool protocol = %q, want %q", got, toolParserProtocolCommonChat)
	}
	if got := curatedToolProtocol(context.Background(), "gemma3-4b", "llama"); got != "" {
		t.Fatalf("gemma should not declare a tool protocol, got %q", got)
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
		EffectiveContext:   8192,
		RequestedGpuLayers: 0,
		ResolvedGpuLayers:  27,
	})
	if cfg.NumGpuLayers != 27 {
		t.Fatalf("NumGpuLayers = %d, want resolved auto offload count", cfg.NumGpuLayers)
	}
}
