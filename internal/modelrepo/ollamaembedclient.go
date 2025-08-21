package modelrepo

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

type OllamaEmbedClient struct {
	ollamaClient *api.Client
	modelName    string
	backendURL   string
}

func (c *OllamaEmbedClient) Embed(ctx context.Context, text string) ([]float64, error) {
	req := &api.EmbeddingRequest{
		Model:  c.modelName,
		Prompt: text,
	}

	resp, err := c.ollamaClient.Embeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	return resp.Embedding, nil
}
