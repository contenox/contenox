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
	target      modeldconn.ModeldTarget
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	ref := modeldconn.ModelRef{
		Name:   c.modelName,
		Type:   "llama",
		Digest: c.modelDigest,
		Path:   c.modelPath,
	}
	// An explicit target (remote or otherwise specific modeld node) always wins:
	// the whole point of a target is to run on that node, not whatever the local
	// process happens to have compiled in or leased. Only the untargeted case
	// falls back to the dual-mode local-CGO/local-lease behavior below.
	if c.target.Endpoint != "" {
		res, err := modeldconn.EmbedTarget(ctx, c.target, ref, transport.Config(c.cfg), prompt)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSessionUnavailable, err)
		}
		return toFloat64Vector(res.Vector), nil
	}
	if embedFunc != nil {
		return newEmbed(ctx, c.modelPath, c.cfg, prompt)
	}
	res, err := modeldconn.Embed(ctx, ref, transport.Config(c.cfg), prompt)
	if err != nil {
		// Preserve the ErrSessionUnavailable contract callers branch on, while
		// keeping the actionable modeld detail (not installed / unreachable / ...).
		return nil, fmt.Errorf("%w: %v", ErrSessionUnavailable, err)
	}
	return toFloat64Vector(res.Vector), nil
}

func toFloat64Vector(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

var _ modelrepo.LLMEmbedClient = (*embedClient)(nil)
