package bedrock

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/modeld"
)

type bedrockChatClient struct{ bedrockClient }

// Chat implements modeld.LLMChatClient via the Bedrock Converse API.
func (c *bedrockChatClient) Chat(ctx context.Context, messages []modeld.Message, args ...modeld.ChatArgument) (modeld.ChatResult, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "bedrock", "model", c.modelName)
	defer end()

	in := buildConverseInput(c.modelName, messages, chatConfigFromArgs(args), c.maxOutputTokens)
	out, err := c.api.Converse(ctx, in)
	if err != nil {
		err = fmt.Errorf("bedrock converse (model=%s): %w", c.modelName, err)
		reportErr(err)
		return modeld.ChatResult{}, err
	}
	res, err := decodeConverse(out)
	if err != nil {
		reportErr(err)
		return modeld.ChatResult{}, err
	}
	reportChange("chat_completed", res)
	return res, nil
}

var _ modeld.LLMChatClient = (*bedrockChatClient)(nil)
