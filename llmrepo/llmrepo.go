package llmrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/activitytracker"
	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/runtime/llmrepo/internal/llmresolver"
	"github.com/contenox/runtime/ollamatokenizer"
	"github.com/contenox/runtime/runtimestate"
)

var _ ModelRepo = (*modelManager)(nil)

// Unified Request type for all operations
type Request struct {
	ProviderTypes []string // Optional: if empty, uses all default providers
	ModelNames    []string // Optional: if empty, any model is considered
	ContextLength int      // Minimum required context length
	Tracker       activitytracker.ActivityTracker
}

type EmbedRequest struct {
	ModelName    string
	ProviderType string
	Tracker      activitytracker.ActivityTracker // Now exported
}

type ModelRepo interface {
	GetTokenizer(ctx context.Context, modelName string) (Tokenizer, error)
	PromptExecute(
		ctx context.Context,
		req Request,
	) (libmodelprovider.LLMPromptExecClient, error)
	Chat(
		ctx context.Context,
		req Request,
	) (libmodelprovider.LLMChatClient, string, error)
	Embed(
		ctx context.Context,
		embedReq EmbedRequest,
	) (libmodelprovider.LLMEmbedClient, error)
	Stream(
		ctx context.Context,
		req Request,
	) (libmodelprovider.LLMStreamClient, error)
}

type Tokenizer interface {
	Tokenize(ctx context.Context, prompt string) ([]int, error)
	CountTokens(ctx context.Context, prompt string) (int, error)
}

type modelManager struct {
	runtime   *runtimestate.State
	tokenizer ollamatokenizer.Tokenizer
	config    ModelManagerConfig
}

type ModelConfig struct {
	Name     string
	Provider string
}

type ModelManagerConfig struct {
	DefaultPromptModel    ModelConfig
	DefaultEmbeddingModel ModelConfig
	DefaultChatModel      ModelConfig
}

func NewModelManager(runtime *runtimestate.State, tokenizer ollamatokenizer.Tokenizer, config ModelManagerConfig) (*modelManager, error) {
	if tokenizer == nil {
		return nil, errors.New("tokenizer cannot be nil")
	}
	return &modelManager{
		runtime:   runtime,
		tokenizer: tokenizer,
		config:    config,
	}, nil
}

// convertToResolverRequest converts llmrepo.Request to llmresolver.Request
func (e *modelManager) convertToResolverRequest(req Request) llmresolver.Request {
	return llmresolver.Request{
		ProviderTypes: req.ProviderTypes,
		ModelNames:    req.ModelNames,
		ContextLength: req.ContextLength,
		Tracker:       req.Tracker,
	}
}

// convertToResolverEmbedRequest converts llmrepo.EmbedRequest to llmresolver.EmbedRequest
func (e *modelManager) convertToResolverEmbedRequest(req EmbedRequest) llmresolver.EmbedRequest {
	return llmresolver.EmbedRequest{
		ModelName:    req.ModelName,
		ProviderType: req.ProviderType,
		Tracker:      req.Tracker,
	}
}

// PromptExecute implements ModelRepo.
func (e *modelManager) PromptExecute(ctx context.Context, req Request) (libmodelprovider.LLMPromptExecClient, error) {
	runtimeStateResolution := e.GetRuntime(ctx)

	// Apply defaults if not provided
	if len(req.ModelNames) == 0 {
		req.ModelNames = []string{e.config.DefaultPromptModel.Name}
	}
	if len(req.ProviderTypes) == 0 {
		req.ProviderTypes = []string{e.config.DefaultPromptModel.Provider}
	}

	resolverReq := e.convertToResolverRequest(req)
	client, err := llmresolver.PromptExecute(ctx,
		resolverReq,
		runtimeStateResolution,
		llmresolver.Randomly,
	)
	if err != nil {
		return nil, fmt.Errorf("prompt execute: client resolution failed: %w", err)
	}

	return client, nil
}

