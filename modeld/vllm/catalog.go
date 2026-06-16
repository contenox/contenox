package vllm

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
	modeld.RegisterCatalogProvider("vllm", func(spec modeld.BackendSpec, opts modeld.CatalogOptions) (modeld.CatalogProvider, error) {
		return &catalogProvider{
			spec:       spec,
			httpClient: opts.HTTPClient,
			tracker:    opts.Tracker,
		}, nil
	})
}

func (p *catalogProvider) Type() string {
	return "vllm"
}

func (p *catalogProvider) ListModels(ctx context.Context) ([]modeld.ObservedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.spec.BaseURL, "/")+"/v1/models", nil)
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
		return nil, fmt.Errorf("vLLM catalog returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			MaxModelLen int    `json:"max_model_len"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode vLLM catalog response: %w", err)
	}

	models := make([]modeld.ObservedModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		models = append(models, modeld.ObservedModel{
			Name:          item.ID,
			ContextLength: item.MaxModelLen,
			CapabilityConfig: modeld.CapabilityConfig{
				ContextLength: item.MaxModelLen,
				CanChat:       true,
				CanPrompt:     true,
				CanStream:     true,
			},
		})
	}
	return models, nil
}

func (p *catalogProvider) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return NewVLLMProvider(
		model.Name,
		[]string{p.spec.BaseURL},
		p.httpClient,
		model.CapabilityConfig,
		p.spec.APIKey,
		p.tracker,
	)
}
