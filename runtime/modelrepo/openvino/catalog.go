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
		if _, ok := modelEntrypointPath(modelPath); !ok {
			continue
		}
		profile, err := loadModelProfile(modelPath)
		if err != nil {
			return nil, err
		}
		caps := profile.capabilityConfig()
		modelDigest, _ := modelIdentity(modelPath)
		// Context window is modeld's physical hot-context decision. Profile config
		// is only the request/cap; the daemon may reduce it for device memory or a
		// user memory ceiling.
		if sessionFactory == nil {
			mi, derr := modeldconn.Describe(ctx, modeldconn.ModelRef{Name: e.Name(), Type: "openvino", Digest: modelDigest, Path: modelPath}, Config{NumCtx: caps.ContextLength})
			switch {
			case derr == nil && mi.EffectiveContext > 0:
				caps.ContextLength = mi.EffectiveContext
			case modeldconn.Available():
				// modeld is live but cannot describe THIS model — it is genuinely
				// unusable, so omit it rather than advertise a broken model.
				continue
			default:
				// modeld is momentarily gone (lease gap / restart); keep the model
				// listed with profile caps so the picker does not flap. The next
				// reconcile against a live daemon fills in the physical context.
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
