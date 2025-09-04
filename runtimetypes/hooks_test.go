package runtimetypes_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnit_RemoteHooks_CreateAndGet(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	hook := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "test-hook",
		EndpointURL:  "https://example.com/hook",
		Method:       "POST",
		TimeoutMs:    5000,
		Headers:      map[string]string{"X-Trace-ID": "test"},
		ProtocolType: "openai",
	}

	// Create the hook
	err := s.CreateRemoteHook(ctx, hook)
	require.NoError(t, err)

	// Retrieve by ID
	retrieved, err := s.GetRemoteHook(ctx, hook.ID)
	require.NoError(t, err)
	require.Equal(t, hook.ID, retrieved.ID)
	require.Equal(t, hook.Name, retrieved.Name)
	require.Equal(t, hook.EndpointURL, retrieved.EndpointURL)
	require.Equal(t, hook.Method, retrieved.Method)
	require.Equal(t, hook.TimeoutMs, retrieved.TimeoutMs)
	require.Equal(t, hook.Headers, retrieved.Headers)
	require.Equal(t, hook.ProtocolType, retrieved.ProtocolType)
	require.WithinDuration(t, time.Now().UTC(), retrieved.CreatedAt, 1*time.Second)
	require.WithinDuration(t, time.Now().UTC(), retrieved.UpdatedAt, 1*time.Second)

	// Retrieve by name
	retrievedByName, err := s.GetRemoteHookByName(ctx, hook.Name)
	require.NoError(t, err)
	require.Equal(t, hook.ID, retrievedByName.ID)
}

func TestUnit_RemoteHooks_WithHeaders(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	t.Run("create with headers", func(t *testing.T) {
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer some-token",
		}
		hook := &runtimetypes.RemoteHook{
			ID:           uuid.New().String(),
			Name:         "hook-with-headers",
			EndpointURL:  "https://example.com/hook",
			Method:       "POST",
			TimeoutMs:    5000,
			Headers:      headers,
			ProtocolType: "openai",
		}

		err := s.CreateRemoteHook(ctx, hook)
		require.NoError(t, err)

		retrieved, err := s.GetRemoteHook(ctx, hook.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved.Headers)
		require.Equal(t, headers, retrieved.Headers)
	})

	t.Run("create with nil headers", func(t *testing.T) {
		hook := &runtimetypes.RemoteHook{
			ID:           uuid.New().String(),
			Name:         "hook-with-nil-headers",
			EndpointURL:  "https://example.com/nil-hook",
			Method:       "POST",
			TimeoutMs:    5000,
			Headers:      nil,
			ProtocolType: "openai",
		}

		err := s.CreateRemoteHook(ctx, hook)
		require.NoError(t, err)

		retrieved, err := s.GetRemoteHook(ctx, hook.ID)
		require.NoError(t, err)
		require.Nil(t, retrieved.Headers)
	})

	t.Run("update headers", func(t *testing.T) {
		initialHeaders := map[string]string{"Initial": "Value"}
		hook := &runtimetypes.RemoteHook{
			ID:           uuid.New().String(),
			Name:         "hook-to-update-headers",
			EndpointURL:  "https://example.com/update-hook",
			Method:       "PUT",
			TimeoutMs:    3000,
			Headers:      initialHeaders,
			ProtocolType: "openai",
		}
		require.NoError(t, s.CreateRemoteHook(ctx, hook))

		updatedHeaders := map[string]string{"Updated": "NewValue", "Another": "Header"}
		hook.Headers = updatedHeaders
		err := s.UpdateRemoteHook(ctx, hook)
		require.NoError(t, err)

		retrieved, err := s.GetRemoteHook(ctx, hook.ID)
		require.NoError(t, err)
		require.Equal(t, updatedHeaders, retrieved.Headers)

		// Test updating to nil
		hook.Headers = nil
		err = s.UpdateRemoteHook(ctx, hook)
		require.NoError(t, err)

		retrieved, err = s.GetRemoteHook(ctx, hook.ID)
		require.NoError(t, err)
		require.Nil(t, retrieved.Headers)
	})
}

func TestUnit_RemoteHooks_Update(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	original := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "original-hook",
		EndpointURL:  "https://original.com",
		Method:       "GET",
		TimeoutMs:    3000,
		Headers:      map[string]string{"Version": "1"},
		ProtocolType: "openai",
	}

	require.NoError(t, s.CreateRemoteHook(ctx, original))

	// Update the hook
	updated := *original
	updated.Name = "updated-hook"
	updated.EndpointURL = "https://updated.com"
	updated.Method = "POST"
	updated.TimeoutMs = 10000
	updated.Headers = map[string]string{"Version": "2"}
	updated.ProtocolType = "langserve"

	err := s.UpdateRemoteHook(ctx, &updated)
	require.NoError(t, err)

	// Verify updates
	retrieved, err := s.GetRemoteHook(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, updated.Name, retrieved.Name)
	require.Equal(t, updated.EndpointURL, retrieved.EndpointURL)
	require.Equal(t, updated.Method, retrieved.Method)
	require.Equal(t, updated.TimeoutMs, retrieved.TimeoutMs)
	require.Equal(t, updated.Headers, retrieved.Headers)
	require.Equal(t, updated.ProtocolType, retrieved.ProtocolType)
	require.True(t, retrieved.UpdatedAt.After(original.UpdatedAt), "UpdatedAt should change")
}

