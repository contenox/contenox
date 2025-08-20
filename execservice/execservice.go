package execservice

import (
	"context"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/llmrepo"
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
	Prompt        string `json:"prompt" example:"Hello, how are you?"`
	ModelName     string `json:"model_name" example:"gpt-3.5-turbo"`
	ModelProvider string `json:"model_provider" example:"openai"`
}

type TaskResponse struct {
	ID       string `json:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Response string `json:"response" example:"I'm doing well, thank you!"`
}

func (s *execService) Execute(ctx context.Context, request *TaskRequest) (*TaskResponse, error) {
	if request == nil {
		return nil, apiframework.ErrEmptyRequest
	}
	if request.Prompt == "" {
		return nil, fmt.Errorf("prompt is empty %w", apiframework.ErrEmptyRequestBody)
	}
	modelNames := []string{}
	providerNames := []string{}
	if request.ModelName != "" {
		modelNames = append(modelNames, request.ModelName)
	}
	if request.ModelProvider != "" {
		providerNames = append(providerNames, request.ModelProvider)
	}
	response, _, err := s.modelRepo.PromptExecute(ctx, llmrepo.Request{
		ModelNames:    modelNames,
		ProviderTypes: providerNames,
	}, "You are a task processing engine talking to other machines. Return the direct answer without explanation to the given task.", 0.1, request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to execute prompt: %w", err)
	}

	return &TaskResponse{
		ID:       uuid.NewString(),
		Response: response,
	}, nil
}
