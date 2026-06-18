package llama

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/runtime/transport"
)

// Service implements the runtime/transport.Service boundary.
// It acts as the opener for native llama.cpp backend sessions.
type Service struct{}

var _ transport.Service = (*Service)(nil)

// OpenSession binds a session to the requested model. It rejects a model typed
// for a different backend (ErrBackendMismatch) before loading, so a GGUF request
// sent to an openvino-mode daemon — or vice versa — fails at the boundary, not
// deep in the engine. The model is loaded from req.Path (resolved by the
// runtime); identity/caching uses req.Digest.
func (s *Service) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if req.Type != "" && req.Type != "llama" {
		return nil, fmt.Errorf("%w: requested %q, this daemon serves llama", transport.ErrBackendMismatch, req.Type)
	}
	return newSession(req.Path, req.Config)
}
