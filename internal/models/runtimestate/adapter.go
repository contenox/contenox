package runtimestate

import (
	"context"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM).
func LocalProviderAdapter(ctx context.Context, tracker libtracker.ActivityTracker, runtime map[string]BackendRuntimeState) ProviderFromRuntimeState {
	// One flat list of providers, one per model per backend.
	providersByType := make(map[string][]modelrepo.Provider)

	for _, state := range runtime {
		if state.Error != "" {
			continue
		}

		backendType := modelrepo.CanonicalBackendType(state.Backend.Type)
		catalog, err := modelrepo.NewCatalogProvider(
			modelrepo.BackendSpec{
				Type:    backendType,
				BaseURL: state.Backend.BaseURL,
				APIKey:  state.GetAPIKey(),
			},
			modelrepo.WithCatalogHTTPClient(modelrepo.SharedHTTPClient),
			modelrepo.WithCatalogTracker(tracker),
			modelrepo.WithCatalogAuthorizer(state.authorize),
		)
		if err != nil {
			continue
		}
		if _, ok := providersByType[backendType]; !ok {
			providersByType[backendType] = []modelrepo.Provider{}
		}

		for _, model := range state.PulledModels {
			p := catalog.ProviderFor(observedModelFromPullStatus(model))
			if p == nil {
				// A catalog that cannot build an execution provider for one of
				// its own observed models must not poison the candidate list.
				continue
			}
			providersByType[backendType] = append(providersByType[backendType], p)
		}
	}

	return func(ctx context.Context, backendTypes ...string) ([]modelrepo.Provider, error) {
		// No specific backend types requested: return providers from every type.
		hasNonEmpty := false
		for _, bt := range backendTypes {
			if bt != "" {
				hasNonEmpty = true
				break
			}
		}
		if !hasNonEmpty {
			var all []modelrepo.Provider
			for _, providers := range providersByType {
				all = append(all, providers...)
			}
			return all, nil
		}
		var providers []modelrepo.Provider
		for _, backendType := range backendTypes {
			backendType = modelrepo.CanonicalBackendType(backendType)
			if typeProviders, ok := providersByType[backendType]; ok {
				providers = append(providers, typeProviders...)
			}
		}
		return providers, nil
	}
}

// ProviderFromRuntimeState retrieves available model providers
type ProviderFromRuntimeState func(ctx context.Context, backendTypes ...string) ([]modelrepo.Provider, error)
