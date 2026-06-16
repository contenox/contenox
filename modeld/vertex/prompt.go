package vertex

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/runtime/modeld"
)

type vertexPromptClient struct {
	vertexClient
}

// Prompt implements modeld.LLMPromptExecClient.
func (c *vertexPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "prompt", "vertex", "model", c.modelName)
	defer end()

	messages := []modeld.Message{
		{Role: "user", Content: prompt},
	}
	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append([]modeld.Message{{Role: "system", Content: s}}, messages...)
	}

	chat := &vertexChatClient{vertexClient: c.vertexClient}
	resp, err := chat.Chat(ctx, messages, modeld.WithTemperature(float64(temperature)))
	if err != nil {
		reportErr(err)
		return "", fmt.Errorf("vertex prompt execution failed: %w", err)
	}

	reportChange("prompt_completed", map[string]any{
		"response_length": len(resp.Message.Content),
	})
	return resp.Message.Content, nil
}

var _ modeld.LLMPromptExecClient = (*vertexPromptClient)(nil)
