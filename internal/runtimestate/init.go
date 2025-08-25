package runtimestate

import (
	"context"
	"errors"
	"fmt"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
)

// Config holds the configuration for the runtime state initializer.
type Config struct {
	DatabaseURL string `json:"database_url"`
	EmbedModel  string `json:"embed_model"`
	TaskModel   string `json:"task_model"`
	ChatModel   string `json:"chat_model"`
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

const (
	ChatPoolID   = "internal_chat_pool"
	ChatPoolName = "Chat"
)

type modelCapability int

const (
	canEmbed modelCapability = iota
	canPrompt
	canChat
)

// InitEmbeder initializes the embedding pool and its designated model.
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
		return fmt.Errorf("init embed pool: %w", err)
	}
	model, err := initEmbedModel(ctx, config, tx, contextLen)
	if err != nil {
		return fmt.Errorf("init embed model: %w", err)
	}
	if err = assignModelToPool(ctx, config, tx, model, pool); err != nil {
		return fmt.Errorf("assign embed model to pool: %w", err)
	}
	return com(ctx)
}

// InitPromptExec initializes the tasks pool and its designated model.
func InitPromptExec(ctx context.Context, config *Config, dbInstance libdb.DBManager, runtime *State, contextLen int) error {
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
		return fmt.Errorf("init task pool: %w", err)
	}
	model, err := initTaskModel(ctx, config, tx, contextLen)
	if err != nil {
		return fmt.Errorf("init task model: %w", err)
	}
	if err = assignModelToPool(ctx, config, tx, model, pool); err != nil {
		return fmt.Errorf("assign task model to pool: %w", err)
	}
	return com(ctx)
}

// InitChatExec initializes the chat pool and its designated model.
func InitChatExec(ctx context.Context, config *Config, dbInstance libdb.DBManager, runtime *State, contextLen int) error {
	tx, com, r, err := dbInstance.WithTransaction(ctx)
	if err != nil {
		return err
	}
	defer r()

	if contextLen <= 0 {
		return fmt.Errorf("invalid context length")
	}
	pool, err := initChatPool(ctx, config, tx, false)
	if err != nil {
		return fmt.Errorf("init chat pool: %w", err)
	}
	model, err := initChatModel(ctx, config, tx, contextLen)
	if err != nil {
		return fmt.Errorf("init chat model: %w", err)
	}
	if err = assignModelToPool(ctx, config, tx, model, pool); err != nil {
		return fmt.Errorf("assign chat model to pool: %w", err)
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

func initChatPool(ctx context.Context, config *Config, tx libdb.Exec, created bool) (*runtimetypes.Pool, error) {
	pool, err := runtimetypes.New(tx).GetPool(ctx, ChatPoolID)
	if !created && errors.Is(err, libdb.ErrNotFound) {
		err = runtimetypes.New(tx).CreatePool(ctx, &runtimetypes.Pool{
			ID:          ChatPoolID,
			Name:        ChatPoolName,
			PurposeType: "Internal Chat",
		})
		if err != nil {
			return nil, err
		}
		return initChatPool(ctx, config, tx, true)
	}
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// initOrUpdateModel is a generic helper that handles the creation or update of a model.
// It ensures a model is created if it doesn't exist or updated with a new capability if it does.
// It returns an error if an existing model has a conflicting context length.
func initOrUpdateModel(ctx context.Context, tx libdb.Exec, tenantID, modelName string, contextLength int, capability modelCapability) (*runtimetypes.Model, error) {
	if modelName == "" {
		return nil, errors.New("model name cannot be empty")
	}
	parsedTenantID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}
	modelID := uuid.NewSHA1(parsedTenantID, []byte(modelName))
	storeInstance := runtimetypes.New(tx)

	// Attempt to retrieve the model by its unique name
	model, err := storeInstance.GetModelByName(ctx, modelName)

	// Case 1: Model does not exist, so we create it.
	if errors.Is(err, libdb.ErrNotFound) {
		newModel := &runtimetypes.Model{
			Model:         modelName,
			ID:            modelID.String(),
			ContextLength: contextLength,
		}
		switch capability {
		case canEmbed:
			newModel.CanEmbed = true
		case canPrompt:
			newModel.CanPrompt = true
		case canChat:
			newModel.CanChat = true
		}
		if err := storeInstance.AppendModel(ctx, newModel); err != nil {
			return nil, fmt.Errorf("failed to append new model '%s': %w", modelName, err)
		}
		return newModel, nil
	}

	// Case 2: An unexpected database error occurred.
	if err != nil {
		return nil, fmt.Errorf("failed to get model '%s': %w", modelName, err)
	}

	// Case 3: Model exists. Validate and update its capabilities if needed.
	if model.ContextLength != contextLength {
		return nil, fmt.Errorf("model '%s' already exists with a different context length (stored: %d, new: %d)", modelName, model.ContextLength, contextLength)
	}

	needsUpdate := false
	switch capability {
	case canEmbed:
		if !model.CanEmbed {
			model.CanEmbed = true
			needsUpdate = true
		}
	case canPrompt:
		if !model.CanPrompt {
			model.CanPrompt = true
			needsUpdate = true
		}
	case canChat:
		if !model.CanChat {
			model.CanChat = true
			needsUpdate = true
		}
	}

	if needsUpdate {
		if err := storeInstance.UpdateModel(ctx, model); err != nil {
			return nil, fmt.Errorf("failed to update model '%s' capabilities: %w", modelName, err)
		}
	}

	return model, nil
}

func initEmbedModel(ctx context.Context, config *Config, tx libdb.Exec, contextLength int) (*runtimetypes.Model, error) {
	return initOrUpdateModel(ctx, tx, config.TenantID, config.EmbedModel, contextLength, canEmbed)
}

func initTaskModel(ctx context.Context, config *Config, tx libdb.Exec, contextLength int) (*runtimetypes.Model, error) {
	return initOrUpdateModel(ctx, tx, config.TenantID, config.TaskModel, contextLength, canPrompt)
}

func initChatModel(ctx context.Context, config *Config, tx libdb.Exec, contextLength int) (*runtimetypes.Model, error) {
	return initOrUpdateModel(ctx, tx, config.TenantID, config.ChatModel, contextLength, canChat)
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
