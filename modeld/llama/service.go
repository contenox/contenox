package llama

import (
	"context"

	"github.com/contenox/runtime/runtime/transport"
)

// Service implements the runtime/transport.Service boundary.
// It acts as the opener for native llama.cpp backend sessions.
type Service struct{}

var _ transport.Service = (*Service)(nil)

// OpenSession binds a session to the configured model and config.
func (s *Service) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	return newSession(req.ModelID, req.Config)
}
