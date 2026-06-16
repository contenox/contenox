package ollama

import (
	"context"
	"net/http"
	"strings"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
	"github.com/ollama/ollama/api"
	ollamamodel "github.com/ollama/ollama/types/model"
)

const displayNameMetaKey = "display_name"

type catalogProvider struct {
	spec       modeld.BackendSpec
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func init() {
	modeld.RegisterCatalogProvider("ollama", func(spec modeld.BackendSpec, opts modeld.CatalogOptions) (modeld.CatalogProvider, error) {
		return newCatalogProvider(spec, opts), nil
	})
}

func newCatalogProvider(spec modeld.BackendSpec, opts modeld.CatalogOptions) modeld.CatalogProvider {
	return &catalogProvider{
		spec:       spec,
		httpClient: opts.HTTPClient,
		tracker:    opts.Tracker,
	}
}

func (p *catalogProvider) Type() string {
	return "ollama"
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modeld.ObservedModel, error) {
	client, err := newOllamaHTTPClient(p.spec.BaseURL, p.spec.APIKey, p.httpClient)
	if err != nil {
		return nil, err
	}

	resp, err := client.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]modeld.ObservedModel, 0, len(resp.Models))
	for _, model := range resp.Models {
		observed := modeld.ObservedModel{
			Name:       model.Model,
			ModifiedAt: model.ModifiedAt,
			Size:       model.Size,
			Digest:     model.Digest,
			Meta: map[string]string{
				displayNameMetaKey: model.Name,
			},
		}

		if showResp, err := client.Show(ctx, &api.ShowRequest{Model: model.Model}); err == nil {
			applyShowMetadata(&observed, showResp)
		}

		out = append(out, observed)
	}

	return out, nil
}

func (p *catalogProvider) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return NewOllamaProvider(
		model.Name,
		[]string{p.spec.BaseURL},
		p.httpClient,
		model.CapabilityConfig,
		p.spec.APIKey,
		p.tracker,
	)
}

func applyShowMetadata(model *modeld.ObservedModel, resp *api.ShowResponse) {
	for _, cap := range resp.Capabilities {
		switch cap {
		case ollamamodel.CapabilityCompletion:
			model.CanChat = true
			model.CanPrompt = true
			model.CanStream = true
		case ollamamodel.CapabilityEmbedding:
			model.CanEmbed = true
		case ollamamodel.CapabilityTools:
			model.CanChat = true
		}
	}

	if model.ContextLength == 0 {
		for key, value := range resp.ModelInfo {
			if !strings.HasSuffix(key, ".context_length") {
				continue
			}
			if n, ok := value.(float64); ok {
				model.ContextLength = int(n)
			}
			break
		}
	}
}
