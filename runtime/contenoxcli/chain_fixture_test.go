package contenoxcli

import (
	"encoding/json"
	"regexp"
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
		{name: "contenox", raw: initChain, wantThink: []string{"coding_chat", "coding_recovery", "contenox_chat", "recovery_chat", "summarise_failure"}, wantNoThink: []string{"classify_request", "coding_tools", "coding_recovery_tools", "run_tools", "recovery_tools"}},
		{name: "run", raw: initRunChain, wantThink: []string{"contenox_run", "recovery_run", "summarise_failure"}, wantNoThink: []string{"run_tools", "recovery_run_tools"}},
		{name: "acp", raw: initACPChain, wantThink: []string{"coding_chat", "coding_recovery", "acp_chat", "recovery_chat", "summarise_failure"}, wantNoThink: []string{"classify_request", "coding_tools", "coding_recovery_tools", "run_tools", "recovery_tools"}},
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

func TestUnit_ACPChain_RoutesToSimpleBoundedLoops(t *testing.T) {
	var chain taskengine.TaskChainDefinition
	require.NoError(t, json.Unmarshal([]byte(initACPChain), &chain))
	require.NotEmpty(t, chain.Tasks)
	require.Equal(t, "classify_request", chain.Tasks[0].ID)
	require.Len(t, chain.Tasks, 10)

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

	for _, oldID := range []string{
		"coding_inspect",
		"coding_inspect_tools",
		"coding_patch",
		"coding_patch_tools",
		"coding_verify",
		"coding_verify_tools",
		"coding_audit",
		"coding_audit_tools",
		"coding_audit_route",
		"coding_final",
		"coding_blocked",
		"verify",
		"revise",
	} {
		require.NotContains(t, byID, oldID)
	}

	requireLoop := func(chatID, toolsID, recoveryID string, toolBudget string) {
		chat := byID[chatID]
		require.Equal(t, taskengine.HandleChatCompletion, chat.Handler)
		require.NotNil(t, chat.ExecuteConfig, "task %s execute_config", chatID)
		require.Equal(t, []string{"*"}, chat.ExecuteConfig.Tools, "task %s tools", chatID)
		require.Equal(t, toolBudget, branchGoto(t, chat, taskengine.OpEdgeTraversedAtLeast, toolBudget, recoveryID).When)
		require.Equal(t, toolsID, branchGoto(t, chat, taskengine.OpEquals, taskengine.TransitionToolCall, toolsID).Goto)
		require.Equal(t, taskengine.TermEnd, branchGoto(t, chat, taskengine.OpDefault, "", taskengine.TermEnd).Goto)
		require.Equal(t, "262144", chat.ExecuteConfig.ToolsPolicies["local_fs"]["_max_read_bytes"])
		require.Equal(t, "131072", chat.ExecuteConfig.ToolsPolicies["local_fs"]["_max_output_bytes"])
		require.Equal(t, "1000", chat.ExecuteConfig.ToolsPolicies["local_fs"]["_max_grep_matches"])
		require.Equal(t, "262144", chat.ExecuteConfig.ToolsPolicies["webtools"]["_max_response_bytes"])

		tools := byID[toolsID]
		require.Equal(t, taskengine.HandleExecuteToolCalls, tools.Handler)
		require.Equal(t, chatID, tools.InputVar)
		require.NotNil(t, tools.ExecuteConfig, "task %s execute_config", toolsID)
		require.Equal(t, []string{"*"}, tools.ExecuteConfig.Tools, "task %s tools", toolsID)
		require.Equal(t, "262144", tools.ExecuteConfig.ToolsPolicies["local_fs"]["_max_read_bytes"])
		require.Equal(t, "131072", tools.ExecuteConfig.ToolsPolicies["local_fs"]["_max_output_bytes"])
	}

	requireLoop("coding_chat", "coding_tools", "coding_recovery", "12")
	requireLoop("acp_chat", "run_tools", "recovery_chat", "10")

	codingRecoveryTools := byID["coding_recovery_tools"]
	require.Equal(t, taskengine.HandleExecuteToolCalls, codingRecoveryTools.Handler)
	require.Equal(t, "coding_recovery", codingRecoveryTools.InputVar)
	require.Equal(t, []string{"*"}, codingRecoveryTools.ExecuteConfig.Tools)

	summary := byID["summarise_failure"]
	require.Equal(t, taskengine.HandleChatCompletion, summary.Handler)
	require.Equal(t, "previous_output", summary.InputVar)
	require.Empty(t, summary.ExecuteConfig.Tools)
}

func TestUnit_ContenoxChain_RoutesToSpecialistLoops(t *testing.T) {
	var chain taskengine.TaskChainDefinition
	require.NoError(t, json.Unmarshal([]byte(initChain), &chain))
	require.NotEmpty(t, chain.Tasks)
	require.Equal(t, "classify_request", chain.Tasks[0].ID)
	require.Len(t, chain.Tasks, 10)

	byID := make(map[string]taskengine.TaskDefinition)
	for _, task := range chain.Tasks {
		byID[task.ID] = task
	}

	classifier := byID["classify_request"]
	require.Equal(t, taskengine.HandleRoute, classifier.Handler)
	require.Equal(t, "coding_chat", branchGoto(t, classifier, taskengine.OpEquals, "coding_change", "coding_chat").Goto)
	require.Equal(t, "contenox_chat", branchGoto(t, classifier, taskengine.OpEquals, "general", "contenox_chat").Goto)
	require.Equal(t, "contenox_chat", branchGoto(t, classifier, taskengine.OpDefault, "", "contenox_chat").Goto)

	requireLoop := func(chatID, toolsID, recoveryID string, toolBudget string) {
		chat := byID[chatID]
		require.Equal(t, taskengine.HandleChatCompletion, chat.Handler)
		require.NotNil(t, chat.ExecuteConfig, "task %s execute_config", chatID)
		require.Equal(t, []string{"*"}, chat.ExecuteConfig.Tools, "task %s tools", chatID)
		require.Equal(t, toolBudget, branchGoto(t, chat, taskengine.OpEdgeTraversedAtLeast, toolBudget, recoveryID).When)
		require.Equal(t, toolsID, branchGoto(t, chat, taskengine.OpEquals, taskengine.TransitionToolCall, toolsID).Goto)
		require.Equal(t, taskengine.TermEnd, branchGoto(t, chat, taskengine.OpDefault, "", taskengine.TermEnd).Goto)

		tools := byID[toolsID]
		require.Equal(t, taskengine.HandleExecuteToolCalls, tools.Handler)
		require.Equal(t, chatID, tools.InputVar)
		require.NotNil(t, tools.ExecuteConfig, "task %s execute_config", toolsID)
		require.Equal(t, []string{"*"}, tools.ExecuteConfig.Tools, "task %s tools", toolsID)
		require.Contains(t, tools.ExecuteConfig.ToolsPolicies, "local_fs", "task %s", toolsID)
		require.Contains(t, tools.ExecuteConfig.ToolsPolicies, "webtools", "task %s", toolsID)
	}

	requireLoop("coding_chat", "coding_tools", "coding_recovery", "12")
	requireLoop("contenox_chat", "run_tools", "recovery_chat", "10")

	codingRecoveryTools := byID["coding_recovery_tools"]
	require.Equal(t, taskengine.HandleExecuteToolCalls, codingRecoveryTools.Handler)
	require.Equal(t, "coding_recovery", codingRecoveryTools.InputVar)
	require.Equal(t, []string{"*"}, codingRecoveryTools.ExecuteConfig.Tools)
	require.Equal(t, "8", branchGoto(t, byID["coding_recovery"], taskengine.OpEdgeTraversedAtLeast, "8", "summarise_failure").When)
	require.Equal(t, "8", branchGoto(t, byID["recovery_chat"], taskengine.OpEdgeTraversedAtLeast, "8", "summarise_failure").When)

	summary := byID["summarise_failure"]
	require.Equal(t, taskengine.HandleChatCompletion, summary.Handler)
	require.Equal(t, "previous_output", summary.InputVar)
	require.Empty(t, summary.ExecuteConfig.Tools)
}

func TestUnit_BuiltinRecoveryTasksUseConfiguredDefaultFallback(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ids  []string
	}{
		{name: "contenox", raw: initChain, ids: []string{"coding_recovery", "recovery_chat", "summarise_failure"}},
		{name: "run", raw: initRunChain, ids: []string{"recovery_run", "summarise_failure"}},
		{name: "acp", raw: initACPChain, ids: []string{"coding_recovery", "recovery_chat", "summarise_failure"}},
		{name: "acpx", raw: initACPXChain, ids: []string{"recovery_chat", "summarise_failure"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var chain taskengine.TaskChainDefinition
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &chain))
			byID := make(map[string]taskengine.TaskDefinition)
			for _, task := range chain.Tasks {
				byID[task.ID] = task
			}
			for _, id := range tc.ids {
				task, ok := byID[id]
				require.True(t, ok, "task %s missing", id)
				require.NotNil(t, task.ExecuteConfig, "task %s execute_config", id)
				require.Equal(t, "{{var:alt_model|var:default_model}}", task.ExecuteConfig.Model, "task %s model", id)
				require.Equal(t, "{{var:alt_provider|var:default_provider}}", task.ExecuteConfig.Provider, "task %s provider", id)
			}
		})
	}
}

