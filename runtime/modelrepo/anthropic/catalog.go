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
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
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
	return p.listModels(ctx, "/v1/models")
}

type modelListResponse struct {
	Data []modelInfo `json:"data"`
}

type modelInfo struct {
	ID             string             `json:"id"`
	CreatedAt      string             `json:"created_at"`
	MaxInputTokens int                `json:"max_input_tokens"`
	Capabilities   *modelCapabilities `json:"capabilities"`
}

type modelCapabilities struct {
	Thinking thinkingCapability `json:"thinking"`
	Effort   effortCapability   `json:"effort"`
}

type thinkingCapability struct {
	Supported bool          `json:"supported"`
	Types     thinkingTypes `json:"types"`
}

type thinkingTypes struct {
	Adaptive capabilitySupport `json:"adaptive"`
	Enabled  capabilitySupport `json:"enabled"`
}

type effortCapability struct {
	Supported bool              `json:"supported"`
	Low       capabilitySupport `json:"low"`
	Medium    capabilitySupport `json:"medium"`
	High      capabilitySupport `json:"high"`
	XHigh     capabilitySupport `json:"xhigh"`
	Max       capabilitySupport `json:"max"`
}

type capabilitySupport struct {
	Supported bool `json:"supported"`
}

func (p *catalogProvider) listModels(ctx context.Context, path string) ([]modelrepo.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+path, nil)
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
		return nil, fmt.Errorf("anthropic catalog %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload modelListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode anthropic catalog response: %w", err)
	}
	models := make([]modelrepo.ObservedModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		// All Claude models are chat models with streaming.
		modifiedAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
		models = append(models, modelrepo.ObservedModel{
			Name:          item.ID,
			ContextLength: item.MaxInputTokens,
			ModifiedAt:    modifiedAt,
			CapabilityConfig: modelrepo.CapabilityConfig{
				ContextLength: item.MaxInputTokens,
				CanChat:       true,
				CanStream:     true,
				CanPrompt:     true,
				CanThink:      anthropicCapabilitiesCanThink(item.Capabilities),
			},
		})
	}
	return models, nil
}

func anthropicCapabilitiesCanThink(caps *modelCapabilities) bool {
	if caps == nil {
		return false
	}
	thinking := caps.Thinking
	if thinking.Supported || thinking.Types.Adaptive.Supported || thinking.Types.Enabled.Supported {
		return true
	}
	effort := caps.Effort
	return effort.Supported || effort.Low.Supported || effort.Medium.Supported || effort.High.Supported || effort.XHigh.Supported || effort.Max.Supported
}

func (p *catalogProvider) ProviderFor(model modelrepo.ObservedModel) modelrepo.Provider {
	return NewAnthropicProvider(p.spec.APIKey, model.Name, []string{p.baseURL()}, model.CapabilityConfig, p.httpClient, p.tracker)
}
