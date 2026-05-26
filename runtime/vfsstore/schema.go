package vfsstore

import (
	"context"
	"fmt"

	"github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/runtimetypes"
)

// InitSchema creates the VFS tables and indexes if they do not already exist,
// then runs idempotent ALTERs so deployments predating tenant-aware storage
// pick up the tenant_id column and the new UNIQUE constraint shape.
//
// Every row carries a tenant_id; queries filter by it so a single store can
// host many tenants. OSS callers use runtimetypes.LocalTenantID.
func InitSchema(ctx context.Context, exec libdbexec.Exec) error {
	_, err := exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS vfs_files (
		    id VARCHAR(255) PRIMARY KEY,
		    tenant_id VARCHAR(255) NOT NULL,
		    type VARCHAR(512) NOT NULL,
		    meta JSONB NOT NULL,
		    blobs_id VARCHAR(255),
		    is_folder BOOLEAN DEFAULT FALSE,
		    created_at TIMESTAMP NOT NULL,
		    updated_at TIMESTAMP NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS vfs_filestree (
		    id VARCHAR(255) PRIMARY KEY,
		    tenant_id VARCHAR(255) NOT NULL,
		    parent_id VARCHAR(255),
		    name VARCHAR(1024) NOT NULL,
		    created_at TIMESTAMP NOT NULL,
		    updated_at TIMESTAMP NOT NULL,
		    CONSTRAINT vfs_filestree_tenant_parent_name_key UNIQUE (tenant_id, parent_id, name)
		);
	`)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS vfs_blobs (
		    id VARCHAR(255) PRIMARY KEY,
		    tenant_id VARCHAR(255) NOT NULL,
		    meta JSONB NOT NULL,
		    data bytea NOT NULL,
		    created_at TIMESTAMP NOT NULL,
		    updated_at TIMESTAMP NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Migration for deployments predating tenancy: backfill tenant_id with the
	// OSS LocalTenantID for any rows that existed before the column did.
	// ADD COLUMN IF NOT EXISTS is a no-op on fresh tables.
	backfill := fmt.Sprintf(
		`ALTER TABLE %%s ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(255) NOT NULL DEFAULT '%s';`,
		runtimetypes.LocalTenantID,
	)
	for _, table := range []string{"vfs_files", "vfs_filestree", "vfs_blobs"} {
		if _, err := exec.ExecContext(ctx, fmt.Sprintf(backfill, table)); err != nil {
			return fmt.Errorf("backfill tenant_id on %s: %w", table, err)
		}
		// Drop the default so future INSERTs must supply tenant_id explicitly,
		// matching the Go layer's contract.
		if _, err := exec.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP DEFAULT;`, table)); err != nil {
			return fmt.Errorf("drop tenant_id default on %s: %w", table, err)
		}
	}

	// Replace the legacy UNIQUE(parent_id, name) constraint with the new
	// UNIQUE(tenant_id, parent_id, name) on existing deployments. The auto-
	// generated name on the old constraint is vfs_filestree_parent_id_name_key.
	if _, err := exec.ExecContext(ctx, `ALTER TABLE vfs_filestree DROP CONSTRAINT IF EXISTS vfs_filestree_parent_id_name_key;`); err != nil {
		return fmt.Errorf("drop legacy unique constraint: %w", err)
	}
	if _, err := exec.ExecContext(ctx, `
		DO $$ BEGIN
		    IF NOT EXISTS (
		        SELECT 1 FROM pg_constraint WHERE conname = 'vfs_filestree_tenant_parent_name_key'
		    ) THEN
		        ALTER TABLE vfs_filestree
		            ADD CONSTRAINT vfs_filestree_tenant_parent_name_key
		            UNIQUE (tenant_id, parent_id, name);
		    END IF;
		END $$;
	`); err != nil {
		return fmt.Errorf("add tenant-aware unique constraint: %w", err)
	}

	// Indexes. CREATE INDEX IF NOT EXISTS is idempotent on both fresh and
	// upgraded schemas.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_vfs_files_tenant ON vfs_files (tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_vfs_filestree_tenant_parent ON vfs_filestree (tenant_id, parent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_vfs_filestree_tenant_name_parent ON vfs_filestree (tenant_id, name, parent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_vfs_blobs_tenant ON vfs_blobs (tenant_id);`,
	} {
		if _, err := exec.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create tenant index: %w", err)
		}
	}

	// Drop now-redundant pre-tenancy indexes if they still exist.
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_vfs_filestree_parent_id;`,
		`DROP INDEX IF EXISTS idx_vfs_filestree_list;`,
	} {
		if _, err := exec.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop legacy index: %w", err)
		}
	}

	// vfs_filestree.id → vfs_files.id   (CASCADE: delete file record removes tree entry)
	// vfs_files.blobs_id → vfs_blobs.id (SET NULL: deleting a blob marks file as blob-less)
	_, err = exec.ExecContext(ctx, `
		DO $$ BEGIN
		    IF NOT EXISTS (
		        SELECT 1 FROM pg_constraint WHERE conname = 'fk_tree_file'
		    ) THEN
		        ALTER TABLE vfs_filestree
		            ADD CONSTRAINT fk_tree_file
		            FOREIGN KEY (id) REFERENCES vfs_files(id) ON DELETE CASCADE;
		    END IF;
		    IF NOT EXISTS (
		        SELECT 1 FROM pg_constraint WHERE conname = 'fk_file_blob'
		    ) THEN
		        ALTER TABLE vfs_files
		            ADD CONSTRAINT fk_file_blob
		            FOREIGN KEY (blobs_id) REFERENCES vfs_blobs(id) ON DELETE SET NULL;
		    END IF;
		END $$;
	`)
	return err
}
