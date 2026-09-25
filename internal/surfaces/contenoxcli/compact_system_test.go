package contenoxcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/chatservice"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	"github.com/stretchr/testify/require"
)

func TestSystem_Compact_LoadsSystemChainAndPersistsSummary(t *testing.T) {
	r := newInProcessRuntime(t, `{
 "model":"scripted-test",
 "turns":[{"text":"Keep the chosen deployment and its pending checks.","usage":{"prompt_tokens":100,"completion_tokens":10}},
 {"text":"general"},{"text":"Continuing from the summary."}]
 }`, askEverything)
	require.NoError(t, os.MkdirAll(systemDir(r.contenoxDir), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(systemDir(r.contenoxDir), chainCompactDefaultFilename), []byte(initCompactChain), 0600))
	sid := r.newSession(t, r.workspace)
	internalID := r.internalID(t, sid)
	base := time.Now().Add(-time.Hour).UTC()
	history := []taskengine.Message{{Role: "system", Content: "Retain this instruction.", Timestamp: base}}
	for i := 0; i < 12; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, taskengine.Message{Role: role, Content: fmt.Sprintf("message %d", i), Timestamp: base.Add(time.Duration(i+1) * time.Second)})
	}
	manager := chatservice.NewManager(r.workspaceID)
	require.NoError(t, manager.PersistDiff(context.Background(), r.db.WithoutTransaction(), internalID, history))
	r.prompt(t, sid, "/compact 2")
	require.Contains(t, r.notifications.text(), "Compacted 13 messages to 4")
	stored := r.storedMessages(t, internalID)
	require.Len(t, stored, 4)
	require.Equal(t, history[0].Content, stored[0].Content)
	require.Equal(t, "<compact-summary>\nKeep the chosen deployment and its pending checks.\n</compact-summary>", stored[1].Content)
	require.Equal(t, history[11].Content, stored[2].Content)
	require.Equal(t, history[12].Content, stored[3].Content)
	totals, err := runtimetypes.NewUsageStore(r.db).UsageByScope(context.Background(), runtimetypes.UsageScopeSession, runtimetypes.SessionUsageScopeID(gateway.LocalClientID, internalID))
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.EqualValues(t, 110, totals[0].Snapshot.TotalTokens)
	r.prompt(t, sid, "continue")
	require.Contains(t, r.notifications.text(), "Continuing from the summary.")
}
