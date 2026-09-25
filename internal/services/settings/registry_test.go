package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnit_DefaultResolutionPreservesSourceAndExplicitZero(t *testing.T) {
	t.Setenv("CONTENOX_DEFAULT_MAX_TOKENS", "16384")
	value := Resolve(MaxOutputTokens, "8192", "global", DefaultOutputTokens, nil)
	require.Equal(t, "16384", value.Value)
	require.Equal(t, "CONTENOX_DEFAULT_MAX_TOKENS", value.Source)
	require.Equal(t, "8192", value.Stored)
	zero := "0"
	value = Resolve("default-max-tokens", "8192", "global", DefaultOutputTokens, &zero)
	require.Equal(t, "0", value.Value)
	require.Equal(t, "invocation", value.Source)
	require.Equal(t, MaxOutputTokens, value.Key)
}

func TestUnit_ContextBudgetLimitsAndUnknownCapacity(t *testing.T) {
	for _, tc := range []struct{ request, chain, model, want int }{
		{230000, 131072, 262144, 131072},
		{230000, 0, 262144, 230000},
		{0, 0, 262144, 262144},
		{262144, 0, 32768, 32768},
		{0, 0, 0, FallbackContextTokens},
		{0, 230000, 0, 230000},
	} {
		require.Equal(t, tc.want, ContextBudget(tc.request, tc.chain, tc.model))
	}
}
