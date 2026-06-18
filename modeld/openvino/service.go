package openvino

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/runtime/transport"
)

// bakedTokenizersPath is the build-time fallback path to libopenvino_tokenizers.so
// (set via build-modeld -ldflags -X) for an in-place dev build whose libs live in
// the venv. OpenVINO GenAI loads that extension via OPENVINO_TOKENIZERS_PATH_GENAI.
var bakedTokenizersPath string

// tokenizersLibName is the extension file the bundle/venv provides.
const tokenizersLibName = "libopenvino_tokenizers.so"

// init points OpenVINO GenAI at the tokenizers extension without requiring the
// caller to set OPENVINO_TOKENIZERS_PATH_GENAI. It prefers a bundle next to the
// binary (bin/modeld + bin/modeld-libs/ — relocatable, the packaged daemon) and
// falls back to the build-time baked venv path (the in-place dev build).
func init() {
	if os.Getenv("OPENVINO_TOKENIZERS_PATH_GENAI") != "" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "modeld-libs", tokenizersLibName)
		if _, err := os.Stat(cand); err == nil {
			_ = os.Setenv("OPENVINO_TOKENIZERS_PATH_GENAI", cand)
			return
		}
	}
	if bakedTokenizersPath != "" {
		_ = os.Setenv("OPENVINO_TOKENIZERS_PATH_GENAI", bakedTokenizersPath)
	}
}

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

// Describe reports the model's trained context window read from the IR's
// config.json (max_position_embeddings) — no pipeline load. The runtime consumes
// this as the model's capacity; it never reads the IR files itself.
func (s *Service) Describe(_ context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	if req.Type != "" && req.Type != "openvino" {
		return transport.ModelInfo{}, fmt.Errorf("%w: requested %q, this daemon serves openvino", transport.ErrBackendMismatch, req.Type)
	}
	return transport.ModelInfo{EffectiveContext: openvinoContextLength(req.Path)}, nil
}

// openvinoContextLength reads max_position_embeddings from an OpenVINO IR model's
// config.json. Returns 0 when absent/unreadable.
func openvinoContextLength(modelDir string) int {
	b, err := os.ReadFile(filepath.Join(modelDir, "config.json"))
	if err != nil {
		return 0
	}
	var cfg struct {
		MaxPositionEmbeddings int `json:"max_position_embeddings"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return 0
	}
	return cfg.MaxPositionEmbeddings
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
