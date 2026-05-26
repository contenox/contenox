package vfsstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/agent/libdbexec"
)

// ChildEntry holds the name and file metadata for a single directory child.
// Returned by ListChildrenByParentID to avoid per-child tree walks.
type ChildEntry struct {
	ID        string
	Name      string
	IsFolder  bool
	Type      string
	Meta      []byte
	BlobsID   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *store) ListFileIDsByParentID(ctx context.Context, tenantID, parentID string) ([]string, error) {
	rows, err := s.Exec.QueryContext(ctx, `
        SELECT id
        FROM vfs_filestree
        WHERE tenant_id = $1 AND parent_id = $2`,
		tenantID, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return ids, nil
}

// ListChildrenByParentID fetches name + file metadata for all items under
// parentID for the given tenant in a single JOIN query.
func (s *store) ListChildrenByParentID(ctx context.Context, tenantID, parentID string) ([]ChildEntry, error) {
	rows, err := s.Exec.QueryContext(ctx, `
		SELECT ft.id, ft.name, f.is_folder, f.type, f.meta,
		       COALESCE(f.blobs_id, ''), f.created_at, f.updated_at
		FROM   vfs_filestree ft
		JOIN   vfs_files     f  ON f.id = ft.id AND f.tenant_id = ft.tenant_id
		WHERE  ft.tenant_id = $1 AND ft.parent_id = $2
		ORDER  BY ft.name`, tenantID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list children failed: %w", err)
	}
	defer rows.Close()
	var out []ChildEntry
	for rows.Next() {
		var e ChildEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.IsFolder, &e.Type, &e.Meta, &e.BlobsID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list children scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *store) ListFileIDsByName(ctx context.Context, tenantID, parentID, name string) ([]string, error) {
	rows, err := s.Exec.QueryContext(ctx, `
        SELECT id
        FROM vfs_filestree
        WHERE tenant_id = $1 AND name = $2 AND parent_id = $3`,
		tenantID, name, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return ids, nil
}

func (s *store) GetFileParentID(ctx context.Context, tenantID, id string) (string, error) {
	var parentID *string
	err := s.Exec.QueryRowContext(ctx, `
        SELECT parent_id
        FROM vfs_filestree
        WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(&parentID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", libdb.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get parent ID: %w", err)
	}
	if parentID == nil {
		return "", libdb.ErrNotFound
	}
	return *parentID, nil
}

func (s *store) GetFileNameByID(ctx context.Context, tenantID, id string) (string, error) {
	var name *string
	err := s.Exec.QueryRowContext(ctx, `
        SELECT name
        FROM vfs_filestree
        WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(&name)

	if errors.Is(err, sql.ErrNoRows) {
		return "", libdb.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get file name: %w", err)
	}
	if name == nil {
		return "", libdb.ErrNotFound
	}
	return *name, nil
}

func (s *store) CreateFileNameID(ctx context.Context, tenantID, id, parentID, name string) error {
	now := time.Now().UTC()
	_, err := s.Exec.ExecContext(ctx, `
        INSERT INTO vfs_filestree (id, tenant_id, parent_id, name, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		id, tenantID, parentID, name, now, now)
	return err
}

func (s *store) DeleteFileNameID(ctx context.Context, tenantID, id string) error {
	result, err := s.Exec.ExecContext(ctx, `DELETE FROM vfs_filestree WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete file name ID: %w", err)
	}
	return checkRowsAffected(result)
}

func (s *store) UpdateFileNameByID(ctx context.Context, tenantID, id, name string) error {
	updatedAt := time.Now().UTC()
	result, err := s.Exec.ExecContext(ctx, `
        UPDATE vfs_filestree
        SET name = $2, updated_at = $3
        WHERE id = $1 AND tenant_id = $4`,
		id, name, updatedAt, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update file name: %w", err)
	}
	return checkRowsAffected(result)
}

func (s *store) UpdateFileParentID(ctx context.Context, tenantID, id, newParentID string) error {
	updatedAt := time.Now().UTC()
	result, err := s.Exec.ExecContext(ctx, `
        UPDATE vfs_filestree
        SET parent_id = $2, updated_at = $3
        WHERE id = $1 AND tenant_id = $4`,
		id, newParentID, updatedAt, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update file parent: %w", err)
	}
	return checkRowsAffected(result)
}
