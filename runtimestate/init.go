package runtimestate

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/ollamatokenizer"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
)

type Config struct {
	DatabaseURL string `json:"database_url"`
	EmbedModel  string `json:"embed_model"`
	TaskModel   string `json:"task_model"`
	TenantID    string `json:"tenant_id"`
}

const (
	EmbedPoolID   = "internal_embed_pool"
	EmbedPoolName = "Embedder"
)

const (
	TasksPoolID   = "internal_tasks_pool"
	TasksPoolName = "Tasks"
)

func InitEmbeder(ctx context.Context, config *Config, dbInstance libdb.DBManager, contextLen int, runtime *State) error {
	tx, com, r, err := dbInstance.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer r()
	if contextLen <= 0 {
		return fmt.Errorf("invalid context length")
	}
	pool, err := initEmbedPool(ctx, config, tx, false)
	if err != nil {
		return fmt.Errorf("init pool: %w", err)
	}
	model, err := initEmbedModel(ctx, config, tx, contextLen, false)
	if err != nil {
		return fmt.Errorf("init model: %w", err)
	}
	err = assignModelToPool(ctx, config, tx, model, pool)
	if err != nil {
		return fmt.Errorf("assign model to pool: %w", err)
	}
	return com(ctx)
}

func InitPromptExec(ctx context.Context, config *Config, dbInstance libdb.DBManager, runtime *State, contextLen int, tokenizer ollamatokenizer.Tokenizer) error {
	tx, com, r, err := dbInstance.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer r()
	if contextLen <= 0 {
		return fmt.Errorf("invalid context length")
	}
	pool, err := initTaskPool(ctx, config, tx, false)
	if err != nil {
		return fmt.Errorf("init pool: %w", err)
	}
	model, err := initTaskModel(ctx, config, tx, contextLen, false)
	if err != nil {
		return fmt.Errorf("init model: %w", err)
	}
	err = assignModelToPool(ctx, config, tx, model, pool)
	if err != nil {
		return fmt.Errorf("assign model to pool: %w", err)
	}

	return com(ctx)
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
