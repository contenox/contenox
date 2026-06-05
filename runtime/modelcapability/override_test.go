package modelcapability

import (
	"context"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (context.Context, Service) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "capability.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx, New(runtimetypes.New(db.WithoutTransaction()))
}

func TestUnit_Service_SetGetUnsetThink(t *testing.T) {
	ctx, svc := newTestService(t)

	override, err := svc.SetThink(ctx, "OpenAI", "gpt-5-mini", true)
	require.NoError(t, err)
	require.Equal(t, "openai", override.Provider)
	require.Equal(t, "gpt-5-mini", override.Model)
	require.NotNil(t, override.CanThink)
	require.True(t, *override.CanThink)

	got, ok, err := svc.Get(ctx, "openai", "gpt-5-mini")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.CanThink)
	require.True(t, *got.CanThink)

	removed, err := svc.Unset(ctx, "openai", "gpt-5-mini")
	require.NoError(t, err)
	require.True(t, removed)

	_, ok, err = svc.Get(ctx, "openai", "gpt-5-mini")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestUnit_Service_FalseOverrideIsPreserved(t *testing.T) {
	ctx, svc := newTestService(t)

	_, err := svc.SetThink(ctx, "vllm", "Qwen/Qwen3-32B", false)
	require.NoError(t, err)

	got, ok, err := svc.Get(ctx, "VLLM", "Qwen/Qwen3-32B")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.CanThink)
	require.False(t, *got.CanThink)
}

func TestUnit_Service_RejectsEmptyProviderOrModel(t *testing.T) {
	ctx, svc := newTestService(t)

	_, err := svc.SetThink(ctx, "", "gpt-5", true)
	require.ErrorContains(t, err, "provider is required")

	_, err = svc.SetThink(ctx, "openai", "  ", true)
	require.ErrorContains(t, err, "model is required")
}
