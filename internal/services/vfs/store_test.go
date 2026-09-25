package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func quiet() func() {
	null, _ := os.Open(os.DevNull)
	sout := os.Stdout
	serr := os.Stderr
	os.Stdout = null
	os.Stderr = null
	return func() {
		defer null.Close()
		os.Stdout = sout
		os.Stderr = serr
	}
}

// setupStore initialises a test Postgres instance with the vfsstore schema.
func setupStore(t *testing.T) (context.Context, fileStore) {
	t.Helper()

	ctx := context.TODO()
	dbManager, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "vfs.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dbManager.Close()) })

	err = InitSchema(ctx, dbManager.WithoutTransaction())
	require.NoError(t, err)

	return ctx, newFileStore(dbManager.WithoutTransaction())
}
