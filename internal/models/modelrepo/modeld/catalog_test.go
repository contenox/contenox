package modeld

import (
	"context"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
)

func TestUnit_ModeldCatalogRegistered(t *testing.T) {
	for _, typ := range []string{"modeld", "local"} {
		cat, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{
			Type:    typ,
			BaseURL: "local",
		})
		if err != nil {
			t.Fatalf("NewCatalogProvider(%q) failed: %v", typ, err)
		}
		if cat.Type() != "modeld" {
			t.Fatalf("cat.Type() = %q, want 'modeld'", cat.Type())
		}
	}
}

func TestUnit_ModeldProviderMethods(t *testing.T) {
	prov := NewModeldProvider("qwen3-8b", []string{"local"}, modelrepo.CapabilityConfig{
		ContextLength: 8192,
		CanChat:       true,
		CanStream:     true,
		CanPrompt:     true,
		CanEmbed:      true,
		CanThink:      true,
		CanVision:     true,
	}, nil)

	if prov.ModelName() != "qwen3-8b" {
		t.Errorf("ModelName = %q, want 'qwen3-8b'", prov.ModelName())
	}
	if prov.GetType() != "modeld" {
		t.Errorf("GetType = %q, want 'modeld'", prov.GetType())
	}
	if prov.GetID() != "modeld:qwen3-8b" {
		t.Errorf("GetID = %q, want 'modeld:qwen3-8b'", prov.GetID())
	}
	if !prov.CanChat() || !prov.CanStream() || !prov.CanPrompt() || !prov.CanEmbed() || !prov.CanThink() || !prov.CanVision() {
		t.Errorf("expected all capabilities to be true")
	}
	if prov.CanAudio() {
		t.Errorf("CanAudio must be false")
	}

	chatClient, err := prov.GetChatConnection(context.Background(), "local")
	if err != nil || chatClient == nil {
		t.Fatalf("GetChatConnection error: %v", err)
	}

	streamClient, err := prov.GetStreamConnection(context.Background(), "local")
	if err != nil || streamClient == nil {
		t.Fatalf("GetStreamConnection error: %v", err)
	}

	promptClient, err := prov.GetPromptConnection(context.Background(), "local")
	if err != nil || promptClient == nil {
		t.Fatalf("GetPromptConnection error: %v", err)
	}

	embedClient, err := prov.GetEmbedConnection(context.Background(), "local")
	if err != nil || embedClient == nil {
		t.Fatalf("GetEmbedConnection error: %v", err)
	}
}
