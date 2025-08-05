package runtimetypes_test

import (
	"fmt"
	"testing"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_Models_AppendAndGetAllModels(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)
	limit := 100 // Use a large limit to fetch all models

	models, err := s.ListModels(ctx, nil, limit)
	require.NoError(t, err)
	require.Empty(t, models)

	// Append a new model.
	model := &runtimetypes.Model{
		ID:    uuid.New().String(),
		Model: "test-model",
	}
	err = s.AppendModel(ctx, model)
	require.NoError(t, err)
	require.NotEmpty(t, model.CreatedAt)
	require.NotEmpty(t, model.UpdatedAt)

	models, err = s.ListModels(ctx, nil, limit)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "test-model", models[0].Model)
	require.WithinDuration(t, model.CreatedAt, models[0].CreatedAt, time.Second)
	require.WithinDuration(t, model.UpdatedAt, models[0].UpdatedAt, time.Second)
}

func TestUnit_Models_DeleteModel(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)
	limit := 100

	model := &runtimetypes.Model{
		ID:    uuid.New().String(),
		Model: "model-to-delete",
	}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	err = s.DeleteModel(ctx, "model-to-delete")
	require.NoError(t, err)

	models, err := s.ListModels(ctx, nil, limit)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestUnit_Models_DeleteNonExistentModel(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	err := s.DeleteModel(ctx, "non-existent-model")
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Models_GetAllModelsOrder(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)
	limit := 100

	model1 := &runtimetypes.Model{
		ID:    uuid.New().String(),
		Model: "model1",
	}
	err := s.AppendModel(ctx, model1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	model2 := &runtimetypes.Model{
		ID:    uuid.New().String(),
		Model: "model2",
	}
	err = s.AppendModel(ctx, model2)
	require.NoError(t, err)

	models, err := s.ListModels(ctx, nil, limit)
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "model2", models[0].Model)
	require.Equal(t, "model1", models[1].Model)
	require.True(t, models[0].CreatedAt.After(models[1].CreatedAt))
}

func TestUnit_Models_AppendDuplicateModel(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	model := &runtimetypes.Model{
		Model: "duplicate-model",
	}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	err = s.AppendModel(ctx, model)
	require.Error(t, err)
}

func TestUnit_Models_GetModelByName(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	model := &runtimetypes.Model{
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

func TestUnit_Models_ListHandlesPagination(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create 5 models with a small delay to ensure distinct creation times.
	var createdModels []*runtimetypes.Model
	for i := 0; i < 5; i++ {
		model := &runtimetypes.Model{
			ID:    uuid.New().String(),
			Model: fmt.Sprintf("model%d", i),
		}
		err := s.AppendModel(ctx, model)
		require.NoError(t, err)
		createdModels = append(createdModels, model)
		time.Sleep(1 * time.Millisecond)
	}

	// Paginate through the results with a limit of 2.
	var receivedModels []*runtimetypes.Model
	var lastCursor *time.Time
	limit := 2

	// Fetch first page
	page1, err := s.ListModels(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	receivedModels = append(receivedModels, page1...)

	lastCursor = &page1[len(page1)-1].CreatedAt

	// Fetch second page
	page2, err := s.ListModels(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	receivedModels = append(receivedModels, page2...)

	lastCursor = &page2[len(page2)-1].CreatedAt

	// Fetch third page (the last one)
	page3, err := s.ListModels(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	receivedModels = append(receivedModels, page3...)

	// Fetch a fourth page, which should be empty
	page4, err := s.ListModels(ctx, &page3[0].CreatedAt, limit)
	require.NoError(t, err)
	require.Empty(t, page4)

	// Verify all models were retrieved in the correct order.
	require.Len(t, receivedModels, 5)

	// The order is newest to oldest, so the last created model should be first.
	require.Equal(t, createdModels[4].ID, receivedModels[0].ID)
	require.Equal(t, createdModels[3].ID, receivedModels[1].ID)
	require.Equal(t, createdModels[2].ID, receivedModels[2].ID)
	require.Equal(t, createdModels[1].ID, receivedModels[3].ID)
	require.Equal(t, createdModels[0].ID, receivedModels[4].ID)
}
