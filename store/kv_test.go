package store_test

import (
	"encoding/json"
	"testing"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnitKV(t *testing.T) {
	ctx, s := store.SetupStore(t)

	type testValue struct {
		Field1 string `json:"field1"`
		Field2 int    `json:"field2"`
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T) (key string, value json.RawMessage)
		cleanup bool
	}{
		{
			name: "Set and Get simple string value",
			setup: func(t *testing.T) (string, json.RawMessage) {
				key := "test-string-" + uuid.NewString()
				value, err := json.Marshal("simple string value")
				require.NoError(t, err)
				return key, value
			},
			cleanup: true,
		},
		{
			name: "Set and Get struct value",
			setup: func(t *testing.T) (string, json.RawMessage) {
				key := "test-struct-" + uuid.NewString()
				value, err := json.Marshal(testValue{
					Field1: "test",
					Field2: 42,
				})
				require.NoError(t, err)
				return key, value
			},
			cleanup: true,
		},
		{
			name: "Set and Get map value",
			setup: func(t *testing.T) (string, json.RawMessage) {
				key := "test-map-" + uuid.NewString()
				value, err := json.Marshal(map[string]interface{}{
					"nested": map[string]int{
						"value": 100,
					},
				})
				require.NoError(t, err)
				return key, value
			},
			cleanup: true,
		},
	}

	// Test basic Set/Get operations
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value := tt.setup(t)

			// Test SetKV
			err := s.SetKV(ctx, key, value)
			require.NoError(t, err)

			if tt.name == "Set and Get simple string value" {
				var str string
				err = s.GetKV(ctx, key, &str)
				require.NoError(t, err)
				require.Equal(t, "simple string value", str)
			} else if tt.name == "Set and Get struct value" {
				var tv testValue
				err = s.GetKV(ctx, key, &tv)
				require.NoError(t, err)
				require.Equal(t, testValue{Field1: "test", Field2: 42}, tv)
			} else {
				var m map[string]interface{}
				err = s.GetKV(ctx, key, &m)
				require.NoError(t, err)
				require.Equal(t, float64(100), m["nested"].(map[string]interface{})["value"])
			}

			if tt.cleanup {
				require.NoError(t, s.DeleteKV(ctx, key))
			}
		})
	}

	t.Run("DeleteKV", func(t *testing.T) {
		key := "to-delete-" + uuid.NewString()
		value := json.RawMessage(`"will be deleted"`)

		err := s.SetKV(ctx, key, value)
		require.NoError(t, err)

		// Verify exists
		var str string
		err = s.GetKV(ctx, key, &str)
		require.NoError(t, err)
		require.Equal(t, "will be deleted", str)

		// Delete
		err = s.DeleteKV(ctx, key)
		require.NoError(t, err)

		// Verify deleted
		err = s.GetKV(ctx, key, &str)
		require.ErrorIs(t, err, libdb.ErrNotFound)
	})

	t.Run("ListKV", func(t *testing.T) {
		// Create some test data
		prefix := "list-test-" + uuid.NewString()
		keys := []string{
			prefix + "-1",
			prefix + "-2",
			prefix + "-3",
			"other-prefix-" + uuid.NewString(),
		}

		for _, key := range keys {
			value := json.RawMessage(`"value"`)
			err := s.SetKV(ctx, key, value)
			require.NoError(t, err)
			defer s.DeleteKV(ctx, key) // Cleanup
		}

		// Test ListKV
		allItems, err := s.ListKV(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(allItems), len(keys))

		// Test ListKVPrefix
		prefixedItems, err := s.ListKVPrefix(ctx, prefix)
		require.NoError(t, err)
		require.Len(t, prefixedItems, 3) // Should match our 3 prefixed keys

		for _, item := range prefixedItems {
			require.Contains(t, item.Key, prefix)
			require.WithinDuration(t, time.Now(), item.CreatedAt, time.Second)
			require.WithinDuration(t, time.Now(), item.UpdatedAt, time.Second)

			// Verify the value can be unmarshaled
			var str string
			err := json.Unmarshal(item.Value, &str)
			require.NoError(t, err)
			require.Equal(t, "value", str)
		}
	})

	t.Run("Non-existent key", func(t *testing.T) {
		var value string
		err := s.GetKV(ctx, "non-existent-key-"+uuid.NewString(), &value)
		require.ErrorIs(t, err, libdb.ErrNotFound)
	})

	t.Run("Update existing key", func(t *testing.T) {
		key := "update-test-" + uuid.NewString()
		initial := json.RawMessage(`{"Field1":"initial","Field2":1}`)
		updated := json.RawMessage(`{"Field1":"updated","Field2":2}`)

		// Set initial value
		err := s.SetKV(ctx, key, initial)
		require.NoError(t, err)
		defer s.DeleteKV(ctx, key)

		// Verify initial value
		var tv testValue
		err = s.GetKV(ctx, key, &tv)
		require.NoError(t, err)
		require.Equal(t, testValue{Field1: "initial", Field2: 1}, tv)

		// Update value
		err = s.SetKV(ctx, key, updated)
		require.NoError(t, err)

		// Verify updated value
		err = s.GetKV(ctx, key, &tv)
		require.NoError(t, err)
		require.Equal(t, testValue{Field1: "updated", Field2: 2}, tv)

		// Verify updated_at changed
		items, err := s.ListKVPrefix(ctx, key)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.True(t, items[0].UpdatedAt.After(items[0].CreatedAt))
	})

	t.Run("RawMessage preservation", func(t *testing.T) {
		key := "raw-message-" + uuid.NewString()
		value := json.RawMessage(`{"field": "value", "number": 123}`)

		err := s.SetKV(ctx, key, value)
		require.NoError(t, err)
		defer s.DeleteKV(ctx, key)

		// Get the raw value back
		var kv *store.KV
		kvs, err := s.ListKVPrefix(ctx, key)
		require.NoError(t, err)
		require.Len(t, kvs, 1)
		kv = kvs[0]

		// Verify the raw JSON is preserved exactly
		require.JSONEq(t, string(value), string(kv.Value))
	})

	t.Run("Upsert", func(t *testing.T) {
		key := "upsert-" + uuid.NewString()
		initial := json.RawMessage(`{"field1": "initial", "field2": 1}`)
		updated := json.RawMessage(`{"field1": "updated", "field2": 2}`)

		// Upsert initial value
		err := s.SetKV(ctx, key, initial)
		require.NoError(t, err)

		// Verify initial value
		var tv testValue
		err = s.GetKV(ctx, key, &tv)
		require.NoError(t, err)
		require.Equal(t, testValue{Field1: "initial", Field2: 1}, tv)

		// Upsert updated value
		err = s.SetKV(ctx, key, updated)
		require.NoError(t, err)

		// Verify updated value
		err = s.GetKV(ctx, key, &tv)
		require.NoError(t, err)
		require.Equal(t, testValue{Field1: "updated", Field2: 2}, tv)

		// Verify updated_at changed
		items, err := s.ListKVPrefix(ctx, key)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.True(t, items[0].UpdatedAt.After(items[0].CreatedAt))
	})
}
