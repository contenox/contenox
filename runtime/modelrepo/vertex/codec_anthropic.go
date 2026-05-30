package vertex

import (
	"context"

	"github.com/contenox/agent/runtime/modelrepo"
	msgcodec "github.com/contenox/agent/runtime/modelrepo/codec/messages"
)

// anthropicVertexVersion is the required body field selecting Anthropic's
// Vertex-hosted API version (sent in the body, not a header, on Vertex).
const anthropicVertexVersion = "vertex-2023-10-16"

// chatAnthropic handles Claude on Vertex via the Anthropic Messages codec over
// the :rawPredict endpoint. The model lives in the URL, so Request.Model stays
// empty; the version goes in the body.
func (c *vertexChatClient) chatAnthropic(ctx context.Context, messages []modelrepo.Message, args []modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "vertex", "model", c.modelName, "publisher", c.publisher)
	defer end()

	cfg := chatConfigFromArgs(args)
	req := msgcodec.Build(messages, cfg)
	req.AnthropicVersion = anthropicVertexVersion

	raw, err := c.postJSON(ctx, c.anthropicURL(false), req)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	res, err := msgcodec.DecodeResponse(raw)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	reportChange("chat_completed", res)
	return res, nil
}

func (c *vertexStreamClient) streamAnthropic(ctx context.Context, messages []modelrepo.Message, args []modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := chatConfigFromArgs(args)
	req := msgcodec.Build(messages, cfg)
	req.AnthropicVersion = anthropicVertexVersion
	req.Stream = true
	return c.streamSSE(ctx, c.anthropicURL(true), req, msgcodec.NewStreamDecoder())
}
