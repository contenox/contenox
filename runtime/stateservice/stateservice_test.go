package stateservice

import (
	"context"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) Service {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), filepath.Join(t.TempDir(), "cli-config.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return New(nil, db, "")
}

func strPtr(s string) *string { return &s }

func TestSetCLIConfig_TelemetryAndUpdateCheckRoundTrip(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	snap, err := svc.SetCLIConfig(ctx, CLIConfigPatch{
		TelemetryEnabled: strPtr("true"),
		UpdateCheck:      strPtr("false"),
	})
	require.NoError(t, err)
	require.Equal(t, "true", snap.TelemetryEnabled)
	require.Equal(t, "false", snap.UpdateCheck)
	require.True(t, snap.Present["telemetry-enabled"])
	require.True(t, snap.Present["update-check"])

	snap, err = svc.CLIConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "true", snap.TelemetryEnabled)
	require.Equal(t, "false", snap.UpdateCheck)
}

func TestSetCLIConfig_DefaultThinkNormalizesLikeCLI(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	snap, err := svc.SetCLIConfig(ctx, CLIConfigPatch{DefaultThink: strPtr("LOW")})
	require.NoError(t, err)
	require.Equal(t, "low", snap.DefaultThink)

	// Empty clears the override so the runtime's own "high" fallback applies.
	snap, err = svc.SetCLIConfig(ctx, CLIConfigPatch{DefaultThink: strPtr("  ")})
	require.NoError(t, err)
	require.Equal(t, "", snap.DefaultThink)

	_, err = svc.SetCLIConfig(ctx, CLIConfigPatch{DefaultThink: strPtr("extreme")})
	require.Error(t, err)
}
