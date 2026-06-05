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
		{name: "acp", raw: initACPChain, wantThink: []string{"acp_chat", "recovery_chat", "revise", "summarise_failure"}, wantNoThink: []string{"run_tools", "recovery_tools", "verify"}},
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