func TestUnit_BuiltinInteractiveChains_UseConservativeToolOutputCaps(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "contenox", raw: initChain},
		{name: "run", raw: initRunChain},
		{name: "acp", raw: initACPChain},
		{name: "acpx", raw: initACPXChain},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var chain taskengine.TaskChainDefinition
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &chain))
			for _, task := range chain.Tasks {
				if task.ExecuteConfig == nil {
					continue
				}
				fsPolicy := task.ExecuteConfig.ToolsPolicies["local_fs"]
				if len(fsPolicy) > 0 {
					require.Equal(t, "262144", fsPolicy["_max_read_bytes"], "task %s", task.ID)
					require.Equal(t, "131072", fsPolicy["_max_output_bytes"], "task %s", task.ID)
					require.Equal(t, "1000", fsPolicy["_max_grep_matches"], "task %s", task.ID)
				}
				webPolicy := task.ExecuteConfig.ToolsPolicies["webtools"]
				if len(webPolicy) > 0 {
					require.Equal(t, "262144", webPolicy["_max_response_bytes"], "task %s", task.ID)
				}
			}
		})
	}
}

func TestUnit_BuiltinInteractiveChains_ScopeToolExecutionNodes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "contenox", raw: initChain},
		{name: "run", raw: initRunChain},
		{name: "acp", raw: initACPChain},
		{name: "acpx", raw: initACPXChain},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var chain taskengine.TaskChainDefinition
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &chain))
			for _, task := range chain.Tasks {
				if task.Handler != taskengine.HandleExecuteToolCalls {
					continue
				}
				require.NotNil(t, task.ExecuteConfig, "task %s execute_config", task.ID)
				require.Equal(t, []string{"*"}, task.ExecuteConfig.Tools, "task %s tools", task.ID)
				require.Contains(t, task.ExecuteConfig.ToolsPolicies, "local_fs", "task %s", task.ID)
				require.Contains(t, task.ExecuteConfig.ToolsPolicies, "webtools", "task %s", task.ID)
			}
		})
	}
}

