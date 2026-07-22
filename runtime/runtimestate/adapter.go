package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/llama"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/modelrepo/openvino"
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

	// Special support for explicit "modeld" backends (LocalSentinel or remote URL).
	// These are observed via processModeldBackend (ListModels over wire).
	// We create targeted providers so that chat/prompt/stream etc. are routed
	// to the specific node (local or remote specialist GPU box).
	// Live addr/instance/engine are resolved only during (debounced) reconcile
	// and stored in the state; we read them here to avoid any per-request
	// network I/O in the hot path.
	for _, state := range runtime {
		if state.Error != "" {
			continue
		}
		if modelrepo.CanonicalBackendType(state.Backend.Type) != "modeld" {
			continue
		}
		addr := state.ResolvedEndpoint
		if addr == "" {
			addr = state.Backend.BaseURL
		}
		if addr == modeldconn.LocalSentinel || addr == "" {
			if r, err := modeldconn.LocalEndpointAddr(ctx); err == nil {
				addr = r
			}
		}
		inst := state.ResolvedInstance
		engine := state.LiveEngine
		if engine == "" {
			engine = "llama"
		}
		tgt := modeldconn.ModeldTarget{
			BackendID: state.Backend.Name, // report the registered name for selection
			Endpoint:  addr,
			Instance:  inst,
		}
		for _, m := range state.PulledModels {
			// Reached only when state.Error == "" (checked above), i.e. ListModels
			// already succeeded against this live, single-slot, single-engine node
			// — every model it reports is chat/prompt/stream-capable by
			// construction. Embed is safe to advertise because modeldconn.EmbedTarget
			// actually routes to this specific target instead of the ambient local
			// lease (see runtime/modelrepo/llama/embed.go, openvino/client.go).
			caps := modelrepo.CapabilityConfig{
				ContextLength: m.ContextLength, // 0 ("unknown") until reconcile enriches it
				CanChat:       true,
				CanPrompt:     true,
				CanStream:     true,
				CanEmbed:      true,
				// Vision is NOT capable-by-construction: it needs the node to have
				// resolved the model's projector/vision encoder, which reconcile
				// reports per model. The resolver routes image-bearing requests on
				// this flag, so dropping it here would make remote vision models
				// unreachable for image input.
				CanVision: m.CanVision,
			}
			var prov modelrepo.Provider
			if engine == "openvino" {
				prov = openvino.NewProviderForTarget(m.Model, "", caps, tgt)
			} else {
				prov = llama.NewProviderForTarget(m.Model, "", caps, tgt)
			}
			providersByType["llama"] = append(providersByType["llama"], prov)
			providersByType["openvino"] = append(providersByType["openvino"], prov)
			if state.Backend.Name != "" {
				providersByType[state.Backend.Name] = append(providersByType[state.Backend.Name], prov)
			}
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
