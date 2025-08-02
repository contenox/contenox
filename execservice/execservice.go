package execservice

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/modelprovider/llmresolver"
	"github.com/contenox/runtime/llmrepo"
	"github.com/google/uuid"
)

type ExecService interface {
	Execute(ctx context.Context, request *TaskRequest) (*TaskResponse, error)
}

type execService struct {
	promptRepo llmrepo.ModelRepo
	db         libdb.DBManager
}

func NewExec(ctx context.Context, promptRepo llmrepo.ModelRepo, dbInstance libdb.DBManager) ExecService {
	return &execService{
		promptRepo: promptRepo,
		db:         dbInstance,
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
	provider, err := s.promptRepo.GetDefaultSystemProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	promptClient, err := llmresolver.PromptExecute(ctx, llmresolver.PromptRequest{
		ModelNames: []string{provider.ModelName()},
	}, s.promptRepo.GetRuntime(ctx), llmresolver.Randomly)
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
