package openvino

import (
	"context"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func init() {
	modelrepo.RegisterCatalogProvider("openvino", func(spec modelrepo.BackendSpec, _ modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		return &catalogProvider{dir: spec.BaseURL}, nil
	})
}

type catalogProvider struct {
	dir string
}

func (c *catalogProvider) Type() string { return "openvino" }

func (c *catalogProvider) ListModels(_ context.Context) ([]modelrepo.ObservedModel, error) {
	// Advertise nothing unless modeld is available to serve these models.
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
		modelPath := filepath.Join(c.dir, e.Name())
		if _, err := os.Stat(filepath.Join(modelPath, "openvino_model.xml")); err != nil {
			continue
		}
		profile, err := loadModelProfile(modelPath)
		if err != nil {
			return nil, err
		}
		info, _ := e.Info()
		out = append(out, modelrepo.ObservedModel{
			Name:             e.Name(),
			CapabilityConfig: profile.capabilityConfig(),
			ModifiedAt:       info.ModTime(),
			Meta: map[string]string{
				"format":  "openvino-ir",
				"runtime": "openvino-genai",
				"node":    "openvino",
			},
		})
	}
	return out, nil
}

func (c *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return newProvider(model.Name, c.dir, model.CapabilityConfig)
}
