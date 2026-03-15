package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/contenox/internal/modelrepo"
	"github.com/contenox/contenox/internal/modelrepo/gemini"
	"github.com/contenox/contenox/internal/modelrepo/ollama"
	"github.com/contenox/contenox/internal/modelrepo/openai"
	"github.com/contenox/contenox/internal/modelrepo/vllm"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/statetype"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, tracker libtracker.ActivityTracker, runtime map[string]statetype.BackendRuntimeState) ProviderFromRuntimeState {
	// Create a flat list of providers (one per model per backend)
	providersByType := make(map[string][]modelrepo.Provider)

	for _, state := range runtime {
		if state.Error != "" {
			continue
		}

		backendType := state.Backend.Type
		if _, ok := providersByType[backendType]; !ok {
			providersByType[backendType] = []modelrepo.Provider{}
		}

		for _, model := range state.PulledModels {
			capability := modelrepo.CapabilityConfig{
				ContextLength: model.ContextLength,
				CanChat:       model.CanChat,
				CanEmbed:      model.CanEmbed,
				CanStream:     model.CanStream,
				CanPrompt:     model.CanPrompt,
			}

			switch backendType {
			case "ollama":
				providersByType[backendType] = append(
					providersByType[backendType],
					ollama.NewOllamaProvider(
						model.Model,
						[]string{state.Backend.BaseURL},
						http.DefaultClient,
						capability,
						tracker,
					),
				)
			case "vllm":
				providersByType[backendType] = append(
					providersByType[backendType],
					vllm.NewVLLMProvider(
						model.Model,
						[]string{state.Backend.BaseURL},
						http.DefaultClient,
						capability,
						state.GetAPIKey(),
						tracker,
					),
				)
			case "openai":
				providersByType[backendType] = append(
					providersByType[backendType],
					openai.NewOpenAIProvider(
						state.GetAPIKey(),
						model.Model,
						[]string{state.Backend.BaseURL},
						capability,
						http.DefaultClient,
						tracker,
					),
				)
			case "gemini":
				providersByType[backendType] = append(
					providersByType[backendType],
					gemini.NewGeminiProvider(
						state.GetAPIKey(),
						model.Model,
						[]string{state.Backend.BaseURL},
						capability,
						http.DefaultClient,
						tracker,
					),
				)
			}
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
		for _, backendType := range backendTypes {
			if typeProviders, ok := providersByType[backendType]; ok {
				providers = append(providers, typeProviders...)
			}
		}
		return providers, nil
	}
}

// ProviderFromRuntimeState retrieves available model providers
type ProviderFromRuntimeState func(ctx context.Context, backendTypes ...string) ([]modelrepo.Provider, error)
