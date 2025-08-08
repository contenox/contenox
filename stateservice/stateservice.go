package stateservice

import (
	"context"

	"github.com/contenox/runtime/runtimestate"
)

type Service interface {
	Get(ctx context.Context) ([]runtimestate.LLMState, error)
}

type service struct {
	state *runtimestate.State
}

// Get implements Service.
func (s *service) Get(ctx context.Context) ([]runtimestate.LLMState, error) {
	m := s.state.Get(ctx)
	l := make([]runtimestate.LLMState, 0, len(m))
	for _, e := range m {
		l = append(l, e)
	}
	return l, nil
}

func New(state *runtimestate.State) Service {
	return &service{
		state: state,
	}
}
