package llmrepo

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/runtime/llmresolver"
	"github.com/google/uuid"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/ollamatokenizer"
	"github.com/contenox/runtime/runtimestate"
	"github.com/contenox/runtime/runtimetypes"
)

const (
	EmbedPoolID   = "internal_embed_pool"
	EmbedPoolName = "Embedder"
)

const (
	TasksPoolID   = "internal_tasks_pool"
	TasksPoolName = "Tasks"
)

type ModelRepo interface {
	GetDefaultSystemProvider(ctx context.Context) (libmodelprovider.Provider, error)
	GetTokenizer(ctx context.Context, modelName string) (Tokenizer, error)
	GetRuntime(ctx context.Context) llmresolver.ProviderFromRuntimeState
	GetAvailableProviders(ctx context.Context) ([]libmodelprovider.Provider, error)
}

type Tokenizer interface {
	Tokenize(ctx context.Context, prompt string) ([]int, error)
	CountTokens(ctx context.Context, prompt string) (int, error)
}

func NewEmbedder(ctx context.Context, config *Config, dbInstance libdb.DBManager, contextLen int, runtime *runtimestate.State) (ModelRepo, error) {
	tx, com, r, err := dbInstance.WithTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer r()
	if contextLen <= 0 {
		return nil, fmt.Errorf("invalid context length")
	}
	pool, err := initEmbedPool(ctx, config, tx, false)
	if err != nil {
		return nil, fmt.Errorf("init pool: %w", err)
	}
	model, err := initEmbedModel(ctx, config, tx, contextLen, false)
	if err != nil {
		return nil, fmt.Errorf("init model: %w", err)
	}
	err = assignModelToPool(ctx, config, tx, model, pool)
	if err != nil {
		return nil, fmt.Errorf("assign model to pool: %w", err)
	}
	return &modelManager{
		pool:       pool,
		model:      model,
		dbInstance: dbInstance,
		runtime:    runtime,
		embed:      true,
		prompt:     false,
		contextLen: contextLen,
	}, com(ctx)
}

func NewExecRepo(ctx context.Context, config *Config, dbInstance libdb.DBManager, runtime *runtimestate.State, contextLen int, tokenizer ollamatokenizer.Tokenizer) (ModelRepo, error) {
	tx, com, r, err := dbInstance.WithTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer r()
	if contextLen <= 0 {
		return nil, fmt.Errorf("invalid context length")
	}
	pool, err := initTaskPool(ctx, config, tx, false)
	if err != nil {
		return nil, fmt.Errorf("init pool: %w", err)
	}
	model, err := initTaskModel(ctx, config, tx, contextLen, false)
	if err != nil {
		return nil, fmt.Errorf("init model: %w", err)
	}
	err = assignModelToPool(ctx, config, tx, model, pool)
	if err != nil {
		return nil, fmt.Errorf("assign model to pool: %w", err)
	}

	return &modelManager{
		pool:       pool,
		model:      model,
		dbInstance: dbInstance,
		runtime:    runtime,
		tokenizer:  tokenizer,
		embed:      model.CanEmbed,
		prompt:     model.CanPrompt,
		canChat:    model.CanChat,
		contextLen: contextLen,
	}, com(ctx)
}

type modelManager struct {
	pool       *runtimetypes.Pool
	model      *runtimetypes.Model
	dbInstance libdb.DBManager
	runtime    *runtimestate.State
	tokenizer  ollamatokenizer.Tokenizer
	embed      bool
	prompt     bool
	canChat    bool
	contextLen int
}

// GetRuntime implements Embedder.
func (e *modelManager) GetRuntime(ctx context.Context) llmresolver.ProviderFromRuntimeState {
	state := e.runtime.Get(ctx)
	return runtimestate.LocalProviderAdapter(ctx, state)
}

