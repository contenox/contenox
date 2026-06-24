package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/statetype"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, tracker libtracker.ActivityTracker, runtime map[string]statetype.BackendRuntimeState) ProviderFromRuntimeState {
	// Create a flat list of providers (one per model per backend)
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
			modelrepo.WithCatalogHTTPClient(http.DefaultClient),
			modelrepo.WithCatalogTracker(tracker),
		)
		if err != nil {
			continue
		}
		if _, ok := providersByType[backendType]; !ok {
			providersByType[backendType] = []modelrepo.Provider{}
		}

		for _, model := range state.PulledModels {
			providersByType[backendType] = append(
				providersByType[backendType],
				catalog.ProviderFor(observedModelFromPullStatus(model)),
			)
		}
	}

	// Collapse the local modeld family into one logical provider. modeld is a
	// single daemon whose engine is autodetected, so only the live engine's
	// catalog yields providers (the dormant format reconciles to an error/empty
	// entry); localProviders is therefore exactly what modeld can serve now.
	// Resolving any local alias to this set means the user's llama-vs-openvino
	// pick (and the two registered rows) no longer has to match the live engine.
	var localProviders []modelrepo.Provider
	for backendType, typeProviders := range providersByType {
		if modelrepo.IsLocalBackendType(backendType) {
			localProviders = append(localProviders, typeProviders...)
		}
	}

	return func(ctx context.Context, backendTypes ...string) ([]modelrepo.Provider, error) {
		// If no specific backend types requested (or only empty strings from an
		// unconfigured default-provider), return providers from ALL backend types.
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
		localAdded := false
		for _, backendType := range backendTypes {
			// Any local alias resolves to the live modeld engine's providers,
			// added at most once even if several local types are requested.
			if modelrepo.IsLocalBackendType(backendType) {
				if !localAdded {
					providers = append(providers, localProviders...)
					localAdded = true
				}
				continue
			}
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
