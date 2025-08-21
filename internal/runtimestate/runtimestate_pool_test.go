package runtimestate_test

// import (
// 	"context"
// 	"encoding/json"
// 	"strings"
// 	"testing"
// 	"time"

// 	libbus "github.com/contenox/runtime/bus"
// 	libdb "github.com/contenox/runtime/dbexec"
// 	"github.com/contenox/runtime-mvp/core/runtimestate"
// 	"github.com/contenox/runtime/store"
// 	"github.com/contenox/runtime-mvp/libs/libroutine"
// 	"github.com/contenox/runtime-mvp/libs/libtestenv"
// 	"github.com/google/uuid"
// 	"github.com/stretchr/testify/require"
// )

// func setupPoolTest(t *testing.T) (context.Context, string, *runtimestate.State, store.Store, func()) {
// 	ctx := context.TODO()

// 	// Setup Ollama instance
// 	ollamaUrl, _, cleanupOllama, err := libtestenv.SetupOllamaLocalInstance(ctx)
// 	require.NoError(t, err)

// 	// Setup database
// 	dbConn, _, cleanupDB, err := libdb.SetupLocalInstance(ctx, "test", "test", "test")
// 	require.NoError(t, err)

// 	dbInstance, err := libdb.NewPostgresDBManager(ctx, dbConn, store.Schema)
// 	require.NoError(t, err)

// 	// Create pubsub
// 	ps, cleanupPS, err := libbus.NewTestPubSub()
// 	require.NoError(t, err)

// 	// Create state with pool feature enabled
// 	backendState, err := runtimestate.New(ctx, dbInstance, ps, runtimestate.WithPools())
// 	require.NoError(t, err)

// 	return ctx, ollamaUrl, backendState, store.New(dbInstance.WithoutTransaction()), func() {
// 		cleanupOllama()
// 		cleanupDB()
// 		cleanupPS()
// 	}
// }

// func TestSystem_PoolAwareState_ShouldSyncAndDownloadModels(t *testing.T) {
// 	ctx, ollamaUrl, backendState, dbStore, cleanup := setupPoolTest(t)
// 	defer cleanup()

// 	triggerChan := make(chan struct{}, 10)
// 	breaker := libroutine.NewRoutine(3, 10*time.Second)
// 	go breaker.Loop(ctx, time.Second, triggerChan, backendState.RunBackendCycle, func(err error) {})
// 	breaker2 := libroutine.NewRoutine(3, 10*time.Second)
// 	go breaker2.Loop(ctx, time.Second, triggerChan, backendState.RunDownloadCycle, func(err error) {})

// 	// Create pool
// 	poolID := uuid.NewString()
// 	require.NoError(t, dbStore.CreatePool(ctx, &store.Pool{
// 		ID:          poolID,
// 		Name:        "test-pool",
// 		PurposeType: "inference",
// 	}))

// 	// Create backend and assign to pool
// 	backendID := uuid.NewString()
// 	require.NoError(t, dbStore.CreateBackend(ctx, &store.Backend{
// 		ID:      backendID,
// 		Name:    "pool-backend",
// 		BaseURL: ollamaUrl,
// 		Type:    "ollama",
// 	}))
// 	require.NoError(t, dbStore.AssignBackendToPool(ctx, poolID, backendID))

// 	// Create model and assign to pool
// 	modelID := uuid.NewString()
// 	require.NoError(t, dbStore.AppendModel(ctx, &store.Model{
// 		ID:    modelID,
// 		Model: "granite-embedding:30m",
// 	}))
// 	require.NoError(t, dbStore.AssignModelToPool(ctx, poolID, modelID))

// 	// Trigger sync and verify state
// 	triggerChan <- struct{}{}
// 	require.Eventually(t, func() bool {
// 		state := backendState.Get(ctx)
// 		if len(state) != 1 {
// 			return false
// 		}
// 		backendState := state[backendID]
// 		return strings.Contains(backendState.Backend.ID, backendID) &&
// 			len(backendState.Models) == 1
// 	}, 5*time.Second, 100*time.Millisecond)

// 	// Verify model download
// 	triggerChan <- struct{}{}
// 	require.Eventually(t, func() bool {
// 		state := backendState.Get(ctx)
// 		if len(state) != 1 {
// 			return false
// 		}
// 		backendState := state[backendID]
// 		if len(backendState.PulledModels) == 0 {
// 			return false
// 		}
// 		r, _ := json.Marshal(backendState.PulledModels[0])
// 		return strings.Contains(string(r), "granite-embedding:30m")
// 	}, 30*time.Second, 1*time.Second)
// }

