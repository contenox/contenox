package vfs

import (
	"context"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

// The schema must run on the OSS runtime's own SQLite, not only on the Postgres
// it was written against: SQLite has no ADD COLUMN IF NOT EXISTS, so a fresh
// database takes the create-only path and a migration would be a syntax error
// there.
func TestUnit_InitSchemaRunsOnSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, ":memory:", runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, InitSchema(ctx, db.WithoutTransaction()), "InitSchema must run on SQLite")

	var n int
	require.NoError(t, db.WithoutTransaction().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('vfs_files','vfs_filestree','vfs_blobs')`).Scan(&n))
	require.Equal(t, 3, n, "all three vfs tables must exist")
}
