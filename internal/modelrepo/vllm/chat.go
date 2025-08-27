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

func (c *VLLMChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.Message, error) {
	request := buildChatRequest(c.modelName, messages, args)

	var response struct {
		Choices []struct {
			Message      modelrepo.Message `json:"message"`
			FinishReason string            `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := c.sendRequest(ctx, "/v1/chat/completions", request, &response); err != nil {
		return modelrepo.Message{}, err
	}

	if len(response.Choices) == 0 {
		return modelrepo.Message{}, fmt.Errorf("no completion choices returned")
	}

	choice := response.Choices[0]
	switch choice.FinishReason {
	case "stop":
		return choice.Message, nil
	case "length":
		return modelrepo.Message{}, fmt.Errorf("token limit reached")
	case "content_filter":
		return modelrepo.Message{}, fmt.Errorf("content filtered")
	default:
		return modelrepo.Message{}, fmt.Errorf("unexpected completion reason: %s", choice.FinishReason)
	}
}
