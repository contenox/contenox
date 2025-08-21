package modelrepo

import (
	"context"
	"fmt"
	"net/http"
)

var _ Provider = (*OpenAIProvider)(nil)

type OpenAIProvider struct {
	id            string
	apiKey        string
	modelName     string
	baseURL       string
	httpClient    *http.Client
	contextLength int
	canChat       bool
	canPrompt     bool
	canEmbed      bool
	canStream     bool
}

func NewOpenAIProvider(apiKey, modelName string, backendURLs []string, capability CapabilityConfig, httpClient *http.Client) *OpenAIProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if len(backendURLs) == 0 {
		backendURLs = []string{"https://api.openai.com/v1"}
	}

	apiBaseURL := backendURLs[0]
	id := fmt.Sprintf("openai-%s", modelName)

	return &OpenAIProvider{
		id:            id,
		apiKey:        apiKey,
		modelName:     modelName,
		baseURL:       apiBaseURL,
		httpClient:    httpClient,
		contextLength: capability.ContextLength,
		canChat:       capability.CanChat,
		canPrompt:     capability.CanPrompt,
		canEmbed:      capability.CanEmbed,
		canStream:     capability.CanStream,
	}
}

func (p *OpenAIProvider) GetBackendIDs() []string {
	return []string{p.baseURL}
}

func (p *OpenAIProvider) ModelName() string {
	return p.modelName
}

func (p *OpenAIProvider) GetID() string {
	return p.id
}

func (p *OpenAIProvider) GetType() string {
	return "openai"
}

func (p *OpenAIProvider) GetContextLength() int {
	return p.contextLength
}

func (p *OpenAIProvider) CanChat() bool {
	return p.canChat
}

func (p *OpenAIProvider) CanEmbed() bool {
	return p.canEmbed
}

func (p *OpenAIProvider) CanStream() bool {
	return p.canStream
}

func (p *OpenAIProvider) CanPrompt() bool {
	return p.canPrompt
}

func (p *OpenAIProvider) CanThink() bool {
	return false
}

func (p *OpenAIProvider) GetChatConnection(ctx context.Context, backendID string) (LLMChatClient, error) {
	if !p.CanChat() {
		return nil, fmt.Errorf("model %s does not support chat interactions", p.modelName)
	}
	return &openAIChatClient{
		openAIClient: openAIClient{
			baseURL:    p.baseURL,
			apiKey:     p.apiKey,
			httpClient: p.httpClient,
			modelName:  p.modelName,
			maxTokens:  p.contextLength,
		},
	}, nil
}

func (p *OpenAIProvider) GetPromptConnection(ctx context.Context, backendID string) (LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, fmt.Errorf("model %s does not support prompt (legacy completion) interactions. Consider using Chat API", p.modelName)
	}
	return &openAIPromptClient{
		openAIClient: openAIClient{
			baseURL:    p.baseURL,
			apiKey:     p.apiKey,
			httpClient: p.httpClient,
			modelName:  p.modelName,
			maxTokens:  p.contextLength,
		},
	}, nil
}

func (p *OpenAIProvider) GetEmbedConnection(ctx context.Context, backendID string) (LLMEmbedClient, error) {
	if !p.CanEmbed() {
		return nil, fmt.Errorf("model %s does not support embedding interactions", p.modelName)
	}
	return &openAIEmbedClient{
		openAIClient: openAIClient{
			baseURL:    p.baseURL,
			apiKey:     p.apiKey,
			httpClient: p.httpClient,
			modelName:  p.modelName,
		},
	}, nil
}

func (p *OpenAIProvider) GetStreamConnection(ctx context.Context, backendID string) (LLMStreamClient, error) {
	if !p.CanStream() {
		return nil, fmt.Errorf("model %s does not support streaming interactions", p.modelName)
	}
	return &openAIStreamClient{
		openAIClient: openAIClient{
			baseURL:    p.baseURL,
			apiKey:     p.apiKey,
			httpClient: p.httpClient,
			modelName:  p.modelName,
			maxTokens:  p.contextLength,
		},
	}, nil
}
