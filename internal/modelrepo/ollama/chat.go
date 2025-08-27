package ollama

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/ollama/ollama/api"
)

type OllamaChatClient struct {
	ollamaClient *api.Client
	modelName    string
	backendURL   string
}

func (c *OllamaChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.Message, error) {
	// Convert messages to Ollama API format
	apiMessages := make([]api.Message, 0, len(messages))
	for _, msg := range messages {
		apiMessages = append(apiMessages, api.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Build configuration from arguments
	config := &modelrepo.ChatConfig{}
	for _, arg := range args {
		arg.Apply(config)
	}

	// Prepare Ollama options
	llamaOptions := make(map[string]any)

	if config.Temperature != nil {
		llamaOptions["temperature"] = *config.Temperature
	}

	if config.MaxTokens != nil {
		llamaOptions["num_predict"] = *config.MaxTokens
	}

	if config.TopP != nil {
		llamaOptions["top_p"] = *config.TopP
	}

	if config.Seed != nil {
		llamaOptions["seed"] = *config.Seed
	}

	think := api.ThinkValue{Value: false}
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

	// Handle the API call
	err := c.ollamaClient.Chat(ctx, req, func(res api.ChatResponse) error {
		content += res.Message.Content
		if res.Done {
			finalResponse = res
		}
		return nil
	})

	if err != nil {
		return modelrepo.Message{}, fmt.Errorf("ollama API chat request failed for model %s: %w", c.modelName, err)
	}

	// Check if we received any response
	if finalResponse.Message.Role == "" {
		return modelrepo.Message{}, fmt.Errorf("no response received from ollama for model %s", c.modelName)
	}

	// Handle completion reasons
	switch finalResponse.DoneReason {
	case "error":
		return modelrepo.Message{}, fmt.Errorf(
			"ollama generation error for model %s: %s",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "length":
		return modelrepo.Message{}, fmt.Errorf(
			"token limit reached for model %s (partial response: %q)",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "stop":
		if finalResponse.Message.Content == "" {
			return modelrepo.Message{}, fmt.Errorf(
				"empty content from model %s despite normal completion",
				c.modelName,
			)
		}
	default:
		return modelrepo.Message{}, fmt.Errorf(
			"unexpected completion reason %q for model %s",
			finalResponse.DoneReason,
			c.modelName,
		)
	}

	// Successful response
	return modelrepo.Message{
		Role:    finalResponse.Message.Role,
		Content: content,
	}, nil
}

var _ modelrepo.LLMChatClient = (*OllamaChatClient)(nil)