func (e *modelManager) GetDefaultSystemProvider(ctx context.Context) (libmodelprovider.Provider, error) {
	backends := map[string]runtimetypes.Backend{}
	foundModel := runtimestate.ListModelResponse{}
	for _, v := range e.runtime.Get(ctx) {
		ok, err := e.backendIsInPool(ctx, v.Backend)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, lmr := range v.PulledModels {
			if lmr.Model == e.model.Model {
				foundModel = lmr
				backends[v.Backend.BaseURL] = v.Backend
			}
		}
	}
	var results []string
	for _, backend := range backends {
		results = append(results, backend.BaseURL)
	}
	if len(results) == 0 {
		return nil, errors.New("no backends found")
	}
	provider := libmodelprovider.NewOllamaModelProvider(e.model.Model, results, http.DefaultClient,
		libmodelprovider.CapabilityConfig{
			CanPrompt:     foundModel.CanPrompt,
			CanEmbed:      foundModel.CanEmbed,
			CanChat:       foundModel.CanChat,
			ContextLength: foundModel.ContextLength,
		})
	return provider, nil
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

func (e *modelManager) backendIsInPool(ctx context.Context, backendToVerify runtimetypes.Backend) (bool, error) {
	backendsConfiguredInPool, err := runtimetypes.New(e.dbInstance.WithoutTransaction()).ListBackendsForPool(ctx, e.pool.ID)
	if err != nil {
		return false, fmt.Errorf("failed to list backends for pool %s: %w", e.pool.ID, err)
	}

	for _, poolBackend := range backendsConfiguredInPool {
		if poolBackend.BaseURL == backendToVerify.BaseURL {
			return true, nil
		}
	}
	return false, nil
}

func (e *modelManager) GetAvailableProviders(ctx context.Context) ([]libmodelprovider.Provider, error) {
	backends := map[string]runtimetypes.Backend{}
	for _, v := range e.runtime.Get(ctx) {
		ok, err := e.backendIsInPool(ctx, v.Backend)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, lmr := range v.PulledModels {
			if lmr.Model == e.model.Model {
				backends[v.Backend.BaseURL] = v.Backend
			}
		}
	}

	var providers []libmodelprovider.Provider
	for _, backend := range backends {
		provider := libmodelprovider.NewOllamaModelProvider(
			e.model.Model,
			[]string{backend.BaseURL},
			http.DefaultClient,
			libmodelprovider.CapabilityConfig{
				ContextLength: e.model.ContextLength,
				CanChat:       e.model.CanChat,
				CanEmbed:      e.model.CanEmbed,
				CanStream:     e.model.CanStream,
				CanPrompt:     e.model.CanPrompt,
			},
		)
		providers = append(providers, provider)
	}
	return providers, nil
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

func initEmbedPool(ctx context.Context, config *Config, tx libdb.Exec, created bool) (*runtimetypes.Pool, error) {
	pool, err := runtimetypes.New(tx).GetPool(ctx, EmbedPoolID)
	if !created && errors.Is(err, libdb.ErrNotFound) {
		err = runtimetypes.New(tx).CreatePool(ctx, &runtimetypes.Pool{
			ID:          EmbedPoolID,
			Name:        EmbedPoolName,
			PurposeType: "Internal Embeddings",
		})
		if err != nil {
			return nil, err
		}
		return initEmbedPool(ctx, config, tx, true)
	}
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func initTaskPool(ctx context.Context, config *Config, tx libdb.Exec, created bool) (*runtimetypes.Pool, error) {
	pool, err := runtimetypes.New(tx).GetPool(ctx, TasksPoolID)
	if !created && errors.Is(err, libdb.ErrNotFound) {
		err = runtimetypes.New(tx).CreatePool(ctx, &runtimetypes.Pool{
			ID:          TasksPoolID,
			Name:        TasksPoolName,
			PurposeType: "Internal Tasks",
		})
		if err != nil {
			return nil, err
		}
		return initTaskPool(ctx, config, tx, true)
	}
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func initEmbedModel(ctx context.Context, config *Config, tx libdb.Exec, contextLength int, created bool) (*runtimetypes.Model, error) {
	tenantID, err := uuid.Parse(config.TenantID)
	if err != nil {
		return nil, err
	}
	modelID := uuid.NewSHA1(tenantID, []byte(config.EmbedModel))
	storeInstance := runtimetypes.New(tx)

	model, err := storeInstance.GetModelByName(ctx, config.EmbedModel)
	if err != nil && !errors.Is(err, libdb.ErrNotFound) {
		return nil, fmt.Errorf("get model: %w", err)
	}
	if !created && errors.Is(err, libdb.ErrNotFound) {
		err = storeInstance.AppendModel(ctx, &runtimetypes.Model{
			Model:         config.EmbedModel,
			ID:            modelID.String(),
			ContextLength: contextLength,
			CanEmbed:      true,
		})
		if err != nil {
			return nil, err
		}
		return initEmbedModel(ctx, config, tx, contextLength, true)
	}
	return model, nil
}

func initTaskModel(ctx context.Context, config *Config, tx libdb.Exec, contextLength int, created bool) (*runtimetypes.Model, error) {
	tenantID, err := uuid.Parse(config.TenantID)
	if err != nil {
		return nil, err
	}
	modelID := uuid.NewSHA1(tenantID, []byte(config.TaskModel))
	storeInstance := runtimetypes.New(tx)

	model, err := storeInstance.GetModelByName(ctx, config.TaskModel)
	if err != nil && !errors.Is(err, libdb.ErrNotFound) {
		return nil, fmt.Errorf("get model: %w", err)
	}
	if !created && errors.Is(err, libdb.ErrNotFound) {
		err = storeInstance.AppendModel(ctx, &runtimetypes.Model{
			Model:         config.TaskModel,
			ID:            modelID.String(),
			ContextLength: contextLength,
			CanPrompt:     true,
		})
		if err != nil {
			return nil, err
		}
		return initTaskModel(ctx, config, tx, contextLength, true)
	}
	return model, nil
}

func assignModelToPool(ctx context.Context, _ *Config, tx libdb.Exec, model *runtimetypes.Model, pool *runtimetypes.Pool) error {
	storeInstance := runtimetypes.New(tx)

	models, err := storeInstance.ListModelsForPool(ctx, pool.ID)
	if err != nil {
		return err
	}
	for _, presentModel := range models {
		if presentModel.ID == model.ID {
			return nil
		}
	}
	if err := storeInstance.AssignModelToPool(ctx, pool.ID, model.ID); err != nil {
		return err
	}
	return nil
}

type Config struct {
	DatabaseURL string `json:"database_url"`
	EmbedModel  string `json:"embed_model"`
	TaskModel   string `json:"task_model"`
	TenantID    string `json:"tenant_id"`
}
