package acpsvc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionCommandCompletions(t *testing.T) {
	tr, db := newMissionTestTransport(t, &fakeDispatcher{}, &fakeResolver{})
	tr.deps.MissionEnvelopes = fakeEnvelopes{}
	require.Equal(t, []string{"--policy hitl-policy-default.json", "--policy hitl-policy-strict.json"}, tr.missionCompletions().Completions[""].Values)
	require.Contains(t, tr.missionCompletions().Completions[""].Hint, "Up/Down")
	setMissionConfig(t, db, "default-mission-policy", "hitl-policy-strict.json")
	store := runtimetypes.New(db.WithoutTransaction())
	for _, name := range []string{"codex", "disabled"} {
		agent := &runtimetypes.Agent{ID: name, Name: name, Enabled: name != "disabled"}
		require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{Transport: runtimetypes.ExternalACPTransportStdio, Command: "codex-acp"}))
		require.NoError(t, store.CreateAgent(context.Background(), agent))
	}
	for _, command := range tr.acpCommands() {
		if command.Name != "mission" {
			continue
		}
		var meta libacp.CommandCompletionMeta
		require.NoError(t, json.Unmarshal(command.Meta, &meta))
		require.Equal(t, []string{"--policy", "codex"}, meta.Completions[""].Values)
		require.Equal(t, []string{"hitl-policy-default.json", "hitl-policy-strict.json"}, meta.Completions["--policy"].Values)
		require.Equal(t, []string{"codex"}, meta.Completions["--policy hitl-policy-strict.json"].Values)
		require.Contains(t, meta.Completions["--policy hitl-policy-strict.json codex"].Hint, "intent")
		return
	}
	t.Fatal("mission command not advertised")
}

func TestUnit_MissionAgentWithoutIntentDoesNotDispatch(t *testing.T) {
	disp := &fakeDispatcher{}
	tr, db := newMissionTestTransport(t, disp, &fakeResolver{known: map[string]bool{"codex": true}})
	setMissionConfig(t, db, "default-mission-agent", "reviewer")
	for _, args := range []string{"codex", "--policy hitl-policy-strict.json codex"} {
		_, err := tr.handleMission(context.Background(), &sessionEntry{}, args)
		require.ErrorContains(t, err, "no mission dispatched")
		require.Empty(t, disp.got.AgentName)
	}
}