// func TestSystem_PoolIsolation_ShouldNotLeakModelsAcrossPools(t *testing.T) {
// 	ctx, ollamaUrl, backendState, dbStore, cleanup := setupPoolTest(t)
// 	defer cleanup()

// 	// Create two pools
// 	pool1ID := uuid.NewString()
// 	require.NoError(t, dbStore.CreatePool(ctx, &store.Pool{
// 		ID:          pool1ID,
// 		Name:        "pool-1",
// 		PurposeType: "inference",
// 	}))
// 	pool2ID := uuid.NewString()
// 	require.NoError(t, dbStore.CreatePool(ctx, &store.Pool{
// 		ID:          pool2ID,
// 		Name:        "pool-2",
// 		PurposeType: "training",
// 	}))

// 	// Create backends for each pool
// 	backend1ID := uuid.NewString()
// 	require.NoError(t, dbStore.CreateBackend(ctx, &store.Backend{
// 		ID:      backend1ID,
// 		Name:    "pool-1-backend",
// 		BaseURL: ollamaUrl,
// 		Type:    "ollama",
// 	}))
// 	require.NoError(t, dbStore.AssignBackendToPool(ctx, pool1ID, backend1ID))

// 	backend2ID := uuid.NewString()
// 	require.NoError(t, dbStore.CreateBackend(ctx, &store.Backend{
// 		ID:      backend2ID,
// 		Name:    "pool-2-backend",
// 		BaseURL: "http://localhost:11435",
// 		Type:    "ollama",
// 	}))
// 	require.NoError(t, dbStore.AssignBackendToPool(ctx, pool2ID, backend2ID))

// 	// Create model for pool1
// 	modelID := uuid.NewString()
// 	require.NoError(t, dbStore.AppendModel(ctx, &store.Model{
// 		ID:    modelID,
// 		Model: "granite-embedding:30m",
// 	}))
// 	require.NoError(t, dbStore.AssignModelToPool(ctx, pool1ID, modelID))

// 	// Trigger sync
// 	triggerChan := make(chan struct{}, 10)
// 	breaker := libroutine.NewRoutine(3, 10*time.Second)
// 	go breaker.Loop(ctx, time.Second, triggerChan, backendState.RunBackendCycle, func(err error) {})
// 	triggerChan <- struct{}{}

// 	// Verify only pool1 backend has the model
// 	require.Eventually(t, func() bool {
// 		state := backendState.Get(ctx)
// 		if len(state) != 2 {
// 			return false
// 		}
// 		return len(state[backend1ID].Models) == 1 &&
// 			len(state[backend2ID].Models) == 0
// 	}, 5*time.Second, 100*time.Millisecond)
// }

// func TestPoolBackendRemoval(t *testing.T) {
// 	ctx, ollamaUrl, backendState, dbStore, cleanup := setupPoolTest(t)
// 	defer cleanup()

// 	// Create pool and backend
// 	poolID := uuid.NewString()
// 	require.NoError(t, dbStore.CreatePool(ctx, &store.Pool{
// 		ID:          poolID,
// 		Name:        "test-pool",
// 		PurposeType: "inference",
// 	}))

// 	backendID := uuid.NewString()
// 	require.NoError(t, dbStore.CreateBackend(ctx, &store.Backend{
// 		ID:      backendID,
// 		Name:    "pool-backend",
// 		BaseURL: ollamaUrl,
// 		Type:    "ollama",
// 	}))
// 	require.NoError(t, dbStore.AssignBackendToPool(ctx, poolID, backendID))

// 	// Initial sync
// 	triggerChan := make(chan struct{}, 10)
// 	breaker := libroutine.NewRoutine(3, 10*time.Second)
// 	go breaker.Loop(ctx, time.Second, triggerChan, backendState.RunBackendCycle, func(err error) {})
// 	triggerChan <- struct{}{}
// 	require.Eventually(t, func() bool {
// 		return len(backendState.Get(ctx)) == 1
// 	}, 5*time.Second, 100*time.Millisecond)

// 	// Remove backend from pool
// 	require.NoError(t, dbStore.RemoveBackendFromPool(ctx, poolID, backendID))
// 	triggerChan <- struct{}{}

// 	// Verify backend is removed from state
// 	require.Eventually(t, func() bool {
// 		return len(backendState.Get(ctx)) == 0
// 	}, 5*time.Second, 100*time.Millisecond)
// }