func TestUnit_RemoteHooks_Delete(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	hook := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "hook-to-delete",
		EndpointURL:  "https://delete.com",
		Method:       "DELETE",
		TimeoutMs:    2000,
		ProtocolType: "langserve",
	}

	require.NoError(t, s.CreateRemoteHook(ctx, hook))

	// Delete the hook
	err := s.DeleteRemoteHook(ctx, hook.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = s.GetRemoteHook(ctx, hook.ID)
	require.Error(t, err, "Should return error after deletion")
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}

func TestUnit_RemoteHooks_List(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create multiple hooks with slight delay
	hooks := []*runtimetypes.RemoteHook{
		{
			ID:           uuid.New().String(),
			Name:         "hook-1",
			EndpointURL:  "https://hook1.com",
			Method:       "POST",
			TimeoutMs:    1000,
			ProtocolType: "langserve",
		},
		{
			ID:           uuid.New().String(),
			Name:         "hook-2",
			EndpointURL:  "https://hook2.com",
			Method:       "PUT",
			TimeoutMs:    2000,
			ProtocolType: "langserve",
		},
		{
			ID:           uuid.New().String(),
			Name:         "hook-3",
			EndpointURL:  "https://hook3.com",
			Method:       "PATCH",
			TimeoutMs:    3000,
			ProtocolType: "langserve",
		},
	}

	for _, hook := range hooks {
		require.NoError(t, s.CreateRemoteHook(ctx, hook))
	}

	// List all hooks using a large limit to simulate a non-paginated call
	list, err := s.ListRemoteHooks(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, list, 3)

	// Verify reverse chronological order (newest first)
	require.Equal(t, hooks[2].ID, list[0].ID)
	require.Equal(t, hooks[1].ID, list[1].ID)
	require.Equal(t, hooks[0].ID, list[2].ID)
}

func TestUnit_RemoteHooks_ListPagination(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create 5 hooks with a small delay to ensure distinct creation times.
	var createdHooks []*runtimetypes.RemoteHook
	for i := range 5 {
		hook := &runtimetypes.RemoteHook{
			ID:           uuid.New().String(),
			Name:         fmt.Sprintf("pagination-hook-%d", i),
			EndpointURL:  "https://example.com",
			Method:       "POST",
			TimeoutMs:    1000,
			ProtocolType: "langserve",
		}
		err := s.CreateRemoteHook(ctx, hook)
		require.NoError(t, err)
		createdHooks = append(createdHooks, hook)
	}

	// Paginate through the results with a limit of 2.
	var receivedHooks []*runtimetypes.RemoteHook
	var lastCursor *time.Time
	limit := 2

	// Fetch first page
	page1, err := s.ListRemoteHooks(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	receivedHooks = append(receivedHooks, page1...)

	lastCursor = &page1[len(page1)-1].CreatedAt

	// Fetch second page
	page2, err := s.ListRemoteHooks(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	receivedHooks = append(receivedHooks, page2...)

	lastCursor = &page2[len(page2)-1].CreatedAt

	// Fetch third page (the last one)
	page3, err := s.ListRemoteHooks(ctx, lastCursor, limit)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	receivedHooks = append(receivedHooks, page3...)

	// Fetch a fourth page, which should be empty
	page4, err := s.ListRemoteHooks(ctx, &page3[0].CreatedAt, limit)
	require.NoError(t, err)
	require.Empty(t, page4)

	// Verify all hooks were retrieved in the correct order.
	require.Len(t, receivedHooks, 5)

	// The order is newest to oldest, so the last created hook should be first.
	require.Equal(t, createdHooks[4].ID, receivedHooks[0].ID)
	require.Equal(t, createdHooks[3].ID, receivedHooks[1].ID)
	require.Equal(t, createdHooks[2].ID, receivedHooks[2].ID)
	require.Equal(t, createdHooks[1].ID, receivedHooks[3].ID)
	require.Equal(t, createdHooks[0].ID, receivedHooks[4].ID)
}

func TestUnit_RemoteHooks_UniqueNameConstraint(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	hook1 := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "unique-hook",
		EndpointURL:  "https://unique1.com",
		Method:       "POST",
		TimeoutMs:    1000,
		ProtocolType: "langserve",
	}

	hook2 := *hook1
	hook2.ID = uuid.New().String()
	hook2.EndpointURL = "https://unique2.com"

	// First creation should succeed
	require.NoError(t, s.CreateRemoteHook(ctx, hook1))

	// Second with same name should fail
	err := s.CreateRemoteHook(ctx, &hook2)
	require.Error(t, err, "Should violate unique name constraint")
}

func TestUnit_RemoteHooks_NotFoundCases(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	t.Run("get_by_id_not_found", func(t *testing.T) {
		_, err := s.GetRemoteHook(ctx, uuid.New().String())
		require.Error(t, err)
		require.True(t, errors.Is(err, libdb.ErrNotFound))
	})

	t.Run("get_by_name_not_found", func(t *testing.T) {
		_, err := s.GetRemoteHookByName(ctx, "non-existent-hook")
		require.Error(t, err)
		require.True(t, errors.Is(err, libdb.ErrNotFound))
	})

	t.Run("update_non_existent", func(t *testing.T) {
		hook := &runtimetypes.RemoteHook{ID: uuid.New().String()}
		err := s.UpdateRemoteHook(ctx, hook)
		require.Error(t, err)
	})

	t.Run("delete_non_existent", func(t *testing.T) {
		err := s.DeleteRemoteHook(ctx, uuid.New().String())
		require.Error(t, err)
	})
}

func TestUnit_RemoteHooks_UpdateNonExistent(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	hook := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(), // Doesn't exist
		Name:         "non-existent",
		EndpointURL:  "https://update.com",
		Method:       "PUT",
		TimeoutMs:    5000,
		ProtocolType: "langserve",
	}

	err := s.UpdateRemoteHook(ctx, hook)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound), "Should return not found error")
}

