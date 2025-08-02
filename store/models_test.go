package store_test

import (
	"testing"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_Models_AppendAndGetAllModels(t *testing.T) {
	ctx, s := store.SetupStore(t)

	models, err := s.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models)

	// Append a new model.
	model := &store.Model{
		ID:    uuid.New().String(),
		Model: "test-model",
	}
	err = s.AppendModel(ctx, model)
	require.NoError(t, err)
	require.NotEmpty(t, model.CreatedAt)
	require.NotEmpty(t, model.UpdatedAt)

	models, err = s.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "test-model", models[0].Model)
	require.WithinDuration(t, model.CreatedAt, models[0].CreatedAt, time.Second)
	require.WithinDuration(t, model.UpdatedAt, models[0].UpdatedAt, time.Second)
}

func TestUnit_Models_DeleteModel(t *testing.T) {
	ctx, s := store.SetupStore(t)

	model := &store.Model{
		ID:    uuid.New().String(),
		Model: "model-to-delete",
	}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	err = s.DeleteModel(ctx, "model-to-delete")
	require.NoError(t, err)

	models, err := s.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestUnit_Models_DeleteNonExistentModel(t *testing.T) {
	ctx, s := store.SetupStore(t)

	err := s.DeleteModel(ctx, "non-existent-model")
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Models_GetAllModelsOrder(t *testing.T) {
	ctx, s := store.SetupStore(t)

	model1 := &store.Model{
		ID:    uuid.New().String(),
		Model: "model1",
	}
	err := s.AppendModel(ctx, model1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	model2 := &store.Model{
		ID:    uuid.New().String(),
		Model: "model2",
	}
	err = s.AppendModel(ctx, model2)
	require.NoError(t, err)

	models, err := s.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "model2", models[0].Model)
	require.Equal(t, "model1", models[1].Model)
	require.True(t, models[0].CreatedAt.After(models[1].CreatedAt))
}

func TestUnit_Models_AppendDuplicateModel(t *testing.T) {
	ctx, s := store.SetupStore(t)

	model := &store.Model{
		Model: "duplicate-model",
	}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	err = s.AppendModel(ctx, model)
	require.Error(t, err)
}

func TestUnit_Models_GetModelByName(t *testing.T) {
	ctx, s := store.SetupStore(t)

	model := &store.Model{
		ID:    uuid.New().String(),
		Model: "model-to-get",
	}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	foundModel, err := s.GetModelByName(ctx, "model-to-get")
	require.NoError(t, err)
	require.Equal(t, model.ID, foundModel.ID)
	require.Equal(t, model.Model, foundModel.Model)
	require.WithinDuration(t, model.CreatedAt, foundModel.CreatedAt, time.Second)
	require.WithinDuration(t, model.UpdatedAt, foundModel.UpdatedAt, time.Second)
}
