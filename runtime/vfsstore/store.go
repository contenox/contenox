package vfsstore

import (
	"context"
	"database/sql"
	"fmt"

	libdb "github.com/contenox/agent/libdbexec"
)

// Store defines all persistence operations for the VFS. Every method takes
// tenantID as an explicit argument; queries filter and writes scope by it.
type Store interface {
	// FileTree operations
	ListFileIDsByParentID(ctx context.Context, tenantID, parentID string) ([]string, error)
	// ListChildrenByParentID returns name + metadata for all children in one JOIN query.
	ListChildrenByParentID(ctx context.Context, tenantID, parentID string) ([]ChildEntry, error)
	ListFileIDsByName(ctx context.Context, tenantID, parentID, name string) ([]string, error)
	GetFileParentID(ctx context.Context, tenantID, id string) (string, error)
	GetFileNameByID(ctx context.Context, tenantID, id string) (string, error)
	CreateFileNameID(ctx context.Context, tenantID, id, parentID, name string) error
	DeleteFileNameID(ctx context.Context, tenantID, id string) error
	UpdateFileNameByID(ctx context.Context, tenantID, id, name string) error
	UpdateFileParentID(ctx context.Context, tenantID, id, newParentID string) error

	// File operations
	CreateFile(ctx context.Context, tenantID string, file *File) error
	GetFileByID(ctx context.Context, tenantID, id string) (*File, error)
	UpdateFile(ctx context.Context, tenantID string, file *File) error
	DeleteFile(ctx context.Context, tenantID, id string) error
	ListFiles(ctx context.Context, tenantID string) ([]string, error)
	EstimateFileCount(ctx context.Context, tenantID string) (int64, error)
	EnforceMaxFileCount(ctx context.Context, tenantID string, maxCount int64) error

	// Blob operations
	CreateBlob(ctx context.Context, tenantID string, blob *Blob) error
	GetBlobByID(ctx context.Context, tenantID, id string) (*Blob, error)
	DeleteBlob(ctx context.Context, tenantID, id string) error
	// UpdateBlob updates blob data and meta in-place without delete+insert churn.
	UpdateBlob(ctx context.Context, tenantID, id string, data, meta []byte) error
}

type store struct {
	Exec libdb.Exec
}

// New creates a new VFS store instance.
func New(exec libdb.Exec) Store {
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
