package store_test

import (
	"testing"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_Backend_CreatesAndFetchesByID(t *testing.T) {
	ctx, s := store.SetupStore(t)

	backend := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "TestBackend",
		BaseURL: "http://localhost:8080",
		Type:    "ollama",
	}

	// Create the backend.
	err := s.CreateBackend(ctx, backend)
	require.NoError(t, err)
	require.NotEmpty(t, backend.ID)

	// Retrieve the backend by ID.
	got, err := s.GetBackend(ctx, backend.ID)
	require.NoError(t, err)
	require.Equal(t, backend.Name, got.Name)
	require.Equal(t, backend.BaseURL, got.BaseURL)
	require.Equal(t, backend.Type, got.Type)
	require.WithinDuration(t, backend.CreatedAt, got.CreatedAt, time.Second)
	require.WithinDuration(t, backend.UpdatedAt, got.UpdatedAt, time.Second)
}

func TestUnit_Backend_UpdatesFieldsCorrectly(t *testing.T) {
	ctx, s := store.SetupStore(t)

	backend := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "InitialBackend",
		BaseURL: "http://initial.url",
		Type:    "ollama",
	}

	// Create the backend.
	err := s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	// Modify some fields.
	backend.Name = "UpdatedBackend"
	backend.BaseURL = "http://updated.url"
	backend.Type = "OpenAI"

	// Update the backend.
	err = s.UpdateBackend(ctx, backend)
	require.NoError(t, err)

	// Retrieve and verify the update.
	got, err := s.GetBackend(ctx, backend.ID)
	require.NoError(t, err)
	require.Equal(t, "UpdatedBackend", got.Name)
	require.Equal(t, "http://updated.url", got.BaseURL)
	require.Equal(t, "OpenAI", got.Type)
	require.True(t, got.UpdatedAt.After(got.CreatedAt))
}

func TestUnit_Backend_DeletesSuccessfully(t *testing.T) {
	ctx, s := store.SetupStore(t)

	backend := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "ToDelete",
		BaseURL: "http://delete.me",
		Type:    "ollama",
	}

	// Create the backend.
	err := s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	// Delete the backend.
	err = s.DeleteBackend(ctx, backend.ID)
	require.NoError(t, err)

	// Attempt to retrieve the deleted backend; expect an error.
	_, err = s.GetBackend(ctx, backend.ID)
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Backend_ListReturnsOrderedByCreationTime(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Initially, the list should be empty.
	backends, err := s.ListBackends(ctx)
	require.NoError(t, err)
	require.Empty(t, backends)

	// Create two backends.
	backend1 := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "Backend1",
		BaseURL: "http://backend1",
		Type:    "ollama",
	}
	backend2 := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "Backend2",
		BaseURL: "http://backend2",
		Type:    "ollama",
	}
	err = s.CreateBackend(ctx, backend1)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	err = s.CreateBackend(ctx, backend2)
	require.NoError(t, err)

	backends, err = s.ListBackends(ctx)
	require.NoError(t, err)
	require.Len(t, backends, 2)
	require.Equal(t, backend2.ID, backends[0].ID)
	require.Equal(t, backend1.ID, backends[1].ID)
}

func TestUnit_Backend_FetchesByName(t *testing.T) {
	ctx, s := store.SetupStore(t)

	backend := &store.Backend{
		ID:      uuid.NewString(),
		Name:    "UniqueBackend",
		BaseURL: "http://unique",
		Type:    "ollama",
	}

	// Create the backend.
	err := s.CreateBackend(ctx, backend)
	require.NoError(t, err)

	// Retrieve the backend by name.
	got, err := s.GetBackendByName(ctx, "UniqueBackend")
	require.NoError(t, err)
	require.Equal(t, backend.ID, got.ID)
}

func TestUnit_Backend_GetNonexistentReturnsNotFound(t *testing.T) {
	ctx, s := store.SetupStore(t)

	// Test retrieval by a non-existent ID.
	_, err := s.GetBackend(ctx, uuid.NewString())
	require.ErrorIs(t, err, libdb.ErrNotFound)

	// Test retrieval by a non-existent name.
	_, err = s.GetBackendByName(ctx, "non-existent-name")
	require.ErrorIs(t, err, libdb.ErrNotFound)
}
