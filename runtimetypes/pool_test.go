package runtimetypes_test

import (
	"fmt"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_Pools_CreateAndGetPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{
		ID:          uuid.NewString(),
		Name:        "TestPool",
		PurposeType: "inference",
	}

	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)
	require.NotEmpty(t, pool.ID)

	got, err := s.GetPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, pool.Name, got.Name)
	require.Equal(t, pool.PurposeType, got.PurposeType)
	require.WithinDuration(t, pool.CreatedAt, got.CreatedAt, time.Second)
	require.WithinDuration(t, pool.UpdatedAt, got.UpdatedAt, time.Second)
}

func TestUnit_Pools_UpdatePool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{
		ID:          uuid.NewString(),
		Name:        "InitialPool",
		PurposeType: "testing",
	}

	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	pool.Name = "UpdatedPool"
	pool.PurposeType = "production"

	err = s.UpdatePool(ctx, pool)
	require.NoError(t, err)

	got, err := s.GetPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, "UpdatedPool", got.Name)
	require.Equal(t, "production", got.PurposeType)
	require.True(t, got.UpdatedAt.After(got.CreatedAt))
}

func TestUnit_Pools_DeletePool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{
		ID:          uuid.NewString(),
		Name:        "ToDelete",
		PurposeType: "testing",
	}

	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	err = s.DeletePool(ctx, pool.ID)
	require.NoError(t, err)

	_, err = s.GetPool(ctx, pool.ID)
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Pools_ListPools(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pools, err := s.ListPools(ctx, nil, 100)
	require.NoError(t, err)
	require.Empty(t, pools)

	pool1 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1", PurposeType: "type1"}
	pool2 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool2", PurposeType: "type2"}

	err = s.CreatePool(ctx, pool1)
	require.NoError(t, err)
	err = s.CreatePool(ctx, pool2)
	require.NoError(t, err)

	pools, err = s.ListPools(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, pools, 2)
	require.Equal(t, pool2.ID, pools[0].ID)
	require.Equal(t, pool1.ID, pools[1].ID)
}

