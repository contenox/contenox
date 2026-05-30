package bedrock

import (
	"context"
	"net/http"
	"strings"

	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/modelrepo"
)

type catalogProvider struct {
	spec       modelrepo.BackendSpec
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func init() {
	modelrepo.RegisterCatalogProvider("bedrock", func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		return &catalogProvider{spec: spec, httpClient: opts.HTTPClient, tracker: opts.Tracker}, nil
	})
}

func (p *catalogProvider) Type() string { return "bedrock" }

// commonConverseModels is a curated discovery list of widely-available
// Converse-capable Bedrock model IDs. It is a hint, not a limit: ProviderFor
// builds a working provider for ANY model id / inference-profile id the user
// sets as default-model, so region-specific profile ids (e.g. "us.anthropic...")
// work even when not listed here.
//
// NOTE: Bedrock requires per-account model enablement in the console. Until a
// model is enabled, invoking it returns AccessDeniedException even though it
// lists here. (A future enhancement can replace this static list with
// bedrock.ListFoundationModels for live, account-aware discovery.)
var commonConverseModels = []string{
	"anthropic.claude-3-5-sonnet-20241022-v2:0",
	"anthropic.claude-3-5-haiku-20241022-v1:0",
	"meta.llama3-1-70b-instruct-v1:0",
	"mistral.mistral-large-2407-v1:0",
	"amazon.nova-pro-v1:0",
	"amazon.nova-lite-v1:0",
}

func (p *catalogProvider) ListModels(_ context.Context) ([]modelrepo.ObservedModel, error) {
	models := make([]modelrepo.ObservedModel, 0, len(commonConverseModels))
	for _, id := range commonConverseModels {
		om := modelrepo.ObservedModel{
			Name: id,
			CapabilityConfig: modelrepo.CapabilityConfig{
				CanChat:   true,
				CanStream: true,
				CanPrompt: true,
				CanEmbed:  strings.Contains(strings.ToLower(id), "embed"),
			},
		}
		models = append(models, om)
	}
	return models, nil
}

func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewBedrockProvider(
		regionFromURL(p.spec.BaseURL),
		p.spec.APIKey, // optional stored static-credentials JSON; empty → ambient chain
		model.Name,
		model.CapabilityConfig,
		p.httpClient,
		p.tracker,
	)
}
