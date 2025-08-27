package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
)

type GeminiProvider struct {
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

func NewGeminiProvider(apiKey string, modelName string, baseURLs []string, cap modelrepo.CapabilityConfig, httpClient *http.Client) modelrepo.Provider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if len(baseURLs) == 0 {
		baseURLs = []string{"https://generativelanguage.googleapis.com"}
	}

	apiBaseURL := baseURLs[0]
	id := fmt.Sprintf("gemini-%s", modelName)

	return &GeminiProvider{
		id:            id,
		apiKey:        apiKey,
		modelName:     modelName,
		baseURL:       apiBaseURL,
		httpClient:    httpClient,
		contextLength: cap.ContextLength,
		canChat:       cap.CanChat,
		canPrompt:     cap.CanPrompt,
		canEmbed:      cap.CanEmbed,
		canStream:     cap.CanStream,
	}
}

func (p *GeminiProvider) GetBackendIDs() []string {
	return []string{p.baseURL}
}

func (p *GeminiProvider) ModelName() string {
	return p.modelName
}

func (p *GeminiProvider) GetID() string {
	return p.id
}

func (p *GeminiProvider) GetType() string {
	return "gemini"
}

func (p *GeminiProvider) GetContextLength() int {
	return p.contextLength
}

func (p *GeminiProvider) CanChat() bool {
	return p.canChat
}

func (p *GeminiProvider) CanEmbed() bool {
	return p.canEmbed
}

func (p *GeminiProvider) CanStream() bool {
	return p.canStream
}

func (p *GeminiProvider) CanPrompt() bool {
	return p.canPrompt
}

func (p *GeminiProvider) CanThink() bool {
	return false
}

func (p *GeminiProvider) GetChatConnection(ctx context.Context, backendID string) (modelrepo.LLMChatClient, error) {
	if !p.CanChat() {
		return nil, fmt.Errorf("model %s does not support chat interactions", p.modelName)
	}
	return &GeminiChatClient{
		geminiClient: geminiClient{
			modelName:  p.modelName,
			baseURL:    p.baseURL,
			httpClient: p.httpClient,
			maxTokens:  p.contextLength,
			apiKey:     p.apiKey,
		},
	}, nil
}

func (p *GeminiProvider) GetPromptConnection(ctx context.Context, backendID string) (modelrepo.LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, fmt.Errorf("model %s does not support prompt interactions", p.modelName)
	}
	return &GeminiPromptClient{
		geminiClient: geminiClient{
			modelName:  p.modelName,
			baseURL:    p.baseURL,
			httpClient: p.httpClient,
			maxTokens:  p.contextLength,
			apiKey:     p.apiKey,
		},
	}, nil
}

func (p *GeminiProvider) GetEmbedConnection(ctx context.Context, backendID string) (modelrepo.LLMEmbedClient, error) {
	if !p.CanEmbed() {
		return nil, fmt.Errorf("model %s does not support embedding interactions", p.modelName)
	}
	return &GeminiEmbedClient{
		geminiClient: geminiClient{
			modelName:  p.modelName,
			baseURL:    p.baseURL,
			httpClient: p.httpClient,
			apiKey:     p.apiKey,
		},
	}, nil
}

func (p *GeminiProvider) GetStreamConnection(ctx context.Context, backendID string) (modelrepo.LLMStreamClient, error) {
	if !p.CanStream() {
		return nil, fmt.Errorf("model %s does not support streaming interactions", p.modelName)
	}
	return &GeminiStreamClient{
		geminiClient: geminiClient{
			modelName:  p.modelName,
			baseURL:    p.baseURL,
			httpClient: p.httpClient,
			maxTokens:  p.contextLength,
			apiKey:     p.apiKey,
		},
	}, nil
}

// Helper function to fetch model info (if needed)
func fetchGeminiModelInfo(ctx context.Context, baseURL, apiKey, modelName string, httpClient *http.Client) (*modelInfo, error) {
	url := fmt.Sprintf("%s/v1beta/models/%s", baseURL, modelName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Goog-Api-Key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned error (%d): %s", resp.StatusCode, string(body))
	}

	var modelResponse struct {
		Name                       string   `json:"name"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Determine capabilities from API response
	canChat := false
	canPrompt := false
	canEmbed := false
	canStream := false

	for _, method := range modelResponse.SupportedGenerationMethods {
		switch method {
		case "generateContent":
			canChat = true
			canPrompt = true
			canStream = true
		case "embedContent":
			canEmbed = true
		}
	}

	return &modelInfo{
		contextLength: modelResponse.InputTokenLimit,
		canChat:       canChat,
		canPrompt:     canPrompt,
		canEmbed:      canEmbed,
		canStream:     canStream,
	}, nil
}

type modelInfo struct {
	contextLength int
	canChat       bool
	canPrompt     bool
	canEmbed      bool
	canStream     bool
}
