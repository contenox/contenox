package contenoxcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/runtimetypes"
)

// openDBAt opens (and creates if needed) the SQLite database at the given path.
func openDBAt(ctx context.Context, dbPath string) (libdb.DBManager, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("cannot create database directory: %w", err)
	}
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %q: %w", dbPath, err)
	}
	return db, nil
}
