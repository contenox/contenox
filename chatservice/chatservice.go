package chatservice

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/taskchainservice"
	"github.com/contenox/runtime/taskengine"
)

const OPENAICHAINKEY = "open-ai-chain-id"

type Service interface {
	OpenAIChatCompletions(ctx context.Context, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error)
	SetTaskChainID(ctx context.Context, taskChainID string) error
	GetTaskChainID(ctx context.Context) (string, error)
}

type service struct {
	dbInstance   libdbexec.DBManager
	chainService taskchainservice.Service
	env          execservice.TasksEnvService
}

func New(
	dbInstance libdbexec.DBManager,
	env execservice.TasksEnvService,
	chainService taskchainservice.Service,
) Service {
	return &service{
		chainService: chainService,
		dbInstance:   dbInstance,
		env:          env,
	}
}

// GetTaskChainID retrieves the currently configured task chain ID for OpenAI compatibility
func (s *service) GetTaskChainID(ctx context.Context) (string, error) {
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)

	var chainID string
	if err := storeInstance.GetKV(ctx, OPENAICHAINKEY, &chainID); err != nil {
		return "", fmt.Errorf("failed to get OpenAI task chain ID: %w", err)
	}

	return chainID, nil
}

// SetTaskChainID configures which task chain to use for OpenAI compatibility
func (s *service) SetTaskChainID(ctx context.Context, taskChainID string) error {
	_, err := s.chainService.Get(ctx, taskChainID)
	if err != nil {
		return fmt.Errorf("invalid task chain ID: %w", err)
	}

	storeInstance := runtimetypes.New(s.dbInstance.WithoutTransaction())
	jsonValue, err := json.Marshal(taskChainID)
	if err != nil {
		return fmt.Errorf("failed to marshal task chain ID: %w", err)
	}

	return storeInstance.SetKV(ctx, OPENAICHAINKEY, jsonValue)
}

func (s *service) OpenAIChatCompletions(ctx context.Context, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	// 1. Get the configured task chain ID
	chainID, err := s.GetTaskChainID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("openai chain not configured: %w", err)
	}

	// 2. Fetch the actual task chain definition
	chain, err := s.chainService.Get(ctx, chainID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load task chain '%s': %w", chainID, err)
	}

	// 3. Execute the task chain with the request data
	result, _, stackTrace, err := s.env.Execute(ctx, chain, req, taskengine.DataTypeOpenAIChat)
	if err != nil {
		return nil, stackTrace, fmt.Errorf("chain execution failed: %w", err)
	}

	// 4. Handle empty results
	if result == nil {
		return nil, stackTrace, fmt.Errorf("empty result from chain execution")
	}

	// 5. Type assertion with proper error handling
	res, ok := result.(taskengine.OpenAIChatResponse)
	if !ok {
		return nil, stackTrace, fmt.Errorf("invalid result type from chain: %T", result)
	}

	return &res, stackTrace, nil
}
