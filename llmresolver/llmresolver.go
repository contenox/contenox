// Package llmresolver selects the most appropriate backend LLM instance based on requirements.
package llmresolver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/contenox/activitytracker"
	libmodelprovider "github.com/contenox/modelprovider"
)

var (
	ErrNoAvailableModels        = errors.New("no models found in runtime state")
	ErrNoSatisfactoryModel      = errors.New("no model matched the requirements")
	ErrUnknownModelCapabilities = errors.New("capabilities not known for this model")
)

var DefaultProviderType string = "ollama"

// Request contains requirements for selecting a model provider.
type Request struct {
	ProviderTypes []string // Optional: if empty, uses all default providers
	ModelNames    []string // Optional: if empty, any model is considered
	ContextLength int      // Minimum required context length
	Tracker       activitytracker.ActivityTracker
}

func filterCandidates(
	ctx context.Context,
	req Request,
	getModels ProviderFromRuntimeState,
	capCheck func(libmodelprovider.Provider) bool,
) ([]libmodelprovider.Provider, error) {
	providerTypes := req.ProviderTypes
	if len(providerTypes) == 0 {
		providerTypes = []string{"ollama", "vllm"}
	}
	providers, err := getModels(ctx, providerTypes...)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}
	if len(providers) == 0 {
		return nil, ErrNoAvailableModels
	}

	// Use a map to track seen providers by ID to prevent duplicates
	seenProviders := make(map[string]bool)
	var candidates []libmodelprovider.Provider

	// Handle model name preferences
	if len(req.ModelNames) > 0 {
		// Check preferred models in order of priority
		for _, preferredModel := range req.ModelNames {
			// Use normalized model names for matching
			normalizedPreferred := NormalizeModelName(preferredModel)

			for _, p := range providers {
				if seenProviders[p.GetID()] {
					continue
				}

				// Normalize provider's model name for comparison
				currentNormalized := NormalizeModelName(p.ModelName())
				currentFull := p.ModelName()

				// Match either normalized or full name
				if currentNormalized != normalizedPreferred && currentFull != preferredModel {
					continue
				}

				if validateProvider(p, req.ContextLength, capCheck) {
					candidates = append(candidates, p)
					seenProviders[p.GetID()] = true
				}
			}
		}
	} else {
		// Consider all providers when no model names specified
		for _, p := range providers {
			if validateProvider(p, req.ContextLength, capCheck) {
				candidates = append(candidates, p)
			}
		}
	}

	if len(candidates) == 0 {
		var builder strings.Builder

		builder.WriteString("no models matched requirements:\n")
		builder.WriteString(fmt.Sprintf("- provider: %q\n", providerTypes))
		builder.WriteString(fmt.Sprintf("- model names: %v\n", req.ModelNames))
		builder.WriteString(fmt.Sprintf("- required context length: %d\n", req.ContextLength))

		builder.WriteString("- available models:\n")
		for _, p := range providers {
			builder.WriteString(fmt.Sprintf("  • %s (ID: %s, context: %d, canchat: %v, can embed: %v, canprompt: %v)\n",
				p.ModelName(), p.GetID(), p.GetContextLength(), p.CanChat(), p.CanEmbed(), p.CanPrompt()))
		}

		return nil, fmt.Errorf("%w\n%s", ErrNoSatisfactoryModel, builder.String())
	}

	return candidates, nil
}

type Policy func(candidates []libmodelprovider.Provider) (libmodelprovider.Provider, string, error)

const (
	StrategyRandom      = "random"
	StrategyAuto        = "auto"
	StrategyLowLatency  = "low-latency"
	StrategyLowPriority = "low-prio"
)

// PolicyFromString maps string names to resolver policies
func PolicyFromString(name string) (Policy, error) {
	switch strings.ToLower(name) {
	case StrategyRandom:
		return Randomly, nil
	case StrategyLowLatency, StrategyAuto:
		return HighestContext, nil
	// case StrategyLowPriority:
	// 	return ResolveLowestPriority, nil
	default:
		return nil, fmt.Errorf("unknown resolver strategy: %s", name)
	}
}

func Randomly(candidates []libmodelprovider.Provider) (libmodelprovider.Provider, string, error) {
	provider, err := selectRandomProvider(candidates)
	if err != nil {
		return nil, "", err
	}

	backend, err := selectRandomBackend(provider)
	if err != nil {
		return nil, "", err
	}

	return provider, backend, nil
}

