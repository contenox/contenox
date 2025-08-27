package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
)

// vLLMPromptClient handles prompt execution
type vLLMPromptClient struct {
	vLLMClient
}

// vLLMChatClient handles chat interaction
type vLLMChatClient struct {
	vLLMClient
}

type vLLMClient struct {
	baseURL    string
	httpClient *http.Client
	modelName  string
	maxTokens  int
	apiKey     string
}

type chatRequest struct {
	Model       string              `json:"model"`
	Messages    []modelrepo.Message `json:"messages"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	Seed        *int                `json:"seed,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
}

func (c *vLLMClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	url := c.baseURL + endpoint
	reqBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vLLM API returned non-200 status: %d for model %s", resp.StatusCode, c.modelName)
	}

	return json.NewDecoder(resp.Body).Decode(response)
}

func buildChatRequest(modelName string, messages []modelrepo.Message, args []modelrepo.ChatArgument) chatRequest {
	config := &modelrepo.ChatConfig{}
	for _, arg := range args {
		arg.Apply(config)
	}

	return chatRequest{
		Model:       modelName,
		Messages:    messages,
		Temperature: config.Temperature,
		MaxTokens:   config.MaxTokens,
		TopP:        config.TopP,
		Seed:        config.Seed,
		Stream:      false,
	}
}
