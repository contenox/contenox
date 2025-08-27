package runtimestate

import (
	"context"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/contenox/runtime/internal/modelrepo/gemini"
	"github.com/contenox/runtime/internal/modelrepo/ollama"
	"github.com/contenox/runtime/internal/modelrepo/openai"
	"github.com/contenox/runtime/internal/modelrepo/vllm"
	"github.com/contenox/runtime/statetype"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, runtime map[string]statetype.BackendRuntimeState) ProviderFromRuntimeState {
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
					),
				)
			}
		}
	}

	return func(ctx context.Context, backendTypes ...string) ([]modelrepo.Provider, error) {
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
