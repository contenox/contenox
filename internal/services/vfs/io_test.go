package vfs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The IO verbs are the runtime's replacement for reaching for os, so they must
// carry the containment the path helpers already enforce: a handle handed a
// path composed from model output may not read or write outside its root.
func TestUnit_ViewIO_ContainsEveryVerb(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("classified"), 0o600))

	view, err := vfs.OpenView(root)
	require.NoError(t, err)

	require.NoError(t, view.MkdirAll(ctx, "nested/deeper"))
	require.NoError(t, view.WriteFile(ctx, "nested/deeper/note.txt", []byte("hello")))
	got, err := view.ReadFile(ctx, "nested/deeper/note.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))

	info, err := view.Stat(ctx, "nested/deeper/note.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(5), info.Size())

	entries, err := view.ReadDir(ctx, "nested")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "deeper", entries[0].Name())

	require.NoError(t, view.Remove(ctx, "nested/deeper/note.txt"))
	_, err = view.ReadFile(ctx, "nested/deeper/note.txt")
	require.Error(t, err)

	escapes := []string{"../secret.txt", "nested/../../secret.txt", outside}
	for _, name := range escapes {
		t.Run(name, func(t *testing.T) {
			_, err := view.ReadFile(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			err = view.WriteFile(ctx, name, []byte("written"))
			assert.ErrorIs(t, err, vfs.ErrEscape)

			_, err = view.Stat(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			_, err = view.ReadDir(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			err = view.Remove(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			err = view.MkdirAll(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)
		})
	}

	content, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "classified", string(content), "a refused write must leave the file outside the root untouched")
}
