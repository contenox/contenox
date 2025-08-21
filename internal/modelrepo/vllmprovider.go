package modelrepo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// vLLMProvider implements the Provider interface for vLLM models
type vLLMProvider struct {
	Name           string
	ID             string
	ContextLength  int
	SupportsChat   bool
	SupportsEmbed  bool
	SupportsStream bool
	SupportsPrompt bool
	Backends       []string // Base URLs to vLLM instances (e.g., "http://vllm-server:8000")
	authToken      string
	client         *http.Client
}

// NewVLLMModelProvider creates a new vLLM model provider with explicit capabilities
func NewVLLMModelProvider(modelName string, backends []string, client *http.Client, caps CapabilityConfig, authToken string) Provider {
	return &vLLMProvider{
		Name:           modelName,
		ID:             "vllm:" + modelName,
		ContextLength:  caps.ContextLength,
		SupportsChat:   caps.CanChat,
		SupportsEmbed:  caps.CanEmbed,
		SupportsStream: caps.CanStream,
		SupportsPrompt: caps.CanPrompt,
		Backends:       backends,
		authToken:      authToken,
		client:         client,
	}
}

func (p *vLLMProvider) GetBackendIDs() []string {
	return p.Backends
}

func (p *vLLMProvider) ModelName() string {
	return p.Name
}

func (p *vLLMProvider) GetID() string {
	return p.ID
}

func (p *vLLMProvider) GetType() string {
	return "vllm"
}

func (p *vLLMProvider) GetContextLength() int {
	return p.ContextLength
}

func (p *vLLMProvider) CanChat() bool {
	return p.SupportsChat
}

func (p *vLLMProvider) CanEmbed() bool {
	return p.SupportsEmbed
}

func (p *vLLMProvider) CanStream() bool {
	return p.SupportsStream
}

func (p *vLLMProvider) CanPrompt() bool {
	return p.SupportsPrompt
}

func (p *vLLMProvider) CanThink() bool {
	return false
}

func (p *vLLMProvider) GetChatConnection(ctx context.Context, backendID string) (LLMChatClient, error) {
	if !p.CanChat() {
		return nil, fmt.Errorf("provider %s (model %s) does not support chat", p.GetID(), p.ModelName())
	}

	// Validate backend URL
	if _, err := url.Parse(backendID); err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s': %w", backendID, err)
	}

	return NewVLLMChatClient(ctx, backendID, p.ModelName(), p.ContextLength, p.client, p.authToken)
}

func (p *vLLMProvider) GetPromptConnection(ctx context.Context, backendID string) (LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, fmt.Errorf("provider %s (model %s) does not support prompting", p.GetID(), p.ModelName())
	}

	// Validate backend URL
	if _, err := url.Parse(backendID); err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s': %w", backendID, err)
	}

	return NewVLLMPromptClient(ctx, backendID, p.ModelName(), p.ContextLength, p.client, p.authToken)
}

func (p *vLLMProvider) GetEmbedConnection(ctx context.Context, backendID string) (LLMEmbedClient, error) {
	return nil, fmt.Errorf("provider %s (model %s) does not support embeddings", p.GetID(), p.ModelName())
}

func (p *vLLMProvider) GetStreamConnection(ctx context.Context, backendID string) (LLMStreamClient, error) {
	if !p.CanStream() {
		return nil, fmt.Errorf("provider %s (model %s) does not support streaming", p.GetID(), p.ModelName())
	}

	// Validate backend URL
	if _, err := url.Parse(backendID); err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s': %w", backendID, err)
	}

	return NewVLLMStreamClient(ctx, backendID, p.ModelName(), p.ContextLength, p.client, p.authToken)
}
