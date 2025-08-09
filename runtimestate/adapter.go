package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/modelprovider"
	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/runtime/llmresolver"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, runtime map[string]LLMState) llmresolver.ProviderFromRuntimeState {
	// Create a flat list of providers (one per model per backend)
	providersByType := make(map[string][]libmodelprovider.Provider)

	for _, state := range runtime {
		if state.Error != "" {
			continue
		}

		backendType := state.Backend.Type
		if _, ok := providersByType[backendType]; !ok {
			providersByType[backendType] = []libmodelprovider.Provider{}
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
					libmodelprovider.NewOllamaModelProvider(
						model.Model,
						[]string{state.Backend.BaseURL},
						http.DefaultClient,
						capability,
					),
				)
			case "vllm":
				providersByType[backendType] = append(
					providersByType[backendType],
					libmodelprovider.NewVLLMModelProvider(
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
					libmodelprovider.NewOpenAIProvider(
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
					libmodelprovider.NewGeminiProvider(
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

	return func(ctx context.Context, backendTypes ...string) ([]libmodelprovider.Provider, error) {
		var providers []libmodelprovider.Provider
		for _, backendType := range backendTypes {
			if typeProviders, ok := providersByType[backendType]; ok {
				providers = append(providers, typeProviders...)
			}
		}
		return providers, nil
	}
}