func selectRandomProvider(candidates []libmodelprovider.Provider) (libmodelprovider.Provider, error) {
	if len(candidates) == 0 {
		return nil, ErrNoSatisfactoryModel
	}

	return candidates[rand.Intn(len(candidates))], nil
}

func selectRandomBackend(provider libmodelprovider.Provider) (string, error) {
	if provider == nil {
		return "", ErrNoSatisfactoryModel
	}

	backendIDs := provider.GetBackendIDs()
	if len(backendIDs) == 0 {
		return "", ErrNoSatisfactoryModel
	}

	return backendIDs[rand.Intn(len(backendIDs))], nil
}

func HighestContext(candidates []libmodelprovider.Provider) (libmodelprovider.Provider, string, error) {
	if len(candidates) == 0 {
		return nil, "", ErrNoSatisfactoryModel
	}

	var bestProvider libmodelprovider.Provider = nil
	maxContextLength := -1

	for _, p := range candidates {
		currentContextLength := p.GetContextLength()
		if currentContextLength > maxContextLength {
			maxContextLength = currentContextLength
			bestProvider = p
		}
	}

	if bestProvider == nil {
		return nil, "", errors.New("failed to select a provider based on context length") // Should never happen
	}

	// Once the best provider is selected, choose a backend randomly for it
	backend, err := selectRandomBackend(bestProvider)
	if err != nil {
		return nil, "", err
	}

	return bestProvider, backend, nil
}

// validateProvider checks if a provider meets requirements
func validateProvider(p libmodelprovider.Provider, minContext int, capCheck func(libmodelprovider.Provider) bool) bool {
	if minContext > 0 && p.GetContextLength() < minContext {
		return false
	}
	return capCheck(p)
}

// NormalizeModelName standardizes model names for comparison
func NormalizeModelName(modelName string) string {
	// Convert to lowercase for case-insensitive comparison
	normalized := strings.ToLower(modelName)

	// Remove common prefixes and suffixes
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, ".", "")

	// Remove organization prefix if present
	if parts := strings.Split(normalized, "/"); len(parts) > 1 {
		normalized = parts[1]
	}

	// Remove quantization suffixes
	normalized = strings.ReplaceAll(normalized, "awq", "")
	normalized = strings.ReplaceAll(normalized, "gptq", "")
	normalized = strings.ReplaceAll(normalized, "4bit", "")
	normalized = strings.ReplaceAll(normalized, "fp16", "")

	// Remove version numbers
	if idx := strings.LastIndex(normalized, ":"); idx != -1 {
		normalized = normalized[:idx]
	}

	return normalized
}

func Chat(
	ctx context.Context,
	req Request,
	getModels ProviderFromRuntimeState,
	resolver Policy,
) (libmodelprovider.LLMChatClient, string, error) {
	tracker := req.Tracker
	if tracker == nil {
		tracker = activitytracker.NoopTracker{}
	}
	reportErr, reportChange, endFn := tracker.Start(
		ctx,
		"resolve",
		"chat_model",
		"provider_types", req.ProviderTypes,
		"model_names", req.ModelNames,
		"context_length", req.ContextLength,
	)
	defer endFn()

	candidates, err := filterCandidates(ctx, req, getModels, libmodelprovider.Provider.CanChat)
	if err != nil {
		reportErr(err)
		return nil, "", err
	}
	provider, backend, err := resolver(candidates)
	if err != nil {
		reportErr(err)
		return nil, "", err
	}
	if req.ContextLength == 0 {
		err = fmt.Errorf("context length must be greater than 0")
		reportErr(err)
		return nil, "", err
	}
	if req.ContextLength < 0 {
		err = fmt.Errorf("context length must be non-negative")
		reportErr(err)
		return nil, "", err
	}
	modelName := provider.ModelName()
	client, err := provider.GetChatConnection(ctx, backend)
	if err != nil {
		reportErr(err)
		return nil, "", err
	}
	reportChange("selected_provider", map[string]string{
		"model_name":  modelName,
		"provider_id": provider.GetID(),
		"backend_id":  backend,
	})
	return client, modelName, nil
}

type EmbedRequest struct {
	ModelName    string
	ProviderType string // Optional. Empty uses default.
	tracker      activitytracker.ActivityTracker
}