func TestUnit_RemoteHooks_ListEmpty(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	hooks, err := s.ListRemoteHooks(ctx, nil, 100)
	require.NoError(t, err)
	require.Empty(t, hooks, "Should return empty list when no hooks exist")
}

func TestUnit_RemoteHooks_ConcurrentUpdates(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create initial hook
	hook := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "concurrent-hook",
		EndpointURL:  "https://concurrent.com",
		Method:       "POST",
		TimeoutMs:    1000,
		ProtocolType: "langserve",
	}
	require.NoError(t, s.CreateRemoteHook(ctx, hook))

	// Simulate concurrent updates
	updateHook := func(name string) {
		h, err := s.GetRemoteHook(ctx, hook.ID)
		require.NoError(t, err)

		h.Name = name
		err = s.UpdateRemoteHook(ctx, h)
		require.NoError(t, err)
	}

	// Run concurrent updates
	var wg sync.WaitGroup
	names := []string{"update1", "update2", "update3"}
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			updateHook(n)
		}(name)
	}
	wg.Wait()

	// Verify the final state
	final, err := s.GetRemoteHook(ctx, hook.ID)
	require.NoError(t, err)

	// Should have one of the updated names
	require.Contains(t, names, final.Name)
	require.True(t, final.UpdatedAt.After(hook.UpdatedAt))
}

func TestUnit_RemoteHooks_DeleteCascade(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Create hook
	hook := &runtimetypes.RemoteHook{
		ID:           uuid.New().String(),
		Name:         "cascade-test",
		EndpointURL:  "https://cascade.com",
		Method:       "POST",
		TimeoutMs:    5000,
		ProtocolType: "langserve",
	}
	require.NoError(t, s.CreateRemoteHook(ctx, hook))

	// Delete and recreate with same name should work
	require.NoError(t, s.DeleteRemoteHook(ctx, hook.ID))

	newHook := *hook
	newHook.ID = uuid.New().String()
	require.NoError(t, s.CreateRemoteHook(ctx, &newHook))

	// Verify new hook exists
	retrieved, err := s.GetRemoteHookByName(ctx, hook.Name)
	require.NoError(t, err)
	require.Equal(t, newHook.ID, retrieved.ID)
}

func TestUnit_RemoteHooks_GetByName_ProtocolType(t *testing.T) {
	ctx, s := runtimetypes.SetupStore(t)

	// Test with different protocol types
	protocols := []runtimetypes.HookProtocolType{
		"openai",
		"langserve",
		"langserve-openai",
		"openai-object",
		"ollama",
	}

	for _, protocol := range protocols {
		t.Run(string(protocol), func(t *testing.T) {
			hook := &runtimetypes.RemoteHook{
				ID:           uuid.New().String(),
				Name:         fmt.Sprintf("test-hook-%s", protocol),
				EndpointURL:  "https://example.com/hook",
				Method:       "POST",
				TimeoutMs:    5000,
				ProtocolType: protocol,
			}

			err := s.CreateRemoteHook(ctx, hook)
			require.NoError(t, err)

			// Retrieve by name and verify protocol type
			retrieved, err := s.GetRemoteHookByName(ctx, hook.Name)
			require.NoError(t, err)
			require.Equal(t, protocol, retrieved.ProtocolType)
		})
	}
}
