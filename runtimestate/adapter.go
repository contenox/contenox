package runtimestate

import (
	"context"
	"net/http"

	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/modelprovider/llmresolver"
)

// LocalProviderAdapter creates providers for self-hosted backends (Ollama, vLLM)
func LocalProviderAdapter(ctx context.Context, runtime map[string]LLMState) llmresolver.ProviderFromRuntimeState {
	// Create a two-level map: backendType -> modelName -> []baseURLs
	modelsByBackendType := make(map[string]map[string][]LLMState)

	for _, state := range runtime {
		backendType := state.Backend.Type

		if _, ok := modelsByBackendType[backendType]; !ok {
			modelsByBackendType[backendType] = make(map[string][]LLMState)
		}

		for _, model := range state.PulledModels {
			modelName := model.Model
			modelsByBackendType[backendType][modelName] = append(
				modelsByBackendType[backendType][modelName],
				state,
			)
		}
	}

	// Create providers grouped by backend type
	providersByType := make(map[string][]libmodelprovider.Provider)
	for backendType, modelMap := range modelsByBackendType {
		for modelName, snapshots := range modelMap {
			apiKey := ""
			backendURLs := []string{}
			for _, state := range snapshots {
				apiKey = state.apiKey
				backendURLs = append(backendURLs, state.Backend.BaseURL)
			}
			switch backendType {
			case "ollama":
				providersByType["ollama"] = append(providersByType["ollama"],
					libmodelprovider.NewOllamaModelProvider(modelName, backendURLs, http.DefaultClient))
			case "vllm":
				providersByType["vllm"] = append(providersByType["vllm"],
					libmodelprovider.NewVLLMModelProvider(modelName, backendURLs, http.DefaultClient))
			case "openai":
				providersByType["openai"] = append(providersByType["openai"],
					libmodelprovider.NewOpenAIProvider(apiKey, modelName, backendURLs, http.DefaultClient))
			case "gemini":
				{
					providersByType["gemini"] = append(providersByType["gemini"],
						libmodelprovider.NewGeminiProvider(apiKey, modelName, backendURLs, http.DefaultClient))
				}
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
