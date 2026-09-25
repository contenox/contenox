package modeld

import (
	"context"
	"fmt"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/transport"
	"github.com/contenox/contenox/libtracker"
)

// ModeldEmbedClient obtains vectors from a native daemon.
type ModeldEmbedClient struct {
	modelName string
	backendID string
	tracker   libtracker.ActivityTracker
}

func (c *ModeldEmbedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	reportErr, _, end := c.tracker.Start(ctx, "embed", "modeld", "model", c.modelName)
	defer end()

	target := resolveTarget(c.backendID)
	tConfig := transport.Config{}

	res, err := embedWithAutoPull(ctx, target, c.modelName, tConfig, prompt, c.tracker)
	if err != nil {
		reportErr(err)
		return nil, fmt.Errorf("modeld embed for %s: %w", c.modelName, err)
	}

	out := make([]float64, len(res.Vector))
	for i, v := range res.Vector {
		out[i] = float64(v)
	}
	return out, nil
}

var _ modelrepo.LLMEmbedClient = (*ModeldEmbedClient)(nil)
