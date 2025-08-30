package gemini

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/internal/modelrepo"
)

type GeminiChatClient struct {
	geminiClient
}

func (c *GeminiChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	// Extract system instruction from messages
	var systemInstruction *geminiSystemInstruction
	for _, msg := range messages {
		if msg.Role == "system" {
			systemInstruction = &geminiSystemInstruction{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			break
		}
	}

	request := buildGeminiRequest(c.modelName, messages, systemInstruction, args)

	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", c.modelName)
	var response geminiGenerateContentResponse
	if err := c.sendRequest(ctx, endpoint, request, &response); err != nil {
		return modelrepo.ChatResult{}, err
	}

	if len(response.Candidates) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("no candidates returned from Gemini for model %s", c.modelName)
	}

	candidate := response.Candidates[0]
	if len(candidate.Content.Parts) == 0 || candidate.Content.Parts[0].Text == "" {
		if len(candidate.FinishReason) > 0 {
			return modelrepo.ChatResult{}, fmt.Errorf(
				"empty content from model %s despite normal completion. Finish reason: %v",
				c.modelName, candidate.FinishReason,
			)
		}
		return modelrepo.ChatResult{}, fmt.Errorf("empty content from model %s", c.modelName)
	}

	return modelrepo.ChatResult{
		Message:   modelrepo.Message{Role: "assistant", Content: candidate.Content.Parts[0].Text},
		ToolCalls: []modelrepo.ToolCall{},
	}, nil
}

var _ modelrepo.LLMChatClient = (*GeminiChatClient)(nil)