func Embed(
	ctx context.Context,
	embedReq EmbedRequest,
	getModels ProviderFromRuntimeState,
	resolver Policy,
) (libmodelprovider.LLMEmbedClient, error) {
	tracker := embedReq.tracker
	if tracker == nil {
		tracker = activitytracker.NoopTracker{}
	}
	reportErr, reportChange, endFn := tracker.Start(
		ctx,
		"resolve",
		"embed_model",
		"model_name", embedReq.ModelName,
		"provider_type", embedReq.ProviderType,
	)
	defer endFn()

	if embedReq.ModelName == "" {
		err := fmt.Errorf("model name is required")
		reportErr(err)
		return nil, err
	}
	if embedReq.ProviderType == "" {
		embedReq.ProviderType = DefaultProviderType
	}
	req := Request{
		ModelNames:    []string{embedReq.ModelName},
		ProviderTypes: []string{embedReq.ProviderType},
	}
	candidates, err := filterCandidates(ctx, req, getModels, libmodelprovider.Provider.CanEmbed)
	if err != nil {
		reportErr(err)
		return nil, fmt.Errorf("failed to filter candidates %w", err)
	}
	provider, backend, err := resolver(candidates)
	if err != nil {
		reportErr(err)
		return nil, fmt.Errorf("failed apply resolver %w", err)
	}
	reportChange("selected_provider", map[string]string{
		"model_name":  provider.ModelName(),
		"provider_id": provider.GetID(),
		"backend_id":  backend,
	})
	return provider.GetEmbedConnection(ctx, backend)
}

func Stream(
	ctx context.Context,
	req Request,
	getModels ProviderFromRuntimeState,
	resolver Policy,
) (libmodelprovider.LLMStreamClient, error) {
	tracker := req.Tracker
	if tracker == nil {
		tracker = activitytracker.NoopTracker{}
	}
	reportErr, reportChange, endFn := tracker.Start(
		ctx,
		"resolve",
		"stream_model",
		"provider_types", req.ProviderTypes,
		"model_names", req.ModelNames,
		"context_length", req.ContextLength,
	)
	defer endFn()

	candidates, err := filterCandidates(ctx, req, getModels, libmodelprovider.Provider.CanStream)
	if err != nil {
		reportErr(err)
		return nil, err
	}
	provider, backend, err := resolver(candidates)
	if err != nil {
		reportErr(err)
		return nil, err
	}
	reportChange("selected_provider", map[string]string{
		"model_name":  provider.ModelName(),
		"provider_id": provider.GetID(),
		"backend_id":  backend,
	})
	return provider.GetStreamConnection(ctx, backend)
}

type PromptRequest struct {
	ModelNames    []string
	ProviderTypes []string // Optional. Empty uses default.
	Tracker       activitytracker.ActivityTracker
	ContextLength int
}

func PromptExecute(
	ctx context.Context,
	reqExec PromptRequest,
	getModels ProviderFromRuntimeState,
	resolver Policy,
) (libmodelprovider.LLMPromptExecClient, error) {
	tracker := reqExec.Tracker
	if tracker == nil {
		tracker = activitytracker.NoopTracker{}
	}
	reportErr, reportChange, endFn := tracker.Start(
		ctx,
		"resolve",
		"prompt_model",
		"model_names", reqExec.ModelNames,
		"provider_types", reqExec.ProviderTypes,
		"context_length", reqExec.ContextLength,
	)
	defer endFn()

	if len(reqExec.ModelNames) == 0 {
		err := fmt.Errorf("at least one model name is required")
		reportErr(err)
		return nil, err
	}
	if len(reqExec.ProviderTypes) == 0 {
		reqExec.ProviderTypes = []string{DefaultProviderType, "vllm"}
	}
	req := Request{
		ModelNames:    reqExec.ModelNames,
		ProviderTypes: reqExec.ProviderTypes,
		ContextLength: reqExec.ContextLength,
	}
	candidates, err := filterCandidates(ctx, req, getModels, libmodelprovider.Provider.CanPrompt)
	if err != nil {
		reportErr(err)
		return nil, err
	}
	provider, backend, err := resolver(candidates)
	if err != nil {
		reportErr(err)
		return nil, err
	}
	reportChange("selected_provider", map[string]string{
		"model_name":    provider.ModelName(),
		"provider_id":   provider.GetID(),
		"provider_type": provider.GetType(),
		"backend_id":    backend,
	})
	return provider.GetPromptConnection(ctx, backend)
}

// ProviderFromRuntimeState retrieves available model providers
type ProviderFromRuntimeState func(ctx context.Context, backendTypes ...string) ([]libmodelprovider.Provider, error)
