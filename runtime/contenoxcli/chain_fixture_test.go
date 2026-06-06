package contenoxcli

import (
	"encoding/json"
	"testing"

	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func TestUnit_BuiltinChains_SetThinkOnlyOnUserFacingChatTasks(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantThink   []string
		wantNoThink []string
	}{
		{name: "contenox", raw: initChain, wantThink: []string{"contenox_chat", "recovery_chat", "summarise_failure"}, wantNoThink: []string{"run_tools", "recovery_tools"}},
		{name: "run", raw: initRunChain, wantThink: []string{"contenox_run", "recovery_run", "summarise_failure"}, wantNoThink: []string{"run_tools", "recovery_run_tools"}},
		{name: "acp", raw: initACPChain, wantThink: []string{"coding_inspect", "coding_patch", "coding_verify", "coding_audit", "coding_final", "coding_blocked", "acp_chat", "recovery_chat", "revise", "summarise_failure"}, wantNoThink: []string{"classify_request", "coding_inspect_tools", "coding_patch_tools", "coding_verify_tools", "coding_audit_tools", "coding_audit_route", "run_tools", "recovery_tools", "verify"}},
		{name: "acpx", raw: initACPXChain, wantThink: []string{"acp_chat", "recovery_chat", "summarise_failure"}, wantNoThink: []string{"run_tools", "recovery_tools"}},
		{name: "compact", raw: initCompactChain, wantNoThink: []string{"compact_history"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var chain taskengine.TaskChainDefinition
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &chain))
			byID := make(map[string]taskengine.TaskDefinition)
			for _, task := range chain.Tasks {
				byID[task.ID] = task
			}
			for _, id := range tc.wantThink {
				task, ok := byID[id]
				require.True(t, ok, "task %s missing", id)
				require.NotNil(t, task.ExecuteConfig, "task %s execute_config", id)
				require.Equal(t, "{{var:think}}", task.ExecuteConfig.Think, "task %s think", id)
			}
			for _, id := range tc.wantNoThink {
				task, ok := byID[id]
				require.True(t, ok, "task %s missing", id)
				if task.ExecuteConfig != nil {
					require.Empty(t, task.ExecuteConfig.Think, "task %s should not set think", id)
				}
			}
		})
	}
}

func TestUnit_ACPChain_CodingRailsUseScopedToolExecution(t *testing.T) {
	var chain taskengine.TaskChainDefinition
	require.NoError(t, json.Unmarshal([]byte(initACPChain), &chain))
	require.NotEmpty(t, chain.Tasks)
	require.Equal(t, "classify_request", chain.Tasks[0].ID)

	byID := make(map[string]taskengine.TaskDefinition)
	for _, task := range chain.Tasks {
		byID[task.ID] = task
	}

	classifier := byID["classify_request"]
	require.Equal(t, taskengine.HandleRoute, classifier.Handler)
	var routeLabels []string
	for _, branch := range classifier.Transition.Branches {
		if branch.Operator == taskengine.OpEquals {
			routeLabels = append(routeLabels, branch.When)
		}
	}
	require.ElementsMatch(t, []string{"coding_change", "general"}, routeLabels)
	require.NotContains(t, byID, "coding_plan")

	inspectTools := byID["coding_inspect_tools"]
	require.NotNil(t, inspectTools.ExecuteConfig)
	require.Equal(t, []string{"local_fs"}, inspectTools.ExecuteConfig.Tools)
	require.ElementsMatch(t, []string{"local_fs.write_file", "local_fs.sed"}, inspectTools.ExecuteConfig.HideTools)

	patchTools := byID["coding_patch_tools"]
	require.NotNil(t, patchTools.ExecuteConfig)
	require.ElementsMatch(t, []string{"local_fs", "local_shell"}, patchTools.ExecuteConfig.Tools)
	require.Empty(t, patchTools.ExecuteConfig.HideTools)

	verifyTools := byID["coding_verify_tools"]
	require.NotNil(t, verifyTools.ExecuteConfig)
	require.ElementsMatch(t, []string{"local_fs", "local_shell"}, verifyTools.ExecuteConfig.Tools)
	require.ElementsMatch(t, []string{"local_fs.write_file", "local_fs.sed"}, verifyTools.ExecuteConfig.HideTools)

	auditTools := byID["coding_audit_tools"]
	require.NotNil(t, auditTools.ExecuteConfig)
	require.Equal(t, []string{"local_fs"}, auditTools.ExecuteConfig.Tools)
	require.ElementsMatch(t, []string{"local_fs.write_file", "local_fs.sed"}, auditTools.ExecuteConfig.HideTools)
}
