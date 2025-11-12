package openai

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
)

type OpenAIChatClient struct {
	openAIClient
}

func (c *OpenAIChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	req, nameMap := buildOpenAIRequest(c.modelName, messages, args)

	var response struct {
		Choices []struct {
			Index   int `json:"index"`
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

	if err := c.sendRequest(ctx, "/chat/completions", req, &response); err != nil {
		return modelrepo.ChatResult{}, err
	}

	if len(response.Choices) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("no chat completion choices returned from OpenAI for model %s", c.modelName)
	}

	choice := response.Choices[0]
	if choice.Message.Content == "" && len(choice.Message.ToolCalls) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("empty content from model %s despite normal completion. Finish reason: %s", c.modelName, choice.FinishReason)
	}

	// Convert to our format
	message := modelrepo.Message{
		Role:    choice.Message.Role,
		Content: choice.Message.Content,
	}

	// Convert tool calls and translate sanitized names back to the original the caller provided
	var toolCalls []modelrepo.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		name := tc.Function.Name
		if orig, ok := nameMap[name]; ok && orig != "" {
			name = orig
		}
		toolCalls = append(toolCalls, modelrepo.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return modelrepo.ChatResult{
		Message:   message,
		ToolCalls: toolCalls,
	}, nil
}

var _ modelrepo.LLMChatClient = (*OpenAIChatClient)(nil)
