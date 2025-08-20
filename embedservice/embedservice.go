package embedservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/llmrepo"
)

type Service interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	DefaultModelName(ctx context.Context) (string, error)
}

type service struct {
	repo          llmrepo.ModelRepo
	modelName     string
	modelProvider string
}

func New(repo llmrepo.ModelRepo, modelName string, modelProvider string) Service {
	return &service{
		repo:          repo,
		modelName:     modelName,
		modelProvider: modelProvider,
	}
}

// Embed implements Service.
func (s *service) Embed(ctx context.Context, text string) ([]float64, error) {
	embedClient, err := s.repo.Embed(ctx, llmrepo.EmbedRequest{
		ModelName:    s.modelName,
		ProviderType: s.modelProvider,
	})
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

// DefaultModelName implements Service.
func (s *service) DefaultModelName(ctx context.Context) (string, error) {
	return s.modelName, nil
}
