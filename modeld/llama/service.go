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

// Describe reports the model's trained context window read from the GGUF header
// (no tensor load). The runtime consumes this as the model's capacity; it never
// reads the GGUF itself.
func (s *Service) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	if req.Type != "" && req.Type != "llama" {
		return transport.ModelInfo{}, fmt.Errorf("%w: requested %q, this daemon serves llama", transport.ErrBackendMismatch, req.Type)
	}
	return transport.ModelInfo{EffectiveContext: ggufContextLength(req.Path)}, nil
}
