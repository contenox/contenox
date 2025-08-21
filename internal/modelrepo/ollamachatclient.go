package modelrepo

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

type OllamaChatClient struct {
	ollamaClient *api.Client // The underlying Ollama API client
	modelName    string      // The specific model this client targets (e.g., "llama3:latest")
	backendURL   string      // backend URL
}

// Adapter so ChatOption can modify Ollama chat requests
type ollamaChatRequestAdapter struct {
	temperature float64
	maxTokens   int
}

func (a *ollamaChatRequestAdapter) SetTemperature(temp float64) {
	a.temperature = temp
}

func (a *ollamaChatRequestAdapter) SetMaxTokens(max int) {
	a.maxTokens = max
}

var _ LLMChatClient = (*OllamaChatClient)(nil)

func (c *OllamaChatClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (Message, error) {
	apiMessages := make([]api.Message, 0, len(messages))
	for _, msg := range messages {
		apiMessages = append(apiMessages, api.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Default values
	defaultTemperature := 0.7
	// Create adapter with default values
	adapter := &ollamaChatRequestAdapter{
		temperature: defaultTemperature,
		maxTokens:   0,
	}

	// Apply ChatOptions using the standard pattern
	for _, opt := range options {
		if opt != nil {
			// First pass: let options observe current values
			opt.SetTemperature(adapter.temperature)
			opt.SetMaxTokens(adapter.maxTokens)
			// Second pass: let options set new values
			opt.SetTemperature(adapter.temperature)
			opt.SetMaxTokens(adapter.maxTokens)
		}
	}
	llamaOptions := map[string]any{
		"temperature": adapter.temperature,
	}

	if adapter.maxTokens > 0 {
		llamaOptions["num_predict"] = adapter.maxTokens
	}

	think := api.ThinkValue{
		Value: false,
	}
	stream := false
	req := &api.ChatRequest{
		Model:    c.modelName,
		Messages: apiMessages,
		Stream:   &stream,
		Think:    &think,
		Options:  llamaOptions,
	}

	var finalResponse api.ChatResponse
	var content string

	// Handle the API call first
	err := c.ollamaClient.Chat(ctx, req, func(res api.ChatResponse) error {
		content += res.Message.Content
		// For non-streaming, we expect exactly one response with Done=true
		if res.Done {
			finalResponse = res
		}
		return nil
	})
	// Check for API-level errors first (network issues, etc.)
	if err != nil {
		return Message{}, fmt.Errorf("ollama API chat request failed for model %s: %w", c.modelName, err)
	}

	// Check if we received any response at all
	if finalResponse.Message.Role == "" {
		return Message{}, fmt.Errorf("no response received from ollama for model %s", c.modelName)
	}

	// Handle completion reasons
	switch finalResponse.DoneReason {
	case "error":
		// Server-side error during generation
		return Message{}, fmt.Errorf(
			"ollama generation error for model %s: %s",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "length":
		// Treat token limit hits as application errors
		return Message{}, fmt.Errorf(
			"token limit reached for model %s (partial response: %q)",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "stop":
		// Normal completion, but ensure content exists
		if finalResponse.Message.Content == "" {
			return Message{}, fmt.Errorf(
				"empty content from model %s despite normal completion",
				c.modelName,
			)
		}
	default:
		// Unknown completion reason
		return Message{}, fmt.Errorf(
			"unexpected completion reason %q for model %s",
			finalResponse.DoneReason,
			c.modelName,
		)
	}

	// Successful response
	return Message{
		Role:    finalResponse.Message.Role,
		Content: content,
	}, nil
}
