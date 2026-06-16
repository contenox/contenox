package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/runtime/modeld"
)

type GeminiPromptClient struct {
	geminiClient
}

// Prompt implements the LLMPromptExecClient interface for a single-turn, non-chat request.
func (c *GeminiPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	// Start tracking the operation
	reportErr, reportChange, end := c.tracker.Start(ctx, "prompt", "gemini", "model", c.modelName)
	defer end()

	messages := []modeld.Message{
		{Role: "user", Content: prompt},
	}

	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append([]modeld.Message{{Role: "system", Content: s}}, messages...)
	}

	chat := &GeminiChatClient{geminiClient: c.geminiClient}
	resp, err := chat.Chat(ctx, messages, modeld.WithTemperature(float64(temperature)))
	if err != nil {
		reportErr(err)
		return "", fmt.Errorf("Gemini prompt execution failed: %w", err)
	}

	reportChange("prompt_completed", map[string]any{
		"response_length": len(resp.Message.Content),
	})
	return resp.Message.Content, nil
}

var _ modeld.LLMPromptExecClient = (*GeminiPromptClient)(nil)
