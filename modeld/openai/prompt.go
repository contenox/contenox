package openai

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/modeld"
)

type OpenAIPromptClient struct {
	openAIClient
}

func (c *OpenAIPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	// Start tracking the operation
	reportErr, reportChange, end := c.tracker.Start(ctx, "prompt", "openai", "model", c.modelName)
	defer end()

	// Convert to chat format for consistency
	messages := []modeld.Message{
		{Role: "system", Content: systemInstruction},
		{Role: "user", Content: prompt},
	}

	// Use the chat client to handle the prompt. Keep the provider-specific
	// parameter rules internal: legacy GPT-5 chat completions reject sampling
	// params, while newer GPT-5.x snapshots may allow them in `reasoning=none`.
	var args []modeld.ChatArgument
	if !openAIShouldOmitSamplingParams(c.modelName, "") {
		args = append(args, modeld.WithTemperature(float64(temperature)))
	}

	response, err := c.Chat(ctx, messages, args...)
	if err != nil {
		reportErr(err)
		return "", fmt.Errorf("OpenAI prompt execution failed: %w", err)
	}

	reportChange("prompt_completed", map[string]any{
		"response_length": len(response.Content),
	})
	return response.Content, nil
}

func (c *OpenAIPromptClient) Chat(ctx context.Context, messages []modeld.Message, args ...modeld.ChatArgument) (modeld.Message, error) {
	chatClient := &OpenAIChatClient{openAIClient: c.openAIClient}
	resp, err := chatClient.Chat(ctx, messages, args...)
	if err != nil {
		return modeld.Message{}, fmt.Errorf("OpenAI chat execution failed: %w", err)
	}

	return resp.Message, nil
}

var _ modeld.LLMPromptExecClient = (*OpenAIPromptClient)(nil)
