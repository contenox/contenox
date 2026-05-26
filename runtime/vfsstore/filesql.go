package vfsstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/agent/libdbexec"
)

func (s *store) CreateFile(ctx context.Context, tenantID string, file *File) error {
	now := time.Now().UTC()
	file.CreatedAt = now
	file.UpdatedAt = now
	// Use sql.NullString so empty BlobsID is stored as NULL, not as the string "".
	// The FK constraint on blobs_id allows NULL but rejects non-existent string refs.
	blobsID := sql.NullString{String: file.BlobsID, Valid: file.BlobsID != ""}
	_, err := s.Exec.ExecContext(ctx, `
        INSERT INTO vfs_files (id, tenant_id, type, meta, blobs_id, is_folder, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		file.ID, tenantID, file.Type, file.Meta, blobsID, file.IsFolder,
		file.CreatedAt, file.UpdatedAt)
	return err
}

func (s *store) GetFileByID(ctx context.Context, tenantID, id string) (*File, error) {
	var file File
	var blobsID sql.NullString
	err := s.Exec.QueryRowContext(ctx, `
        SELECT id, type, meta, blobs_id, is_folder, created_at, updated_at
        FROM vfs_files
        WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&file.ID,
		&file.Type,
		&file.Meta,
		&blobsID,
		&file.IsFolder,
		&file.CreatedAt,
		&file.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, libdb.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	if blobsID.Valid {
		file.BlobsID = blobsID.String
	}
	return &file, nil
}

func (s *store) UpdateFile(ctx context.Context, tenantID string, file *File) error {
	file.UpdatedAt = time.Now().UTC()
	blobsID := sql.NullString{String: file.BlobsID, Valid: file.BlobsID != ""}
	result, err := s.Exec.ExecContext(ctx, `
        UPDATE vfs_files
        SET type = $2, meta = $3, is_folder = $4, blobs_id = $5, updated_at = $6
        WHERE id = $1 AND tenant_id = $7`,
		file.ID, file.Type, file.Meta, file.IsFolder, blobsID, file.UpdatedAt, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}
	return checkRowsAffected(result)
}

func (s *store) DeleteFile(ctx context.Context, tenantID, id string) error {
	result, err := s.Exec.ExecContext(ctx, `DELETE FROM vfs_files WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return checkRowsAffected(result)
}

func (s *store) ListFiles(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.Exec.QueryContext(ctx, `SELECT id FROM vfs_files WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list file IDs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return ids, nil
}

// EstimateFileCount returns an exact count of files for the given tenant.
func (s *store) EstimateFileCount(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := s.Exec.QueryRowContext(ctx, `SELECT COUNT(*) FROM vfs_files WHERE tenant_id = $1`, tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count files failed: %w", err)
	}
	return count, nil
}

func (s *store) EnforceMaxFileCount(ctx context.Context, tenantID string, maxCount int64) error {
	count, err := s.EstimateFileCount(ctx, tenantID)
	if err != nil {
		return err
	}
	if count >= maxCount {
		return fmt.Errorf("file limit reached (max %d)", maxCount)
	}
	return nil
}
