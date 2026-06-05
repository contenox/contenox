// Package anthropic is a direct (non-Vertex) provider for the Anthropic API
// (api.anthropic.com), which speaks the Messages API. It reuses the shared
// messages codec; only the transport (x-api-key + anthropic-version header,
// model in body, vendor base URL) is Anthropic-specific.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	modelrepo.RegisterCatalogProvider("anthropic", func(spec modelrepo.BackendSpec, opts modelrepo.CatalogOptions) (modelrepo.CatalogProvider, error) {
		client := opts.HTTPClient
		if client == nil {
			client = http.DefaultClient
		}
		return &catalogProvider{spec: spec, httpClient: client, tracker: opts.Tracker}, nil
	})
}

func (p *catalogProvider) Type() string { return "anthropic" }

func (p *catalogProvider) baseURL() string {
	if base := strings.TrimSpace(p.spec.BaseURL); base != "" {
		return base
	}
	return defaultBaseURL
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modelrepo.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if p.spec.APIKey != "" {
		req.Header.Set("x-api-key", p.spec.APIKey)
	}
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic catalog returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode anthropic catalog response: %w", err)
	}
	models := make([]modelrepo.ObservedModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		// All Claude models are chat models with streaming.
		models = append(models, modelrepo.ObservedModel{
			Name: item.ID,
			CapabilityConfig: modelrepo.CapabilityConfig{
				CanChat:   true,
				CanStream: true,
				CanPrompt: true,
				CanThink:  anthropicModelCanThink(item.ID),
			},
		})
	}
	return models, nil
}

func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewAnthropicProvider(p.spec.APIKey, model.Name, []string{p.baseURL()}, model.CapabilityConfig, p.httpClient, p.tracker)
}