// Chat implements ModelRepo.
func (e *modelManager) Chat(ctx context.Context, req Request) (libmodelprovider.LLMChatClient, string, error) {
	runtimeStateResolution := e.GetRuntime(ctx)

	// Apply defaults if not provided
	if len(req.ModelNames) == 0 {
		req.ModelNames = []string{e.config.DefaultChatModel.Name}
	}
	if len(req.ProviderTypes) == 0 {
		req.ProviderTypes = []string{e.config.DefaultChatModel.Provider}
	}

	resolverReq := e.convertToResolverRequest(req)
	client, model, err := llmresolver.Chat(ctx,
		resolverReq,
		runtimeStateResolution,
		llmresolver.Randomly,
	)
	if err != nil {
		return nil, "", fmt.Errorf("chat: client resolution failed: %w", err)
	}

	return client, model, nil
}

// Embed implements ModelRepo.
func (e *modelManager) Embed(ctx context.Context, embedReq EmbedRequest) (libmodelprovider.LLMEmbedClient, error) {
	runtimeStateResolution := e.GetRuntime(ctx)

	// Apply defaults if not provided
	if embedReq.ModelName == "" {
		embedReq.ModelName = e.config.DefaultEmbeddingModel.Name
	}
	if embedReq.ProviderType == "" {
		embedReq.ProviderType = e.config.DefaultEmbeddingModel.Provider
	}

	resolverReq := e.convertToResolverEmbedRequest(embedReq)
	client, err := llmresolver.Embed(ctx,
		resolverReq,
		runtimeStateResolution,
		llmresolver.Randomly,
	)
	if err != nil {
		return nil, fmt.Errorf("embed: client resolution failed: %w", err)
	}

	return client, nil
}

// Stream implements ModelRepo.
func (e *modelManager) Stream(ctx context.Context, req Request) (libmodelprovider.LLMStreamClient, error) {
	runtimeStateResolution := e.GetRuntime(ctx)

	// Apply defaults if not provided
	if len(req.ModelNames) == 0 && e.config.DefaultChatModel.Name != "" {
		req.ModelNames = []string{e.config.DefaultChatModel.Name}
	}
	if len(req.ProviderTypes) == 0 && e.config.DefaultChatModel.Provider != "" {
		req.ProviderTypes = []string{e.config.DefaultChatModel.Provider}
	}

	resolverReq := e.convertToResolverRequest(req)
	client, err := llmresolver.Stream(ctx,
		resolverReq,
		runtimeStateResolution,
		llmresolver.Randomly,
	)
	if err != nil {
		return nil, fmt.Errorf("stream: client resolution failed: %w", err)
	}

	return client, nil
}

// GetRuntime implements Embedder.
func (e *modelManager) GetRuntime(ctx context.Context) runtimestate.ProviderFromRuntimeState {
	state := e.runtime.Get(ctx)
	return runtimestate.LocalProviderAdapter(ctx, state)
}

func (e *modelManager) GetTokenizer(ctx context.Context, modelName string) (Tokenizer, error) {
	if e.tokenizer == nil {
		return nil, errors.New("tokenizer not initialized")
	}

	// Get the optimal model for tokenization
	modelForTokenization, err := e.tokenizer.OptimalModel(ctx, modelName)
	if err != nil {
		return nil, err
	}

	// Return an adapter that uses the optimal model
	return &tokenizerAdapter{
		tokenizer: e.tokenizer,
		modelName: modelForTokenization,
	}, nil
}

type tokenizerAdapter struct {
	tokenizer ollamatokenizer.Tokenizer
	modelName string
}

func (a *tokenizerAdapter) Tokenize(ctx context.Context, prompt string) ([]int, error) {
	return a.tokenizer.Tokenize(ctx, a.modelName, prompt)
}

func (a *tokenizerAdapter) CountTokens(ctx context.Context, prompt string) (int, error) {
	return a.tokenizer.CountTokens(ctx, a.modelName, prompt)
}
