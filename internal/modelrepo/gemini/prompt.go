package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/runtime/internal/modelrepo"
)

type GeminiPromptClient struct {
	geminiClient
}

// Prompt implements the LLMPromptExecClient interface for a single-turn, non-chat request.
func (c *GeminiPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	messages := []modelrepo.Message{
		{Role: "user", Content: prompt},
	}

	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append([]modelrepo.Message{{Role: "system", Content: s}}, messages...)
	}

	chat := &GeminiChatClient{geminiClient: c.geminiClient}
	resp, err := chat.Chat(ctx, messages, modelrepo.WithTemperature(float64(temperature)))
	if err != nil {
		return "", fmt.Errorf("Gemini prompt execution failed: %w", err)
	}

	return resp.Message.Content, nil
}

var _ modelrepo.LLMPromptExecClient = (*GeminiPromptClient)(nil)
