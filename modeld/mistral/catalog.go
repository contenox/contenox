// Package mistral is a direct (non-Vertex) provider for the Mistral API
// (api.mistral.ai), which speaks the OpenAI-compatible chat/completions format.
// It reuses the shared chatcompletions codec; only the transport (API-key auth,
// vendor base URL) is Mistral-specific.
package mistral

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
)

type catalogProvider struct {
	spec       modeld.BackendSpec
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func init() {
	modeld.RegisterCatalogProvider("mistral", func(spec modeld.BackendSpec, opts modeld.CatalogOptions) (modeld.CatalogProvider, error) {
		client := opts.HTTPClient
		if client == nil {
			client = http.DefaultClient
		}
		return &catalogProvider{spec: spec, httpClient: client, tracker: opts.Tracker}, nil
	})
}

func (p *catalogProvider) Type() string { return "mistral" }

func (p *catalogProvider) baseURL() string {
	if base := strings.TrimSpace(p.spec.BaseURL); base != "" {
		return base
	}
	return defaultBaseURL
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modeld.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if p.spec.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.spec.APIKey)
	}
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
		return nil, fmt.Errorf("mistral catalog returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []struct {
			ID              string `json:"id"`
			MaxOutputTokens int    `json:"max_output_tokens"`
			MaxTokens       int    `json:"max_tokens"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode mistral catalog response: %w", err)
	}
	models := make([]modeld.ObservedModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		model := inferObservedModel(item.ID)
		model.MaxOutputTokens = item.MaxOutputTokens
		if model.MaxOutputTokens <= 0 {
			model.MaxOutputTokens = item.MaxTokens
		}
		models = append(models, model)
	}
	return models, nil
}

func (p *catalogProvider) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return NewMistralProvider(p.spec.APIKey, model.Name, []string{p.baseURL()}, model.CapabilityConfig, p.httpClient, p.tracker)
}

func inferObservedModel(id string) modeld.ObservedModel {
	om := modeld.ObservedModel{Name: id}
	if strings.Contains(strings.ToLower(id), "embed") {
		om.CanEmbed = true
		return om
	}
	om.CanChat = true
	om.CanPrompt = true
	om.CanStream = true
	return om
}
