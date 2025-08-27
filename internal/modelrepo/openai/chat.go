package openai

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
)

type OpenAIChatClient struct {
	openAIClient
}

func (c *OpenAIChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.Message, error) {
	request := buildOpenAIRequest(c.modelName, messages, args)

	var response struct {
		Choices []struct {
			Index        int               `json:"index"`
			Message      modelrepo.Message `json:"message"`
			FinishReason string            `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := c.sendRequest(ctx, "/chat/completions", request, &response); err != nil {
		return modelrepo.Message{}, err
	}

	if len(response.Choices) == 0 {
		return modelrepo.Message{}, fmt.Errorf("no chat completion choices returned from OpenAI for model %s", c.modelName)
	}

	choice := response.Choices[0]
	if choice.Message.Content == "" {
		return modelrepo.Message{}, fmt.Errorf("empty content from model %s despite normal completion. Finish reason: %s", c.modelName, choice.FinishReason)
	}

	return choice.Message, nil
}

var _ modelrepo.LLMChatClient = (*OpenAIChatClient)(nil)
