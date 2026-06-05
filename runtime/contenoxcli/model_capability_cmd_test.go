package contenoxcli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/modelcapability"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func TestUnit_ModelCapabilitySetCmd_PersistsThinkOverride(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capability-cli.db")
	cmd := testCobraCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Root().PersistentFlags().Set("db", dbPath))
	cmd.Flags().String("think", "", "")
	require.NoError(t, cmd.Flags().Set("think", "true"))

	require.NoError(t, modelCapabilitySetCmd.RunE(cmd, []string{"OpenAI", "gpt-5-mini"}))

	db, err := libdb.NewSQLiteDBManager(context.Background(), dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()

	override, ok, err := modelcapability.New(runtimetypes.New(db.WithoutTransaction())).Get(context.Background(), "openai", "gpt-5-mini")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, override.CanThink)
	require.True(t, *override.CanThink)
}
