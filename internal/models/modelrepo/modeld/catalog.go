package modeld

import (
	"context"
	"fmt"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/transport"
	"github.com/contenox/contenox/libtracker"
)

type catalogProvider struct {
	spec    modelrepo.BackendSpec
	tracker libtracker.ActivityTracker
}

func init() {
	constructor := func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		return newCatalogProvider(spec, opts), nil
	}
	modelrepo.RegisterCatalogProvider("modeld", constructor)
	modelrepo.RegisterCatalogProvider("local", constructor)
}
func newCatalogProvider(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) modelrepo.CatalogProvider {
	return &catalogProvider{spec: spec, tracker: opts.Tracker}
}
func (p *catalogProvider) Type() string { return "modeld" }
func (p *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	ctx, cancel := modelrepo.NonStreamingContext(ctx)
	defer cancel()
	target := resolveTarget(p.spec.BaseURL)
	models, err := modeldconn.ListModelsTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	out := make([]modelrepo.ObservedModel, 0, len(models))
	for _, m := range models {
		info, err := modeldconn.DescribeTarget(ctx, target, modeldconn.ModelRef{Name: m.Name, Type: m.Type, Digest: m.Digest}, transport.Config{})
		if err != nil {
			return nil, fmt.Errorf("describe modeld model %s: %w", m.Name, err)
		}
		out = append(out, modelrepo.ObservedModel{Name: m.Name, ContextLength: info.EffectiveContext, Size: m.SizeBytes, Digest: m.Digest, CapabilityConfig: modelrepo.CapabilityConfig{ContextLength: info.EffectiveContext, CanChat: true, CanPrompt: true, CanStream: true, CanThink: info.ChatTemplateSupportsThinking, CanVision: info.SupportsVision}, Meta: map[string]string{"type": m.Type}})
	}
	return out, nil
}
func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewModeldProvider(model.Name, []string{p.spec.BaseURL}, model.CapabilityConfig, p.tracker)
}
