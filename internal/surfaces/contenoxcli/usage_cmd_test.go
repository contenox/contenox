package contenoxcli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func TestUnit_LocalUsage_ReadsPersistentSessionAndExcludesOtherClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	db, err := libdbexec.NewSQLiteDBManager(context.Background(), path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	usage := runtimetypes.NewUsageStore(db)
	for _, call := range []struct{ client, session string }{
		{gateway.LocalClientID, "one"}, {gateway.LocalClientID, "two"}, {"remote-client", "one"},
	} {
		require.NoError(t, usage.RecordUsage(context.Background(), runtimetypes.ProxyUsage{
			KeyHash: call.client, ClientID: call.client, SessionID: call.session, Model: "model",
			PromptTokens: 100, CompletionTokens: 40, ThinkingTokens: 10, EffectiveInput: 100, EffectiveOutput: 40, CostMicrodollars: 125000,
		}))
	}
	require.NoError(t, db.Close())
	for _, tc := range []struct {
		session string
		want    int64
	}{{"", 280}, {"one", 140}} {
		t.Run(tc.session, func(t *testing.T) {
			cmd := testCobraCmd()
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.Root().PersistentFlags().Set("db", path))
			cmd.Flags().String("session", tc.session, "")
			cmd.Flags().Bool("json", true, "")
			var out bytes.Buffer
			cmd.SetOut(&out)
			require.NoError(t, runLocalUsage(cmd, nil))
			var totals []runtimetypes.UsageModelTotal
			require.NoError(t, json.Unmarshal(out.Bytes(), &totals))
			require.Len(t, totals, 1)
			require.EqualValues(t, tc.want, totals[0].Snapshot.TotalTokens)
			require.EqualValues(t, tc.want/140*125000, totals[0].Snapshot.CostMicrodollars)
		})
	}
}