func TestUnit_BuiltinChains_LLMTasksIncludeDateMacro(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "contenox", raw: initChain},
		{name: "run", raw: initRunChain},
		{name: "compact", raw: initCompactChain},
		{name: "acp", raw: initACPChain},
		{name: "acpx", raw: initACPXChain},
		{name: "fim", raw: initFIMChain},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var chain taskengine.TaskChainDefinition
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &chain))
			for _, task := range chain.Tasks {
				switch task.Handler {
				case taskengine.HandleChatCompletion, taskengine.HandleRoute:
					require.Contains(t, task.SystemInstruction, "{{date}}", "task %s", task.ID)
				}
			}
		})
	}
}

func branchGoto(t *testing.T, task taskengine.TaskDefinition, operator taskengine.OperatorTerm, when, gotoID string) taskengine.TransitionBranch {
	t.Helper()
	for _, branch := range task.Transition.Branches {
		if branch.Operator == operator && branch.When == when && branch.Goto == gotoID {
			return branch
		}
	}
	require.Failf(t, "missing branch", "task %s missing branch operator=%s when=%q goto=%q", task.ID, operator, when, gotoID)
	return taskengine.TransitionBranch{}
}

// Every model macro in the seeded chains must bottom out in a var that both
// execution paths (CLI buildTemplateVars, ACP chainTemplateVars) always seed
// when a model is known. A final fallback outside this set fails at runtime
// with "template fallback var not set" (BUG-014: ACP did not seed
// default_model, so every recovery task died before model resolution).
func TestUnit_BuiltinChains_ModelMacroFallbacksAlwaysSeeded(t *testing.T) {
	alwaysSeeded := map[string]bool{
		"model": true, "provider": true,
		"default_model": true, "default_provider": true,
	}
	chains := map[string]string{
		"contenox": initChain,
		"run":      initRunChain,
		"acp":      initACPChain,
		"acpx":     initACPXChain,
		"compact":  initCompactChain,
	}
	macroRe := regexp.MustCompile(`^\{\{var:([a-z_]+)(\|var:([a-z_]+))?\}\}$`)
	for name, raw := range chains {
		var chain taskengine.TaskChainDefinition
		require.NoError(t, json.Unmarshal([]byte(raw), &chain), name)
		for _, task := range chain.Tasks {
			if task.ExecuteConfig == nil || task.ExecuteConfig.Model == "" {
				continue
			}
			m := macroRe.FindStringSubmatch(task.ExecuteConfig.Model)
			require.NotNil(t, m, "%s/%s: unexpected model macro shape %q", name, task.ID, task.ExecuteConfig.Model)
			final := m[1]
			if m[3] != "" {
				final = m[3] // fallback var is the floor
			}
			require.True(t, alwaysSeeded[final],
				"%s/%s: model macro %q bottoms out in %q, which is not always seeded",
				name, task.ID, task.ExecuteConfig.Model, final)
		}
	}
}
