package execservice

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/llmrepo"
	"github.com/contenox/runtime/llmresolver"
	"github.com/google/uuid"
)

type ExecService interface {
	Execute(ctx context.Context, request *TaskRequest) (*TaskResponse, error)
}

type execService struct {
	modelRepo llmrepo.ModelRepo
	db        libdb.DBManager
}

func NewExec(ctx context.Context, modelRepo llmrepo.ModelRepo, dbInstance libdb.DBManager) ExecService {
	return &execService{
		modelRepo: modelRepo,
		db:        dbInstance,
	}
}

type TaskRequest struct {
	Prompt string `json:"prompt"`
}

type TaskResponse struct {
	ID       string `json:"id"`
	Response string `json:"response"`
}

func (s *execService) Execute(ctx context.Context, request *TaskRequest) (*TaskResponse, error) {
	if request == nil {
		return nil, apiframework.ErrEmptyRequest
	}
	if request.Prompt == "" {
		return nil, fmt.Errorf("prompt is empty %w", apiframework.ErrEmptyRequestBody)
	}

	provider, err := s.modelRepo.GetDefaultSystemProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get default system provider: %w", err)
	}
	promptClient, err := llmresolver.PromptExecute(ctx, llmresolver.PromptRequest{
		ModelNames: []string{provider.ModelName()},
	}, s.modelRepo.GetRuntime(ctx), llmresolver.Randomly)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve prompt client: %w", err)
	}

	if promptClient == nil {
		return nil, errors.New("prompt client is nil")
	}

	response, err := promptClient.Prompt(ctx, request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to execute prompt: %w", err)
	}

	return &TaskResponse{
		ID:       uuid.NewString(),
		Response: response,
	}, nil
}
