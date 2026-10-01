package fleetservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/libsandbox"
	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

type missionMCPAsker struct {
	asked chan missiontools.AttentionAsk
}

func (a missionMCPAsker) RaiseAttention(_ context.Context, ask missiontools.AttentionAsk) (string, error) {
	a.asked <- ask
	return "yes", nil
}

func TestSystem_ExternalMissionMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("builds sandboxed agent and host binaries")
	}
	if err := libsandbox.Preflight(); err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(root, 0o700))
	forbidden := filepath.Join(parent, "outside-workspace")
	require.NoError(t, os.WriteFile(forbidden, []byte("must remain inaccessible"), 0o600))
	bin := filepath.Join(root, "contenox")
	agentBin := filepath.Join(root, "mission-agent")
	for _, target := range []struct{ path, pkg string }{
		{bin, "github.com/contenox/contenox/cmd/contenox"},
		{agentBin, "./testdata/mission-agent"},
	} {
		out, err := exec.Command("go", "build", "-o", target.path, target.pkg).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "missions.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	bus := libbus.NewInMem()
	defer bus.Close()
	missions := missionservice.New(db, missionservice.WithEventPublisher(bus))
	asker := missionMCPAsker{asked: make(chan missiontools.AttentionAsk, 1)}
	fleet, agents, stop, err := BuildInProcess(ctx, InProcessDeps{
		DB: db, Bus: bus, Missions: missions, ProjectRoot: root, AttentionAsker: asker,
		MissionProxyCommand: []string{bin, "mission", "bridge"}, Stderr: os.Stderr,
	})
	require.NoError(t, err)
	defer stop()
	agent := &runtimetypes.Agent{Name: "external", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{Transport: runtimetypes.ExternalACPTransportStdio, Command: agentBin, Env: map[string]string{"MISSION_TEST_FORBIDDEN_PATH": forbidden}}))
	require.NoError(t, agents.Create(ctx, agent))
	result, err := fleet.Dispatch(ctx, DispatchRequest{AgentName: "external", Intent: "exercise mission tools", HITLPolicyName: "default"})
	require.NoError(t, err)
	var landed *missionservice.Mission
	require.Eventually(t, func() bool {
		landed, err = missions.Get(ctx, result.MissionID)
		return err == nil && missionservice.IsTerminalStatus(landed.Status)
	}, 20*time.Second, 20*time.Millisecond)
	require.Equal(t, missionservice.StatusLanded, landed.Status)
	reports, err := missions.ListReports(ctx, result.MissionID, 10)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.Equal(t, "external agent reporting via MCP", reports[0].Summary)
	require.NotNil(t, landed.Plan)
	select {
	case ask := <-asker.asked:
		require.Equal(t, result.MissionID, ask.MissionID)
	default:
		t.Fatal("external agent did not reach the attention service")
	}
}
