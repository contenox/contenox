package vllm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
)

type VLLMChatClient struct {
	vLLMClient
}

func NewVLLMChatClient(ctx context.Context, baseURL, modelName string, contextLength, maxOutputTokens int, httpClient *http.Client, apiKey string, canThink bool, tracker libtracker.ActivityTracker) (modeld.LLMChatClient, error) {
	client := &VLLMChatClient{
		vLLMClient: vLLMClient{
			baseURL:         baseURL,
			httpClient:      httpClient,
			modelName:       modelName,
			maxOutputTokens: maxOutputTokens,
			canThink:        canThink,
			apiKey:          apiKey,
			tracker:         tracker,
		},
	}

	client.maxTokens = min(contextLength, 2048)
	return client, nil
}

func (c *VLLMChatClient) Chat(ctx context.Context, messages []modeld.Message, args ...modeld.ChatArgument) (modeld.ChatResult, error) {
	// Start tracking the operation
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "vllm", "model", c.modelName)
	defer end()

	request := buildChatRequest(c.modelName, messages, args, c.canThink)
	c.clampChatRequest(&request)

	var response chatResponse

	if err := c.sendRequest(ctx, "/v1/chat/completions", request, &response); err != nil {
		reportErr(err)
		return modeld.ChatResult{}, err
	}

	if len(response.Choices) == 0 {
		err := fmt.Errorf("no completion choices returned")
		reportErr(err)
		return modeld.ChatResult{}, err
	}

	choice := response.Choices[0]

	// Convert to our format
	message := modeld.Message{
		Role:     choice.Message.Role,
		Content:  choice.Message.Content,
		Thinking: choice.Message.Thinking(),
	}

	// Convert tool calls
	toolCalls := convertChatToolCalls(choice.Message.ToolCalls)

	result := modeld.ChatResult{
		Message:   message,
		ToolCalls: toolCalls,
	}

	switch choice.FinishReason {
	case "stop", "tool_calls":
		reportChange("chat_completed", map[string]any{
			"finish_reason":    choice.FinishReason,
			"content_length":   len(message.Content),
			"thinking_length":  len(message.Thinking),
			"tool_calls_count": len(toolCalls),
		})
		return result, nil
	case "length":
		err := fmt.Errorf("token limit reached")
		reportErr(err)
		return modeld.ChatResult{}, err
	case "content_filter":
		err := fmt.Errorf("content filtered")
		reportErr(err)
		return modeld.ChatResult{}, err
	default:
		err := fmt.Errorf("unexpected completion reason: %s", choice.FinishReason)
		reportErr(err)
		return modeld.ChatResult{}, err
	}
}

var _ modeld.LLMChatClient = (*VLLMChatClient)(nil)
