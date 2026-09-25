package vfs

import (
	"context"
	"database/sql"
	"fmt"

	libdb "github.com/contenox/contenox/libdbexec"
)

// fileStore defines all persistence operations for the VFS. Every method takes
// tenantID as an explicit argument; queries filter and writes scope by it.
type fileStore interface {
	// FileTree operations
	// HasChildren reports whether parentID has any direct children.
	HasChildren(ctx context.Context, tenantID, parentID string) (bool, error)
	// ListChildrenByParentID returns name + metadata for the children under
	// parentID in one JOIN query, ordered and keyset-paginated per page.
	ListChildrenByParentID(ctx context.Context, tenantID, parentID string, page Page) ([]childEntry, error)
	// FindFileIDByName returns the single child id named name under parentID, or
	// libdb.ErrNotFound. At most one match exists (UNIQUE tenant_id,parent_id,name).
	FindFileIDByName(ctx context.Context, tenantID, parentID, name string) (string, error)
	GetFileParentID(ctx context.Context, tenantID, id string) (string, error)
	GetFileNameByID(ctx context.Context, tenantID, id string) (string, error)
	CreateFileNameID(ctx context.Context, tenantID, id, parentID, name string) error
	DeleteFileNameID(ctx context.Context, tenantID, id string) error
	UpdateFileNameByID(ctx context.Context, tenantID, id, name string) error
	UpdateFileParentID(ctx context.Context, tenantID, id, newParentID string) error

	// File operations
	CreateFile(ctx context.Context, tenantID string, file *fileRow) error
	GetFileByID(ctx context.Context, tenantID, id string) (*fileRow, error)
	UpdateFile(ctx context.Context, tenantID string, file *fileRow) error
	DeleteFile(ctx context.Context, tenantID, id string) error
	ListFiles(ctx context.Context, tenantID string, page Page) ([]string, error)
	EstimateFileCount(ctx context.Context, tenantID string) (int64, error)
	EnforceMaxFileCount(ctx context.Context, tenantID string, maxCount int64) error

	// Blob operations
	CreateBlob(ctx context.Context, tenantID string, blob *blobRow) error
	GetBlobByID(ctx context.Context, tenantID, id string) (*blobRow, error)
	DeleteBlob(ctx context.Context, tenantID, id string) error
	// UpdateBlob updates blob data and meta in-place without delete+insert churn.
	UpdateBlob(ctx context.Context, tenantID, id string, data, meta []byte) error
}

type store struct {
	Exec libdb.Exec
}

func newFileStore(exec libdb.Exec) fileStore {
	return &store{Exec: exec}
}

func checkRowsAffected(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return libdb.ErrNotFound
	}
	return nil
}
