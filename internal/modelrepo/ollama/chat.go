package ollama

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/ollama/ollama/api"
)

type OllamaChatClient struct {
	ollamaClient *api.Client
	modelName    string
	backendURL   string
}

func (c *OllamaChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (*modelrepo.ChatResult, error) {
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

	// Convert modelrepo tools to api tools
	var apiTools api.Tools
	if len(config.Tools) > 0 {
		apiTools = make(api.Tools, len(config.Tools))
		for i, tool := range config.Tools {
			// Convert parameters to the expected Ollama format
			var params struct {
				Type       string                      `json:"type"`
				Defs       any                         `json:"$defs,omitempty"`
				Items      any                         `json:"items,omitempty"`
				Required   []string                    `json:"required"`
				Properties map[string]api.ToolProperty `json:"properties"`
			}

			// Try to convert the interface{} parameters to the expected format
			if tool.Function.Parameters != nil {
				paramsData, err := json.Marshal(tool.Function.Parameters)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal tool parameters: %w", err)
				}

				if err := json.Unmarshal(paramsData, &params); err != nil {
					return nil, fmt.Errorf("failed to unmarshal tool parameters: %w", err)
				}
			}

			apiTools[i] = api.Tool{
				Type: tool.Type,
				Function: api.ToolFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  params,
				},
			}
		}
	}

	req := &api.ChatRequest{
		Model:    c.modelName,
		Messages: apiMessages,
		Stream:   &stream,
		Think:    &think,
		Options:  llamaOptions,
		Tools:    apiTools,
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
		return nil, fmt.Errorf("ollama API chat request failed for model %s: %w", c.modelName, err)
	}

	// Check if we received any response
	if finalResponse.Message.Role == "" {
		return nil, fmt.Errorf("no response received from ollama for model %s", c.modelName)
	}

	// Handle completion reasons
	switch finalResponse.DoneReason {
	case "error":
		return nil, fmt.Errorf(
			"ollama generation error for model %s: %s",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "length":
		return nil, fmt.Errorf(
			"token limit reached for model %s (partial response: %q)",
			c.modelName,
			finalResponse.Message.Content,
		)
	case "stop":
		if finalResponse.Message.Content == "" && len(finalResponse.Message.ToolCalls) == 0 {
			return nil, fmt.Errorf(
				"empty content from model %s despite normal completion",
				c.modelName,
			)
		}
	default:
		return nil, fmt.Errorf(
			"unexpected completion reason %q for model %s",
			finalResponse.DoneReason,
			c.modelName,
		)
	}

	// Convert the response to our format
	message := modelrepo.Message{
		Role:    finalResponse.Message.Role,
		Content: finalResponse.Message.Content,
	}

	// Convert tool calls
	var toolCalls []modelrepo.ToolCall
	for _, tc := range finalResponse.Message.ToolCalls {
		// Convert arguments from map to JSON string
		argsBytes, err := json.Marshal(tc.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool call arguments: %w", err)
		}

		toolCalls = append(toolCalls, modelrepo.ToolCall{
			ID:   "", // Ollama doesn't provide IDs for tool calls
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: string(argsBytes),
			},
		})
	}

	return &modelrepo.ChatResult{
		Message:   message,
		ToolCalls: toolCalls,
	}, nil
}

var _ modelrepo.LLMChatClient = (*OllamaChatClient)(nil)
