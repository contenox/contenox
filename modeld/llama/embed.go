package llama

import (
	"context"

	"github.com/contenox/runtime/modeld"
)

// embedClient implements modeld.LLMEmbedClient via the native backend. It is a
// one-shot path (no session/KV reuse); embeddings are non-causal and process the
// whole input in a single batch.
type embedClient struct {
	modelPath string
	cfg       Config
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	return newEmbed(ctx, c.modelPath, c.cfg, prompt)
}

var _ modeld.LLMEmbedClient = (*embedClient)(nil)
