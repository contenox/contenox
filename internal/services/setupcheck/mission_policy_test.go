package setupcheck_test

import (
	"testing"

	"github.com/contenox/contenox/internal/services/setupcheck"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionPolicyWarning(t *testing.T) {
	for _, policy := range []string{"", "  "} {
		result := setupcheck.AddMissionPolicyIssue(setupcheck.Result{}, policy)
		require.Len(t, result.Issues, 1)
		require.Equal(t, "warning", result.Issues[0].Severity)
		require.Equal(t, "missing_default_mission_policy", result.Issues[0].Code)
		require.Equal(t, "contenox config set execution.missions.permissions.policy hitl-policy-default.json", result.Issues[0].CLICommand)
		require.Empty(t, result.BlockingIssues())
	}
	require.Empty(t, setupcheck.AddMissionPolicyIssue(setupcheck.Result{}, "custom.json").Issues)
}
