package vllm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
)

type VLLMChatClient struct {
	vLLMClient
}

func NewVLLMChatClient(ctx context.Context, baseURL, modelName string, contextLength int, httpClient *http.Client, apiKey string) (modelrepo.LLMChatClient, error) {
	client := &VLLMChatClient{
		vLLMClient: vLLMClient{
			baseURL:    baseURL,
			httpClient: httpClient,
			modelName:  modelName,
			apiKey:     apiKey,
		},
	}

	client.maxTokens = min(contextLength, 2048)
	return client, nil
}

func (c *VLLMChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	request := buildChatRequest(c.modelName, messages, args)

	var response struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := c.sendRequest(ctx, "/v1/chat/completions", request, &response); err != nil {
		return modelrepo.ChatResult{}, err
	}

	if len(response.Choices) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("no completion choices returned")
	}

	choice := response.Choices[0]

	// Convert to our format
	message := modelrepo.Message{
		Role:    choice.Message.Role,
		Content: choice.Message.Content,
	}

	// Convert tool calls
	var toolCalls []modelrepo.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, modelrepo.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	switch choice.FinishReason {
	case "stop":
		return modelrepo.ChatResult{
			Message:   message,
			ToolCalls: toolCalls,
		}, nil
	case "length":
		return modelrepo.ChatResult{}, fmt.Errorf("token limit reached")
	case "content_filter":
		return modelrepo.ChatResult{}, fmt.Errorf("content filtered")
	default:
		return modelrepo.ChatResult{}, fmt.Errorf("unexpected completion reason: %s", choice.FinishReason)
	}
}
