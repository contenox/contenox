package gemini

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

const defaultBaseURL = "https://generativelanguage.googleapis.com"

type catalogProvider struct {
	spec       modeld.BackendSpec
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func init() {
	modeld.RegisterCatalogProvider("gemini", func(spec modeld.BackendSpec, opts modeld.CatalogOptions) (modeld.CatalogProvider, error) {
		return &catalogProvider{
			spec:       spec,
			httpClient: opts.HTTPClient,
			tracker:    opts.Tracker,
		}, nil
	})
}

func (p *catalogProvider) Type() string {
	return "gemini"
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modeld.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/v1beta/models", nil)
	if err != nil {
		return nil, err
	}
	if p.spec.APIKey != "" {
		req.Header.Set("X-Goog-Api-Key", p.spec.APIKey)
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
		return nil, fmt.Errorf("Gemini catalog returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Gemini catalog response: %w", err)
	}

	models := make([]modeld.ObservedModel, 0, len(payload.Models))
	for _, item := range payload.Models {
		observed, err := p.describeModel(ctx, item.Name)
		if err != nil {
			return nil, err
		}
		models = append(models, observed)
	}
	return models, nil
}

func (p *catalogProvider) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return NewGeminiProvider(
		p.spec.APIKey,
		model.Name,
		[]string{p.baseURL()},
		model.CapabilityConfig,
		p.httpClient,
		p.tracker,
	)
}

func (p *catalogProvider) baseURL() string {
	base := strings.TrimSpace(p.spec.BaseURL)
	if base == "" {
		return defaultBaseURL
	}
	return base
}

func (p *catalogProvider) describeModel(ctx context.Context, modelName string) (modeld.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1beta/%s", strings.TrimRight(p.baseURL(), "/"), modelName), nil)
	if err != nil {
		return modeld.ObservedModel{}, err
	}
	if p.spec.APIKey != "" {
		req.Header.Set("X-Goog-Api-Key", p.spec.APIKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return modeld.ObservedModel{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return modeld.ObservedModel{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return modeld.ObservedModel{}, fmt.Errorf("Gemini describe returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Name                       string   `json:"name"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		Thinking                   bool     `json:"thinking"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return modeld.ObservedModel{}, fmt.Errorf("decode Gemini model response: %w", err)
	}

	observed := modeld.ObservedModel{
		Name:          modelName,
		ContextLength: payload.InputTokenLimit,
		CapabilityConfig: modeld.CapabilityConfig{
			ContextLength:   payload.InputTokenLimit,
			MaxOutputTokens: payload.OutputTokenLimit,
		},
	}
	observed.CanThink = payload.Thinking
	for _, method := range payload.SupportedGenerationMethods {
		switch method {
		case "generateContent":
			observed.CanChat = true
			observed.CanPrompt = true
			observed.CanStream = true
		case "embedContent":
			observed.CanEmbed = true
		}
	}

	return observed, nil
}
