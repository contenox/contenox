package llama

import (
	"context"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/modeld"
)

func init() {
	modeld.RegisterCatalogProvider("llama", func(spec modeld.BackendSpec, _ modeld.CatalogOptions) (modeld.CatalogProvider, error) {
		return &catalogProvider{dir: spec.BaseURL}, nil
	})
}

type catalogProvider struct {
	dir string
}

func (c *catalogProvider) Type() string { return "llama" }

func (c *catalogProvider) ListModels(_ context.Context) ([]modeld.ObservedModel, error) {
	// Advertise nothing when the native backend is not compiled in.
	if !SessionAvailable() {
		return nil, nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	var out []modeld.ObservedModel
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.dir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "model.gguf")); err != nil {
			continue
		}
		profile, err := loadModelProfile(dir)
		if err != nil {
			return nil, err
		}
		caps := profile.capabilityConfig()
		info, _ := e.Info()
		out = append(out, modeld.ObservedModel{
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

func (c *catalogProvider) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return newProvider(model.Name, c.dir, model.CapabilityConfig)
}
