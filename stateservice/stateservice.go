package stateservice

import (
	"context"

	"github.com/contenox/runtime/internal/runtimestate"
	"github.com/contenox/runtime/statetype"
)

type Service interface {
	Get(ctx context.Context) ([]statetype.LLMState, error)
}

type service struct {
	state *runtimestate.State
}

// Get implements Service.
func (s *service) Get(ctx context.Context) ([]statetype.LLMState, error) {
	m := s.state.Get(ctx)
	l := make([]statetype.LLMState, 0, len(m))
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
