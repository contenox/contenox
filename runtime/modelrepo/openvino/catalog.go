package openvino

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

func init() {
	modelrepo.RegisterCatalogProvider("openvino", func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		tracker := opts.Tracker
		if tracker == nil {
			tracker = libtracker.NoopTracker{}
		}
		return nil, fmt.Errorf("openvino backend is temporarily disabled pending modeld local runtime transition")
	})
	// Native GenAI sessions hold process-lifetime KV/pipeline state; register a
	// drain so the runtime can release them on shutdown without importing this
	// backend directly. No-op in builds without the native backend.
	modelrepo.RegisterShutdownHook(ShutdownGenAISessions)
}

type catalogProvider struct {
	dir     string
	tracker libtracker.ActivityTracker
}

func (c *catalogProvider) Type() string { return "openvino" }

func (c *catalogProvider) ListModels(_ context.Context) ([]modelrepo.ObservedModel, error) {
	// When the native backend isn't compiled in, advertise nothing rather than
	// list models that cannot run.
	if !ovsession.Available {
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
				"format":         "openvino-ir",
				"native_session": "true",
				"profile":        "defaults-or-" + profileFileName,
			},
		})
	}
	return out, nil
}

func (c *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return newProvider(model.Name, c.dir, model.CapabilityConfig, c.tracker)
}
