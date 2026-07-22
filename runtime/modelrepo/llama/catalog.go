package llama

import (
	"context"
	"fmt"
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
	var omitted int
	var lastDescribeErr error
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
		if mmprojPresent(dir) {
			caps.CanVision = true
		}
		modelDigest := profile.ModelDigest
		if modelDigest == "" {
			modelDigest, err = modelFileDigest(modelPath)
			if err != nil {
				return nil, err
			}
		}
		adapters, err := resolveProfileAdapters(dir, profile.Adapters)
		if err != nil {
			return nil, err
		}
		// Context window is modeld's logical planner decision when available. The
		// dense effective context remains the hot KV budget, but a configured
		// host-cold budget lets the runtime assemble longer contexts that modeld
		// parks cold behind the session boundary.
		if sessionFactory == nil {
			info, derr := modeldconn.Describe(ctx, modeldconn.ModelRef{Name: e.Name(), Type: "llama", Digest: modelDigest, Path: modelPath, Adapters: adapters}, transport.Config(profile.describeConfig()))
			if derr == nil {
				applyModeldTemplateCapabilities(&profile, info)
				caps.CanThink = caps.CanThink || profile.Reasoning.Protocol != ""
				// Vision is assigned, not OR'd: modeld's Describe answer is
				// authoritative and must be able to downgrade the offline
				// mmproj-presence signal — an older daemon without vision
				// support reports false and would silently drop image parts.
				caps.CanVision = info.SupportsVision
			}
			switch {
			case derr == nil && info.PlannerEffectiveContext > 0:
				caps.ContextLength = info.PlannerEffectiveContext
			case derr == nil && info.EffectiveContext > 0:
				caps.ContextLength = info.EffectiveContext
			case modeldconn.Available():
				// modeld is live but cannot describe THIS model — it is genuinely
				// unusable, so omit it rather than advertise a broken model.
				omitted++
				lastDescribeErr = derr
				continue
			default:
				// modeld is momentarily gone (lease gap / restart); keep the model
				// listed with profile caps so the picker does not flap. The next
				// reconcile against a live daemon fills in the physical context.
			}
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
	// Omitting individually broken models is fine, but if a live modeld could
	// describe none of them the backend is misconfigured, not empty — surface
	// the diagnosis instead of an empty catalog.
	if len(out) == 0 && omitted > 0 {
		return nil, fmt.Errorf("modeld is live but could not describe any of the %d llama model(s) in %s: %w", omitted, c.dir, lastDescribeErr)
	}
	return out, nil
}

func (c *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return newProvider(model.Name, c.dir, model.CapabilityConfig)
}
