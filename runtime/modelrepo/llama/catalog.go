package llama

import (
	"context"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

func init() {
	modelrepo.RegisterCatalogProvider("llama", func(spec modelrepo.BackendSpec, _ modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		return &catalogProvider{dir: spec.BaseURL}, nil
	})
}

type catalogProvider struct {
	dir string
}

func (c *catalogProvider) Type() string { return "llama" }

func (c *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	// Advertise nothing when the native backend is not compiled in.
	if !SessionAvailable() {
		return nil, nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	var out []modelrepo.ObservedModel
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.dir, e.Name())
		modelPath := filepath.Join(dir, "model.gguf")
		if _, err := os.Stat(modelPath); err != nil {
			continue
		}
		profile, err := loadModelProfile(dir)
		if err != nil {
			return nil, err
		}
		caps := profile.capabilityConfig()
		// Context window is modeld's physical hot-context decision. Profile config
		// is only the request/cap; the daemon may reduce it for device memory or a
		// user memory ceiling.
		if info, derr := modeldconn.Describe(ctx, modeldconn.ModelRef{Name: e.Name(), Type: "llama", Path: modelPath}, transport.Config(profile.config())); derr == nil && info.EffectiveContext > 0 {
			caps.ContextLength = info.EffectiveContext
		}
		info, _ := e.Info()
		out = append(out, modelrepo.ObservedModel{
			Name: e.Name(),
			// ObservedModel has both top-level fields and an embedded
			// CapabilityConfig. Fill both so runtime-state normalization does
			// not drop the tested profile limits.
			ContextLength:    caps.ContextLength,
			CapabilityConfig: caps,
			ModifiedAt:       info.ModTime(),
			Meta: map[string]string{
				"format":  "gguf",
				"runtime": "llamacpp",
				"node":    "llama",
			},
		})
	}
	return out, nil
}

func (c *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return newProvider(model.Name, c.dir, model.CapabilityConfig)
}
