package acpsvc

import (
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"testing"
)

func TestUnit_ModelContextMinimumAcrossEligibleWorkers(t *testing.T) {
	worker := func(provider string, cap int, broken bool) runtimestate.BackendRuntimeState {
		state := runtimestate.BackendRuntimeState{Backend: runtimetypes.Backend{Type: provider}, PulledModels: []runtimestate.ModelPullStatus{{Name: "same-model", ContextLength: cap}}}
		if broken {
			state.Error = "unreachable"
		}
		return state
	}
	states := []runtimestate.BackendRuntimeState{worker("modeld", 32768, false), worker("modeld", 8192, false), worker("modeld", 1024, true), worker("openai", 2048, false)}
	for i := 0; i < 2; i++ {
		if got := minimumModelContext(states, "modeld", "same-model"); got != 8192 {
			t.Fatalf("cap=%d", got)
		}
		states[0], states[1] = states[1], states[0]
	}
	if got := minimumModelContext(states, "anthropic", "same-model"); got != 0 {
		t.Fatalf("cross-provider fallback: %d", got)
	}
}
