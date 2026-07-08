package bedrock

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
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

func (p *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(regionFromURL(p.spec.BaseURL)))
	if err != nil {
		return nil, err
	}

	client := bedrock.NewFromConfig(cfg)

	output, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list foundation models: %w", err)
	}

	var models []modelrepo.ObservedModel
	for _, summary := range output.ModelSummaries {
		modelID := *summary.ModelId
		isEmbed := strings.Contains(strings.ToLower(modelID), "embed")

		models = append(models, modelrepo.ObservedModel{
			Name: modelID,
			CapabilityConfig: modelrepo.CapabilityConfig{
				CanChat:   !isEmbed,
				CanStream: !isEmbed,
				CanPrompt: !isEmbed,
				CanEmbed:  isEmbed,
			},
		})
	}

	return models, nil
}

func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewBedrockProvider(
		regionFromURL(p.spec.BaseURL),
		p.spec.APIKey,
		model.Name,
		model.CapabilityConfig,
		p.httpClient,
		p.tracker,
	)
}
