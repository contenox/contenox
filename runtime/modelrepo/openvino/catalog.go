package openvino

import (
	"context"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
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

func (c *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
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
		caps := profile.capabilityConfig()
		// Context window is a model fact owned by modeld (it loads the IR). The
		// runtime never reads config.json; it asks modeld. A profile-declared value
		// is an explicit cap and wins; otherwise use the model's reported capacity.
		if caps.ContextLength == 0 {
			if mi, derr := modeldconn.Describe(ctx, modeldconn.ModelRef{Name: e.Name(), Type: "openvino", Path: modelPath}); derr == nil && mi.EffectiveContext > 0 {
				caps.ContextLength = mi.EffectiveContext
			}
		}
		fi, _ := e.Info()
		out = append(out, modelrepo.ObservedModel{
			Name:             e.Name(),
			ContextLength:    caps.ContextLength,
			CapabilityConfig: caps,
			ModifiedAt:       fi.ModTime(),
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
