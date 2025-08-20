package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/modelprovider"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, runtime map[string]LLMState) ProviderFromRuntimeState {
	// Create a flat list of providers (one per model per backend)
	providersByType := make(map[string][]modelprovider.Provider)

	for _, state := range runtime {
		if state.Error != "" {
			continue
		}

		backendType := state.Backend.Type
		if _, ok := providersByType[backendType]; !ok {
			providersByType[backendType] = []modelprovider.Provider{}
		}

		for _, model := range state.PulledModels {
			capability := modelprovider.CapabilityConfig{
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
					modelprovider.NewOllamaModelProvider(
						model.Model,
						[]string{state.Backend.BaseURL},
						http.DefaultClient,
						capability,
					),
				)
			case "vllm":
				providersByType[backendType] = append(
					providersByType[backendType],
					modelprovider.NewVLLMModelProvider(
						model.Model,
						[]string{state.Backend.BaseURL},
						http.DefaultClient,
						capability,
						state.apiKey,
					),
				)
			case "openai":
				providersByType[backendType] = append(
					providersByType[backendType],
					modelprovider.NewOpenAIProvider(
						state.apiKey,
						model.Model,
						[]string{state.Backend.BaseURL},
						capability,
						http.DefaultClient,
					),
				)
			case "gemini":
				providersByType[backendType] = append(
					providersByType[backendType],
					modelprovider.NewGeminiProvider(
						state.apiKey,
						model.Model,
						[]string{state.Backend.BaseURL},
						capability,
						http.DefaultClient,
					),
				)
			}
		}
	}

	return func(ctx context.Context, backendTypes ...string) ([]modelprovider.Provider, error) {
		var providers []modelprovider.Provider
		for _, backendType := range backendTypes {
			if typeProviders, ok := providersByType[backendType]; ok {
				providers = append(providers, typeProviders...)
			}
		}
		return providers, nil
	}
}

// ProviderFromRuntimeState retrieves available model providers
type ProviderFromRuntimeState func(ctx context.Context, backendTypes ...string) ([]modelprovider.Provider, error)
