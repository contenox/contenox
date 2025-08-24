package chatservice

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/taskchainservice"
	"github.com/contenox/runtime/taskengine"
)

type Service interface {
	OpenAIChatCompletions(ctx context.Context, taskChainID string, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error)
}

type service struct {
	dbInstance   libdbexec.DBManager
	chainService taskchainservice.Service
	env          execservice.TasksEnvService
}

func New(
	env execservice.TasksEnvService,
	chainService taskchainservice.Service,
) Service {
	return &service{
		chainService: chainService,
		env:          env,
	}
}

func (s *service) OpenAIChatCompletions(ctx context.Context, taskChainID string, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	chain, err := s.chainService.Get(ctx, taskChainID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load task chain '%s': %w", taskChainID, err)
	}

	result, _, stackTrace, err := s.env.Execute(ctx, chain, req, taskengine.DataTypeOpenAIChat)
	if err != nil {
		return nil, stackTrace, fmt.Errorf("chain execution failed: %w", err)
	}

	if result == nil {
		return nil, stackTrace, fmt.Errorf("empty result from chain execution")
	}

	res, ok := result.(taskengine.OpenAIChatResponse)
	if !ok {
		return nil, stackTrace, fmt.Errorf("invalid result type from chain: %T", result)
	}

	return &res, stackTrace, nil
}
