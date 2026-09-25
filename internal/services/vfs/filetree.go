package vfs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
)

// childEntry holds the name and file metadata for a single directory child.
// Returned by ListChildrenByParentID to avoid per-child tree walks.
type childEntry struct {
	ID        string
	Name      string
	IsFolder  bool
	Type      string
	Meta      []byte
	BlobsID   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// HasChildren reports whether parentID has any direct children for the tenant.
func (s *store) HasChildren(ctx context.Context, tenantID, parentID string) (bool, error) {
	var exists bool
	err := s.Exec.QueryRowContext(ctx, `
        SELECT EXISTS(SELECT 1 FROM vfs_filestree WHERE tenant_id = $1 AND parent_id = $2)`,
		tenantID, parentID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check for children: %w", err)
	}
	return exists, nil
}

// ListChildrenByParentID fetches name + file metadata for the children under
// parentID for the given tenant in a single JOIN query, ordered and paginated
// per page. Ordering columns come from vfs_filestree so the keyset scan is
// index-backed.
func (s *store) ListChildrenByParentID(ctx context.Context, tenantID, parentID string, page Page) ([]childEntry, error) {
	sortBy, limit, err := page.resolve()
	if err != nil {
		return nil, err
	}
	col := "ft.created_at"
	if sortBy == SortByName {
		col = "ft.name"
	}
	where, order, cargs := keyset(sortBy, page.Desc, page.After, col, "ft.id", 3)
	query := `
		SELECT ft.id, ft.name, f.is_folder, f.type, f.meta,
		       COALESCE(f.blobs_id, ''), ft.created_at, ft.updated_at
		FROM   vfs_filestree ft
		JOIN   vfs_files     f  ON f.id = ft.id AND f.tenant_id = ft.tenant_id
		WHERE  ft.tenant_id = $1 AND ft.parent_id = $2` + where + order +
		fmt.Sprintf(" LIMIT $%d", 3+len(cargs))
	args := append([]any{tenantID, parentID}, cargs...)
	args = append(args, limit)
	rows, err := s.Exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list children failed: %w", err)
	}
	defer rows.Close()
	var out []childEntry
	for rows.Next() {
		var e childEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.IsFolder, &e.Type, &e.Meta, &e.BlobsID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list children scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FindFileIDByName returns the id of the single child named name under parentID,
// or libdb.ErrNotFound. The UNIQUE(tenant_id, parent_id, name) constraint
// guarantees at most one match.
func (s *store) FindFileIDByName(ctx context.Context, tenantID, parentID, name string) (string, error) {
	var id string
	err := s.Exec.QueryRowContext(ctx, `
        SELECT id
        FROM vfs_filestree
        WHERE tenant_id = $1 AND parent_id = $2 AND name = $3`,
		tenantID, parentID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", libdb.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to find file id by name: %w", err)
	}
	return id, nil
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
