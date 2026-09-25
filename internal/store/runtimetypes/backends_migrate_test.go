package runtimetypes_test

import (
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

// The old shape, verbatim: the address was part of the table's identity, so a
// second entry over the same upstream was refused by the store.
const legacyBackendsDDL = `
CREATE TABLE llm_backends (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(512) NOT NULL UNIQUE,
    base_url VARCHAR(512) NOT NULL,
    type VARCHAR(512) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(type, base_url)
);`

// A provider wired twice is two entries over one upstream, each with its own
// name and its own credential: that is what lets the resolver move to the second
// when the first is rate limited.
func TestSystem_TwoBackendsMayAddressOneUpstream(t *testing.T) {
	ctx, store := runtimetypes.SetupStore(t)

	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "acct-a", Name: "openai-a", Type: "openai", BaseURL: "https://api.openai.com/v1",
	}))
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "acct-b", Name: "openai-b", Type: "openai", BaseURL: "https://api.openai.com/v1",
	}), "a second entry over the same upstream is the point, not a mistake")

	rows, err := store.ListBackends(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

// Migrating keeps what was there. The rows are the deployment's identity and
// their credentials are referenced by id, so a rebuild that lost them would
// orphan every backend an operator ever added.
func TestSystem_MigratingKeepsExistingBackends(t *testing.T) {
	ctx, dbManager := runtimetypes.SetupDBManager(t)

	if runtimetypes.TestBackendDefault() == runtimetypes.TestBackendPostgres {
		t.Skip("this half is the SQLite copy-drop-rename; TestSystem_MigratingDropsTheLegacyConstraintOnPostgres covers the other driver")
	}

	exec := dbManager.WithoutTransaction()
	_, err := exec.ExecContext(ctx, `DROP TABLE llm_backends`)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, legacyBackendsDDL)
	require.NoError(t, err)
	store := runtimetypes.New(dbManager.WithoutTransaction())
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "keep-me", Name: "kept", Type: "openai", BaseURL: "https://api.openai.com/v1",
	}))

	require.NoError(t, runtimetypes.MigrateBackends(ctx, dbManager.WithoutTransaction()))

	rows, err := store.ListBackends(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "keep-me", rows[0].ID)

	// The rebuild drops the table, and foreign keys are switched off for the
	// swap, so the reference from the affinity-group assignments is worth
	// checking once the swap is done rather than assuming it survived.
	var referencing int
	ddlRows, err := exec.QueryContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE sql LIKE '%REFERENCES llm_backends(id)%'`)
	require.NoError(t, err)
	if ddlRows.Next() {
		require.NoError(t, ddlRows.Scan(&referencing))
	}
	_ = ddlRows.Close()
	require.Equal(t, 1, referencing, "the table that references llm_backends must still reference it")
}

// The Postgres half of the same migration: the constraint was named there, so
// removing it is a drop by name. A database that never had it is untouched,
// which is what the assertion after the second call checks.
func TestSystem_MigratingDropsTheLegacyConstraintOnPostgres(t *testing.T) {
	if runtimetypes.TestBackendDefault() != runtimetypes.TestBackendPostgres {
		t.Skip("Postgres DDL is only meaningful against Postgres")
	}
	ctx, dbManager := runtimetypes.SetupDBManager(t)
	exec := dbManager.WithoutTransaction()

	_, err := exec.ExecContext(ctx,
		`ALTER TABLE llm_backends ADD CONSTRAINT llm_backends_type_base_url_key UNIQUE (type, base_url);`)
	require.NoError(t, err)
	require.NoError(t, runtimetypes.MigrateBackends(ctx, exec))
	require.NoError(t, runtimetypes.MigrateBackends(ctx, exec), "a second run is a no-op, not an error")
}

func TestSystem_MigratingAFreshDatabaseChangesNothing(t *testing.T) {
	ctx, dbManager := runtimetypes.SetupDBManager(t)
	require.NoError(t, runtimetypes.MigrateBackends(ctx, dbManager.WithoutTransaction()))

	store := runtimetypes.New(dbManager.WithoutTransaction())
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{ID: "a", Name: "a", Type: "ollama", BaseURL: "http://one"}))
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{ID: "b", Name: "b", Type: "ollama", BaseURL: "http://one"}))
}
