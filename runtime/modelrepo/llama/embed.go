package llama

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// embedClient implements modelrepo.LLMEmbedClient as a one-shot path (no
// session/KV reuse); embeddings are non-causal and process the whole input in a
// single batch. It is dual-mode, mirroring chat's newSession: a registered
// native backend (tests / CGO builds) wins, otherwise the embedding runs on the
// modeld daemon over runtime/transport. Before this it had no daemon arm, so the
// shipped pure-Go CLI reported llama embeddings as unavailable while chat worked.
type embedClient struct {
	modelName   string
	modelPath   string
	modelDigest string
	cfg         Config
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	if embedFunc != nil {
		return newEmbed(ctx, c.modelPath, c.cfg, prompt)
	}
	res, err := modeldconn.Embed(ctx, modeldconn.ModelRef{
		Name:   c.modelName,
		Type:   "llama",
		Digest: c.modelDigest,
		Path:   c.modelPath,
	}, transport.Config(c.cfg), prompt)
	if err != nil {
		// Preserve the ErrSessionUnavailable contract callers branch on, while
		// keeping the actionable modeld detail (not installed / unreachable / ...).
		return nil, fmt.Errorf("%w: %v", ErrSessionUnavailable, err)
	}
	out := make([]float64, len(res.Vector))
	for i, v := range res.Vector {
		out[i] = float64(v)
	}
	return out, nil
}

var _ modelrepo.LLMEmbedClient = (*embedClient)(nil)
