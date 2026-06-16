package bedrock

import (
	"context"
	"strings"

	"github.com/contenox/runtime/modeld"
)

type bedrockPromptClient struct{ bedrockClient }

// Prompt implements modeld.LLMPromptExecClient by wrapping Chat.
func (c *bedrockPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	msgs := []modeld.Message{{Role: "user", Content: prompt}}
	if s := strings.TrimSpace(systemInstruction); s != "" {
		msgs = append([]modeld.Message{{Role: "system", Content: s}}, msgs...)
	}
	chat := &bedrockChatClient{c.bedrockClient}
	res, err := chat.Chat(ctx, msgs, modeld.WithTemperature(float64(temperature)))
	if err != nil {
		return "", err
	}
	return res.Message.Content, nil
}

var _ modeld.LLMPromptExecClient = (*bedrockPromptClient)(nil)
