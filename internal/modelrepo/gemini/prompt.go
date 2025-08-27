package gemini

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
)

type GeminiPromptClient struct {
	geminiClient
}

func (c *GeminiPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	// Convert to chat format for consistency
	messages := []modelrepo.Message{
		{Role: "system", Content: systemInstruction},
		{Role: "user", Content: prompt},
	}

	// Use the chat client to handle the prompt
	chatClient := &GeminiChatClient{geminiClient: c.geminiClient}

	// Convert temperature to float64 and create argument
	tempArg := modelrepo.WithTemperature(float64(temperature))

	response, err := chatClient.Chat(ctx, messages, tempArg)
	if err != nil {
		return "", fmt.Errorf("Gemini prompt execution failed: %w", err)
	}

	return response.Content, nil
}

var _ modelrepo.LLMPromptExecClient = (*GeminiPromptClient)(nil)
