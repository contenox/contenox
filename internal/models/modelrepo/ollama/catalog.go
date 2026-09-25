package ollama

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
)

const displayNameMetaKey = "display_name"

type catalogProvider struct {
	spec       modelrepo.BackendSpec
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func init() {
	modelrepo.RegisterCatalogProvider("ollama", func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		return newCatalogProvider(spec, opts), nil
	})
}

func newCatalogProvider(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) modelrepo.CatalogProvider {
	return &catalogProvider{
		spec:       spec,
		httpClient: opts.HTTPClient,
		tracker:    opts.Tracker,
	}
}

func (p *catalogProvider) Type() string {
	return "ollama"
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	// Catalog listing is a non-streaming call: bound it end-to-end.
	ctx, cancel := modelrepo.NonStreamingContext(ctx)
	defer cancel()

	client, err := newOllamaHTTPClient(p.spec.BaseURL, p.spec.APIKey, p.httpClient)
	if err != nil {
		return nil, err
	}

	speaksExtensions, speaksSession := false, false
	if endpoint, err := client.Contenox(ctx); err == nil && endpoint.IsContenoxGateway() {
		speaksExtensions = endpoint.Supports(ExtensionAudios)
		speaksSession = endpoint.Supports(ExtensionSession)
	}

	resp, err := client.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]modelrepo.ObservedModel, 0, len(resp.Models))
	for _, model := range resp.Models {
		observed := modelrepo.ObservedModel{
			Name:       model.Model,
			ModifiedAt: model.ModifiedAt,
			Size:       model.Size,
			Digest:     model.Digest,
			Meta: map[string]string{
				displayNameMetaKey: model.Name,
			},
		}

		observed.CapabilityConfig.AudioExtension = speaksExtensions
		observed.CapabilityConfig.SessionExtension = speaksSession

		if showResp, err := client.Show(ctx, &ShowRequest{Model: model.Model}); err == nil {
			applyShowMetadata(&observed, showResp, speaksExtensions)
		}

		out = append(out, observed)
	}

	return out, nil
}

func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewOllamaProvider(
		model.Name,
		[]string{p.spec.BaseURL},
		p.httpClient,
		model.CapabilityConfig,
		p.spec.APIKey,
		p.tracker,
	)
}

func applyShowMetadata(model *modelrepo.ObservedModel, resp *ShowResponse, audioExtension bool) {
	for _, cap := range resp.Capabilities {
		switch cap {
		case CapabilityCompletion:
			model.CanChat = true
			model.CanPrompt = true
			model.CanStream = true
		case CapabilityEmbedding:
			model.CanEmbed = true
		case CapabilityTools:
			model.CanChat = true
		case CapabilityVision:
			model.CanVision = true
		case CapabilityThinking:
			model.CanThink = true
		case CapabilityAudio:
			model.CanAudio = audioExtension
		}
	}

	// Both fields carry the window and both are read: the ObservedModel one by
	// the runtime state, the CapabilityConfig one by every provider built from
	// this observation. Setting only the first left a provider reporting an
	// unknown window for a model the catalogue had just measured.
	if model.ContextLength == 0 {
		for key, value := range resp.ModelInfo {
			if !strings.HasSuffix(key, ".context_length") {
				continue
			}
			if n, ok := value.(float64); ok {
				model.ContextLength = int(n)
				model.CapabilityConfig.ContextLength = int(n)
			}
			break
		}
	}

	if model.MaxOutputTokens == 0 {
		model.MaxOutputTokens = parameterInt(resp.Parameters, "num_predict")
	}
}

// parameterInt reads one parameter out of the Modelfile-style parameters string
// /api/show reports, whose lines are "<name> <value>" — "num_predict 8192",
// "num_ctx 4096". A parameter that is absent, not a number, or not positive is
// unknown rather than a ceiling of zero.
func parameterInt(parameters, name string) int {
	for _, line := range strings.Split(parameters, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != name {
			continue
		}
		if n, err := strconv.Atoi(fields[1]); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
