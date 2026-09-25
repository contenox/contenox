package runtimetypes

import (
	"context"
	"fmt"
	"strings"

	libdb "github.com/contenox/contenox/libdbexec"
)

// legacyBackendAddressKey is the auto-generated name Postgres gave the
// UNIQUE(type, base_url) constraint on llm_backends.
const legacyBackendAddressKey = "llm_backends_type_base_url_key"

// MigrateBackends carries a database that predates per-entry upstreams to the
// shape the code declares: several backend rows may address the same upstream,
// each with its own name and its own credential, so the resolver has more than
// one candidate to move between when one is rate limited or its key dies.
//
// The constraint lives inside the table definition on SQLite, so removing it
// there is a copy-drop-rename rebuild; Postgres named it and drops it by name.
// Both paths are idempotent and both are no-ops on a database that never had it,
// which is what a fresh file gets.
func MigrateBackends(ctx context.Context, exec libdb.Exec) error {
	if exec == nil {
		return nil
	}
	if exec.DriverName() == "postgres" {
		if _, err := exec.ExecContext(ctx,
			`ALTER TABLE llm_backends DROP CONSTRAINT IF EXISTS `+legacyBackendAddressKey+`;`); err != nil {
			return fmt.Errorf("drop the backend address constraint: %w", err)
		}
		return nil
	}
	return rebuildBackendsWithoutAddressKey(ctx, exec)
}

func rebuildBackendsWithoutAddressKey(ctx context.Context, exec libdb.Exec) error {
	var ddl string
	rows, err := exec.QueryContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'llm_backends'`)
	if err != nil {
		return fmt.Errorf("read the backend table definition: %w", err)
	}
	if rows.Next() {
		if err := rows.Scan(&ddl); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read the backend table definition: %w", err)
		}
	}
	_ = rows.Close()
	if ddl == "" || !strings.Contains(ddl, "UNIQUE(type, base_url)") {
		return nil
	}

	// Foreign keys off for the swap: llm_affinity_group_backend_assignments
	// references this table, and a rename under an open reference is refused.
	swap := []string{
		`PRAGMA foreign_keys=off;`,
		`CREATE TABLE llm_backends_migrated (
		    id VARCHAR(255) PRIMARY KEY,
		    name VARCHAR(512) NOT NULL UNIQUE,
		    base_url VARCHAR(512) NOT NULL,
		    type VARCHAR(512) NOT NULL,
		    created_at TIMESTAMP NOT NULL,
		    updated_at TIMESTAMP NOT NULL
		);`,
		`INSERT INTO llm_backends_migrated (id, name, base_url, type, created_at, updated_at)
		 SELECT id, name, base_url, type, created_at, updated_at FROM llm_backends;`,
		`DROP TABLE llm_backends;`,
		`ALTER TABLE llm_backends_migrated RENAME TO llm_backends;`,
		`PRAGMA foreign_keys=on;`,
	}
	for _, stmt := range swap {
		if _, err := exec.ExecContext(ctx, stmt); err != nil {
			_, _ = exec.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys=on;`)
			return fmt.Errorf("relax the backend address constraint: %w", err)
		}
	}
	return nil
}
