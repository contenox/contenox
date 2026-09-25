package acpsvc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/operatorinbox"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libacp "github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

// wiredMissionsDeps builds the host-level stores over one SQLite database,
// the production arrangement: the operator inbox and the mission store share
// the machine's KV table.
func wiredMissionsDeps(t *testing.T) (Deps, missionservice.Service, operatorinbox.Service) {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), filepath.Join(t.TempDir(), "host.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	missions := missionservice.New(db)
	inbox := operatorinbox.New(db)
	return Deps{DB: db, Missions: missions, Inbox: inbox}, missions, inbox
}

func TestExtMissions_ListAndGetOverTheWire(t *testing.T) {
	deps, missions, _ := wiredMissionsDeps(t)
	require.NoError(t, missions.Create(context.Background(), &missionservice.Mission{
		Intent: "Triage the failing relay tests", AgentName: "agent-ada", HITLPolicyName: "default",
	}))
	require.NoError(t, missions.Create(context.Background(), &missionservice.Mission{
		Intent: "Write the migration notes", AgentName: "native", HITLPolicyName: "default",
	}))

	_, client, _ := startExtWire(t, deps)

	raw, err := extCall(t, client, extMethodMissionsList, nil)
	require.NoError(t, err)
	var rows []MissionRow
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, rows, 2)
	intents := map[string]bool{}
	for _, row := range rows {
		intents[row.Intent] = true
		require.NotEmpty(t, row.ID, "each row carries its mission id")
		require.Equal(t, "open", row.Status, "Create leaves the mission open")
	}
	require.True(t, intents["Triage the failing relay tests"])
	require.True(t, intents["Write the migration notes"])

	raw, err = extCall(t, client, extMethodMissionsGet, map[string]any{"missionId": rows[0].ID})
	require.NoError(t, err)
	var got MissionRow
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, rows[0].ID, got.ID)
	require.Equal(t, rows[0].Intent, got.Intent)

	_, err = extCall(t, client, extMethodMissionsGet, map[string]any{"missionId": "no-such-mission"})
	extWireError(t, err, libacp.ErrResourceNotFound)
}

func TestExtMissions_ReportsOverTheWire(t *testing.T) {
	deps, missions, _ := wiredMissionsDeps(t)
	require.NoError(t, missions.Create(context.Background(), &missionservice.Mission{
		Intent: "Write the migration notes", AgentName: "native", HITLPolicyName: "default",
	}))
	m, err := missions.List(context.Background(), nil, 1)
	require.NoError(t, err)
	require.Len(t, m, 1)
	require.NoError(t, missions.AddReport(context.Background(), m[0].ID, &missionservice.Report{
		Kind: missionservice.ReportKindFinding, Summary: "Found the flake",
	}))

	_, client, _ := startExtWire(t, deps)

	raw, err := extCall(t, client, extMethodMissionsReports, map[string]any{"missionId": m[0].ID})
	require.NoError(t, err)
	var reports []ReportRow
	require.NoError(t, json.Unmarshal(raw, &reports))
	require.Len(t, reports, 1)
	require.Equal(t, string(missionservice.ReportKindFinding), reports[0].Kind)
	require.Equal(t, "Found the flake", reports[0].Summary)
	require.Equal(t, m[0].ID, reports[0].MissionID)
}

func TestExtInbox_ListOverTheWire(t *testing.T) {
	deps, missions, inbox := wiredMissionsDeps(t)
	require.NoError(t, missions.Create(context.Background(), &missionservice.Mission{
		Intent: "Triage the failing relay tests", AgentName: "agent-ada", HITLPolicyName: "default",
	}))
	m, err := missions.List(context.Background(), nil, 1)
	require.NoError(t, err)
	require.Len(t, m, 1)
	require.NoError(t, inbox.Add(context.Background(), &operatorinbox.Item{
		MissionID: m[0].ID, AgentName: "agent-ada", Intent: m[0].Intent,
		Reason: operatorinbox.ReasonParentGone,
		Report: missionservice.Report{
			Kind: missionservice.ReportKindBlocker, Summary: "Needs a decision on the flaky retries",
		},
	}))

	_, client, _ := startExtWire(t, deps)

	raw, err := extCall(t, client, extMethodInboxList, nil)
	require.NoError(t, err)
	var rows []OperatorItemRow
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, rows, 1)
	require.Equal(t, m[0].ID, rows[0].MissionID)
	require.Equal(t, string(operatorinbox.ReasonParentGone), rows[0].Reason)
	require.Equal(t, "Needs a decision on the flaky retries", rows[0].Report.Summary)
	require.Equal(t, string(missionservice.ReportKindBlocker), rows[0].Report.Kind)
	require.False(t, rows[0].Acked, "a fresh item is not acked")

	require.NoError(t, inbox.Ack(context.Background(), rows[0].ID))
	raw, err = extCall(t, client, extMethodInboxList, nil)
	require.NoError(t, err)
	rows = nil
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, rows, 1)
	require.True(t, rows[0].Acked, "the wire row reflects the ack")
}

func TestExtMissions_UnwiredStoresAnswerMethodNotFoundOverTheWire(t *testing.T) {
	// The dispatch switch routes the four host-level methods to their handlers
	// even when no store is wired; each handler then refuses with
	// MethodNotFound — the posture a transport without a mission store answers,
	// proven through a real connection rather than by calling handlers.
	_, client, _ := startExtWire(t, Deps{})

	for _, method := range []string{extMethodMissionsList, extMethodMissionsGet, extMethodMissionsReports, extMethodInboxList} {
		_, err := extCall(t, client, method, nil)
		extWireError(t, err, libacp.ErrMethodNotFound)
	}
}
