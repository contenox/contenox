package modelrepo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"
)

type OllamaProvider struct {
	Name           string
	ID             string
	ContextLength  int
	SupportsChat   bool
	SupportsEmbed  bool
	SupportsStream bool
	SupportsPrompt bool
	SupportsThink  bool
	httpClient     *http.Client
	Backends       []string
}

func (p *OllamaProvider) GetBackendIDs() []string {
	return p.Backends
}

func (p *OllamaProvider) ModelName() string {
	return p.Name
}

func (p *OllamaProvider) GetID() string {
	return p.ID
}

func (p *OllamaProvider) GetType() string {
	return "ollama"
}

func (p *OllamaProvider) GetContextLength() int {
	return p.ContextLength
}

func (p *OllamaProvider) CanChat() bool {
	return p.SupportsChat
}

func (p *OllamaProvider) CanEmbed() bool {
	return p.SupportsEmbed
}

func (p *OllamaProvider) CanStream() bool {
	return p.SupportsStream
}

func (p *OllamaProvider) CanPrompt() bool {
	return p.SupportsPrompt
}

func (p *OllamaProvider) CanThink() bool {
	return p.SupportsThink
}

func (p *OllamaProvider) GetChatConnection(ctx context.Context, backendID string) (LLMChatClient, error) {
	if !p.CanChat() {
		return nil, fmt.Errorf("provider %s (model %s) does not support chat", p.GetID(), p.ModelName())
	}
	u, err := url.Parse(backendID)
	if err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s' for provider %s: %w", backendID, p.GetID(), err)
	}
	httpClient := p.httpClient
	ollamaAPIClient := api.NewClient(u, httpClient)

	chatClient := &OllamaChatClient{
		ollamaClient: ollamaAPIClient,
		modelName:    p.ModelName(),
		backendURL:   backendID,
	}

	return chatClient, nil
}

func (p *OllamaProvider) GetEmbedConnection(ctx context.Context, backendID string) (LLMEmbedClient, error) {
	if !p.CanEmbed() {
		return nil, fmt.Errorf("provider %s (model %s) does not support embeddings", p.GetID(), p.ModelName())
	}
	u, err := url.Parse(backendID)
	if err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s' for provider %s: %w", backendID, p.GetID(), err)
	}
	httpClient := p.httpClient
	ollamaAPIClient := api.NewClient(u, httpClient)

	embedClient := &OllamaEmbedClient{
		ollamaClient: ollamaAPIClient,
		modelName:    p.ModelName(),
		backendURL:   backendID,
	}

	return embedClient, nil
}

func (p *OllamaProvider) GetPromptConnection(ctx context.Context, backendID string) (LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, fmt.Errorf("provider %s (model %s) does not support prompting", p.GetID(), p.ModelName())
	}
	u, err := url.Parse(backendID)
	if err != nil {
		return nil, fmt.Errorf("invalid backend URL '%s' for provider %s: %w", backendID, p.GetID(), err)
	}
	httpClient := p.httpClient
	ollamaAPIClient := api.NewClient(u, httpClient)

	promptClient := &OllamaPromptClient{
		ollamaClient: ollamaAPIClient,
		modelName:    p.ModelName(),
		backendURL:   backendID,
	}

	return promptClient, nil
}

func (p *OllamaProvider) GetStreamConnection(ctx context.Context, backendID string) (LLMStreamClient, error) {
	return nil, fmt.Errorf("streaming not implemented for Ollama provider")
}

// CapabilityConfig holds the required capability information
type CapabilityConfig struct {
	ContextLength int
	CanChat       bool
	CanEmbed      bool
	CanStream     bool
	CanPrompt     bool
	CanThink      bool
}

// NewOllamaModelProvider creates a provider with explicit capabilities
func NewOllamaModelProvider(name string, backends []string, httpClient *http.Client, caps CapabilityConfig) Provider {
	return &OllamaProvider{
		Name:           name,
		ID:             "ollama:" + name,
		ContextLength:  caps.ContextLength,
		SupportsChat:   caps.CanChat,
		SupportsEmbed:  caps.CanEmbed,
		SupportsStream: caps.CanStream,
		SupportsPrompt: caps.CanPrompt,
		SupportsThink:  caps.CanThink,
		Backends:       backends,
		httpClient:     httpClient,
	}
}
