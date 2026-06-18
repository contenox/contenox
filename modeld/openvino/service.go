package openvino

import (
	"context"
	"fmt"
	"os"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/runtime/transport"
)

// Service implements the runtime/transport.Service boundary for the OpenVINO
// GenAI backend. It opens persistent, manifest-keyed sessions on the owned
// device (CPU / GPU / NPU); the runtime reaches it as a client over the
// transport and never imports this package.
type Service struct{}

var _ transport.Service = (*Service)(nil)

// OpenSession makes the model at req.Path (an OpenVINO IR directory, resolved by
// the runtime) resident and returns a session bound to it. It rejects a model
// typed for a different backend (ErrBackendMismatch) before loading, so a request
// for a llama model on an openvino-mode daemon fails at the boundary. In a build
// without the openvino + openvino_genai tags, ovsession.NewGenAI reports the
// backend is not compiled in and that error surfaces here unchanged.
func (s *Service) OpenSession(_ context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	if req.Type != "" && req.Type != "openvino" {
		return nil, fmt.Errorf("%w: requested %q, this daemon serves openvino", transport.ErrBackendMismatch, req.Type)
	}
	// The OpenVINO-specific tuning (KV precision, sparse attention, cache size) is
	// model-driven: read from the model's own contenox-openvino.json profile, not
	// hardcoded. transport.Config carries only the neutral context window; the
	// device (incl. NPU) is resolved from the environment.
	backend, err := ovsession.NewGenAI(req.Path, genAIConfigFromProfile(req.Path, resolveDevice()))
	if err != nil {
		return nil, err
	}
	return newGenaiSession(backend, req.Config.NumCtx), nil
}

// resolveDevice selects the OpenVINO inference device. CONTENOX_OPENVINO_DEVICE
// is the explicit override (set it to NPU on an Intel NPU node); the test device
// hint and a CPU default follow.
func resolveDevice() string {
	if device := os.Getenv("CONTENOX_OPENVINO_DEVICE"); device != "" {
		return device
	}
	if device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE"); device != "" {
		return device
	}
	return "CPU"
}
