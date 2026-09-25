package modelruntime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func TestUnit_BuildWithoutDefaultModel(t *testing.T) {
	ctx := context.Background()
	db, err := libdbexec.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "runtime.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	runtime, err := Build(ctx, db, Config{SkipBackendCycle: true})
	require.NoError(t, err)
	runtime.Stop()
	store := runtimetypes.New(db.WithoutTransaction())
	models, err := store.ListModels(ctx, nil, 100)
	require.NoError(t, err)
	require.Empty(t, models)

	runtime, err = Build(ctx, db, Config{DefaultModel: "configured-model", ContextLength: 8192, SkipBackendCycle: true})
	require.NoError(t, err)
	runtime.Stop()
	models, err = store.ListModels(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "configured-model", models[0].Model)
}
