package modeld

import (
	"context"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
)

// ModeldPromptClient executes plain prompts through the native session transport.
type ModeldPromptClient struct {
	modelName       string
	backendID       string
	maxOutputTokens int
	supportsThink   bool
	tracker         libtracker.ActivityTracker
}

// Prompt executes a system instruction and user prompt through the native chat path.
func (c *ModeldPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, *modelrepo.TokenUsage, error) {
	client := &ModeldChatClient{modelName: c.modelName, backendID: c.backendID, maxOutputTokens: c.maxOutputTokens, supportsThink: c.supportsThink, tracker: c.tracker}
	result, err := client.Chat(ctx, []modelrepo.Message{{Role: "system", Content: systemInstruction}, {Role: "user", Content: prompt}}, modelrepo.WithTemperature(float64(temperature)))
	return result.Message.Content, result.Usage, err
}

var _ modelrepo.LLMPromptExecClient = (*ModeldPromptClient)(nil)
