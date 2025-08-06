package stateservice

import (
	"context"

	"github.com/contenox/runtime/runtimestate"
)

type Service interface {
	Get(ctx context.Context) (map[string]runtimestate.LLMState, error)
}

type service struct {
	state *runtimestate.State
}

// Get implements Service.
func (s *service) Get(ctx context.Context) (map[string]runtimestate.LLMState, error) {
	return s.state.Get(ctx), nil
}

func New(state *runtimestate.State) Service {
	return &service{
		state: state,
	}
}