func TestUnit_Pools_ListPoolsPagination(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create 5 pools with a small delay to ensure distinct creation times.
	var createdPools []*runtimetypes.Pool
	for i := range 5 {
		pool := &runtimetypes.Pool{
			ID:          uuid.NewString(),
			Name:        fmt.Sprintf("pagination-pool-%d", i),
			PurposeType: "inference",
		}
		err := s.CreatePool(ctx, pool)
		require.NoError(t, err)
		createdPools = append(createdPools, pool)
	}

	// Paginate through the results with a limit of 2.
	var receivedPools []*runtimetypes.Pool
	var lastCursor *time.Time
	limit := 2

	// Fetch first page
	page1, err := s.ListPools(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	receivedPools = append(receivedPools, page1...)
	lastCursor = &page1[len(page1)-1].CreatedAt

	// Fetch second page
	page2, err := s.ListPools(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	receivedPools = append(receivedPools, page2...)
	lastCursor = &page2[len(page2)-1].CreatedAt

	// Fetch third page (the last one)
	page3, err := s.ListPools(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	receivedPools = append(receivedPools, page3...)

	// Fetch a fourth page, which should be empty
	page4, err := s.ListPools(ctx, &page3[0].CreatedAt, limit)
	require.NoError(t, err)
	require.Empty(t, page4)

	// Verify all pools were retrieved in the correct order.
	require.Len(t, receivedPools, 5)

	// The order is newest to oldest, so the last created pool should be first.
	require.Equal(t, createdPools[4].ID, receivedPools[0].ID)
	require.Equal(t, createdPools[3].ID, receivedPools[1].ID)
	require.Equal(t, createdPools[2].ID, receivedPools[2].ID)
	require.Equal(t, createdPools[1].ID, receivedPools[3].ID)
	require.Equal(t, createdPools[0].ID, receivedPools[4].ID)
}

func TestUnit_Pools_GetPoolByName(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{
		ID:          uuid.NewString(),
		Name:        "UniquePool",
		PurposeType: "inference",
	}

	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	got, err := s.GetPoolByName(ctx, "UniquePool")
	require.NoError(t, err)
	require.Equal(t, pool.ID, got.ID)
}

func TestUnit_Pools_ListPoolsByPurpose(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	purpose := "inference"
	pool1 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1", PurposeType: purpose}
	pool2 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool2", PurposeType: "training"}

	err := s.CreatePool(ctx, pool1)
	require.NoError(t, err)
	err = s.CreatePool(ctx, pool2)
	require.NoError(t, err)

	pools, err := s.ListPoolsByPurpose(ctx, purpose, nil, 100)
	require.NoError(t, err)
	require.Len(t, pools, 1)
	require.Equal(t, pool1.ID, pools[0].ID)
}

func TestUnit_Pools_ListPoolsByPurposePagination(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create pools with different purpose types
	purpose := "inference"
	otherPurpose := "training"
	var createdPools []*runtimetypes.Pool
	for i := range 5 {
		pool := &runtimetypes.Pool{
			ID:          uuid.NewString(),
			Name:        fmt.Sprintf("inference-pool-%d", i),
			PurposeType: purpose,
		}
		err := s.CreatePool(ctx, pool)
		require.NoError(t, err)
		createdPools = append(createdPools, pool)
	}

	// Create an extra pool with a different purpose type
	otherPool := &runtimetypes.Pool{
		ID:          uuid.NewString(),
		Name:        "other-pool",
		PurposeType: otherPurpose,
	}
	err := s.CreatePool(ctx, otherPool)
	require.NoError(t, err)

	// Paginate through the results with a limit of 2, filtering by purpose.
	var receivedPools []*runtimetypes.Pool
	var lastCursor *time.Time
	limit := 2

	// Fetch first page
	page1, err := s.ListPoolsByPurpose(ctx, purpose, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	receivedPools = append(receivedPools, page1...)
	lastCursor = &page1[len(page1)-1].CreatedAt

	// Fetch second page
	page2, err := s.ListPoolsByPurpose(ctx, purpose, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	receivedPools = append(receivedPools, page2...)
	lastCursor = &page2[len(page2)-1].CreatedAt

	// Fetch third page (the last one)
	page3, err := s.ListPoolsByPurpose(ctx, purpose, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	receivedPools = append(receivedPools, page3...)

	// Fetch a fourth page, which should be empty
	page4, err := s.ListPoolsByPurpose(ctx, purpose, &page3[0].CreatedAt, limit)
	require.NoError(t, err)
	require.Empty(t, page4)

	// Verify all pools for the specific purpose were retrieved in the correct order.
	require.Len(t, receivedPools, 5)
	require.Equal(t, createdPools[4].ID, receivedPools[0].ID)
	require.Equal(t, createdPools[3].ID, receivedPools[1].ID)
	require.Equal(t, createdPools[2].ID, receivedPools[2].ID)
	require.Equal(t, createdPools[1].ID, receivedPools[3].ID)
	require.Equal(t, createdPools[0].ID, receivedPools[4].ID)

	// Verify that the other purpose pool was not returned.
	for _, p := range receivedPools {
		require.Equal(t, purpose, p.PurposeType)
	}
}

func TestUnit_Pools_AssignAndListBackendsForPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1"}
	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	backend := &runtimetypes.Backend{
		ID:      uuid.NewString(),
		Name:    "Backend1",
		BaseURL: "http://backend1",
		Type:    "ollama",
	}
	err = s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	err = s.AssignBackendToPool(ctx, pool.ID, backend.ID)
	require.NoError(t, err)

	backends, err := s.ListBackendsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Len(t, backends, 1)
	require.Equal(t, backend.ID, backends[0].ID)
}

func TestUnit_Pools_RemoveBackendFromPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1"}
	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	backend := &runtimetypes.Backend{ID: uuid.NewString(), Name: "Backend1"}
	err = s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	err = s.AssignBackendToPool(ctx, pool.ID, backend.ID)
	require.NoError(t, err)

	backends, err := s.ListBackendsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Len(t, backends, 1)

	err = s.RemoveBackendFromPool(ctx, pool.ID, backend.ID)
	require.NoError(t, err)

	backends, err = s.ListBackendsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Empty(t, backends)
}

func TestUnit_Pools_ListPoolsForBackend(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	backend := &runtimetypes.Backend{ID: uuid.NewString(), Name: "Backend1"}
	err := s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	pool1 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1"}
	pool2 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool2"}
	err = s.CreatePool(ctx, pool1)
	require.NoError(t, err)
	err = s.CreatePool(ctx, pool2)
	require.NoError(t, err)

	err = s.AssignBackendToPool(ctx, pool1.ID, backend.ID)
	require.NoError(t, err)
	err = s.AssignBackendToPool(ctx, pool2.ID, backend.ID)
	require.NoError(t, err)

	pools, err := s.ListPoolsForBackend(ctx, backend.ID)
	require.NoError(t, err)
	require.Len(t, pools, 2)
	poolIDs := map[string]bool{pool1.ID: true, pool2.ID: true}
	for _, p := range pools {
		require.True(t, poolIDs[p.ID])
	}
}

func TestUnit_PoolModel_AssignModelToPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create a model with capability fields
	model := &runtimetypes.Model{
		ID:            uuid.New().String(),
		Model:         "test-model",
		ContextLength: 4096,
		CanChat:       true,
		CanEmbed:      false,
		CanPrompt:     true,
		CanStream:     false,
	}
	require.NoError(t, s.AppendModel(ctx, model))

	// Create a pool
	pool := &runtimetypes.Pool{
		ID:          uuid.New().String(),
		Name:        "test-pool",
		PurposeType: "test-purpose",
	}
	require.NoError(t, s.CreatePool(ctx, pool))

	// Assign model to pool
	require.NoError(t, s.AssignModelToPool(ctx, pool.ID, model.ID))

	// Verify model is in the pool with correct capabilities
	models, err := s.ListModelsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Len(t, models, 1)

	// Verify all capability fields
	require.Equal(t, model.ID, models[0].ID)
	require.Equal(t, "test-model", models[0].Model)
	require.Equal(t, 4096, models[0].ContextLength)
	require.True(t, models[0].CanChat)
	require.False(t, models[0].CanEmbed)
	require.True(t, models[0].CanPrompt)
	require.False(t, models[0].CanStream)
	require.WithinDuration(t, model.CreatedAt, models[0].CreatedAt, time.Second)
	require.WithinDuration(t, model.UpdatedAt, models[0].UpdatedAt, time.Second)
}

func TestUnit_Pools_RemoveModelFromPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	model := &runtimetypes.Model{Model: "model1", ContextLength: 1024, CanPrompt: true, CanStream: false}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	pool := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1"}
	err = s.CreatePool(ctx, pool)
	require.NoError(t, err)

	err = s.AssignModelToPool(ctx, pool.ID, model.ID)
	require.NoError(t, err)

	models, err := s.ListModelsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Len(t, models, 1)

	err = s.RemoveModelFromPool(ctx, pool.ID, model.ID)
	require.NoError(t, err)

	models, err = s.ListModelsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestUnit_Pools_ListPoolsForModel(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	model := &runtimetypes.Model{Model: "model1", ContextLength: 1024, CanPrompt: true, CanStream: false}
	err := s.AppendModel(ctx, model)
	require.NoError(t, err)

	pool1 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool1"}
	pool2 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Pool2"}
	err = s.CreatePool(ctx, pool1)
	require.NoError(t, err)
	err = s.CreatePool(ctx, pool2)
	require.NoError(t, err)

	err = s.AssignModelToPool(ctx, pool1.ID, model.ID)
	require.NoError(t, err)
	err = s.AssignModelToPool(ctx, pool2.ID, model.ID)
	require.NoError(t, err)

	pools, err := s.ListPoolsForModel(ctx, model.ID)
	require.NoError(t, err)
	require.Len(t, pools, 2)
	poolIDs := map[string]bool{pool1.ID: true, pool2.ID: true}
	for _, p := range pools {
		require.True(t, poolIDs[p.ID])
	}
}

func TestUnit_Pools_GetNonExistentPool(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	_, err := s.GetPool(ctx, uuid.NewString())
	require.ErrorIs(t, err, libdb.ErrNotFound)

	_, err = s.GetPoolByName(ctx, "non-existent")
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Pools_DuplicatePoolName(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	pool := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Duplicate"}
	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	pool2 := &runtimetypes.Pool{ID: uuid.NewString(), Name: "Duplicate"}
	err = s.CreatePool(ctx, pool2)
	require.Error(t, err)
}

// TestUnit_Pools_ListEmptyAssociations verifies that listing associations
// for a new resource correctly returns an empty slice, not nil.
func TestUnit_Pools_ListEmptyAssociations(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// 1. Test for a new pool
	pool := &runtimetypes.Pool{ID: uuid.NewString(), Name: "EmptyPool"}
	err := s.CreatePool(ctx, pool)
	require.NoError(t, err)

	// Backends for pool should be an empty slice
	backends, err := s.ListBackendsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.NotNil(t, backends, "ListBackendsForPool should return an empty slice, not nil")
	require.Len(t, backends, 0)

	// Models for pool should be an empty slice
	models, err := s.ListModelsForPool(ctx, pool.ID)
	require.NoError(t, err)
	require.NotNil(t, models, "ListModelsForPool should return an empty slice, not nil")
	require.Len(t, models, 0)

	// 2. Test for a new backend
	backend := &runtimetypes.Backend{ID: uuid.NewString(), Name: "EmptyBackend"}
	err = s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	// Pools for backend should be an empty slice
	pools, err := s.ListPoolsForBackend(ctx, backend.ID)
	require.NoError(t, err)
	require.NotNil(t, pools, "ListPoolsForBackend should return an empty slice, not nil")
	require.Len(t, pools, 0)

	// 3. Test for a new model
	model := &runtimetypes.Model{Model: "empty-model", ContextLength: 1024}
	err = s.AppendModel(ctx, model)
	require.NoError(t, err)

	// Pools for model should be an empty slice
	poolsForModel, err := s.ListPoolsForModel(ctx, model.ID)
	require.NoError(t, err)
	require.NotNil(t, poolsForModel, "ListPoolsForModel should return an empty slice, not nil")
	require.Len(t, poolsForModel, 0)
}
