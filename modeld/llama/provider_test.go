package llama

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/modeld"
)

func TestUnit_LocalNodeProvider_DefaultBuildReportsNotWired(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })

	p := newProvider("coder", "/models", modeld.CapabilityConfig{ContextLength: 8192})

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
	if !errors.Is(err, ErrSessionUnavailable) || !strings.Contains(err.Error(), "compile with -tags llamanode") {
		t.Fatalf("expected not-wired error, got: %v", err)
	}
}
