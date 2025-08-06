package embedservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/llmrepo"
	"github.com/contenox/runtime/llmresolver"
)

type Service interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

type service struct {
	repo llmrepo.ModelRepo
}

func New(repo llmrepo.ModelRepo) Service {
	return &service{
		repo: repo,
	}
}

// Embed implements Service.
func (s *service) Embed(ctx context.Context, text string) ([]float64, error) {
	embedProvider, err := s.repo.GetDefaultSystemProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedder provider: %w", err)
	}
	embedClient, err := llmresolver.Embed(ctx, llmresolver.EmbedRequest{
		ModelName: embedProvider.ModelName(),
	}, s.repo.GetRuntime(ctx), llmresolver.Randomly)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embed client: %w", err)
	}
	if embedClient == nil {
		return nil, errors.New("embed client is nil")
	}
	vectorData, err := embedClient.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	return vectorData, nil
}
