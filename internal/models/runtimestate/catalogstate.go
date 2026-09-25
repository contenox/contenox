package runtimestate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/contenox/contenox/internal/models/modelcapability"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libkvstore"
)

const observedModelCachePrefix = "prov:"

func observedModelCacheKey(backendID string) string {
	return observedModelCachePrefix + backendID
}

// InvalidateModelCache removes the cached observed-model list for one backend,
// forcing the next backend cycle to refetch from the provider. Safe when no
// entry exists; no-op when kv is nil.
func InvalidateModelCache(ctx context.Context, kv libkvstore.KVManager, backendID string) error {
	if kv == nil {
		return nil
	}
	exec, err := kv.Executor(ctx)
	if err != nil {
		return err
	}
	return exec.Delete(ctx, observedModelCacheKey(backendID))
}

// ClearModelCache removes every cached observed-model list (all "prov:*" keys)
// and returns how many were cleared. No-op (0) when kv is nil.
func ClearModelCache(ctx context.Context, kv libkvstore.KVManager) (int, error) {
	if kv == nil {
		return 0, nil
	}
	exec, err := kv.Executor(ctx)
	if err != nil {
		return 0, err
	}
	keys, err := exec.Keys(ctx, observedModelCachePrefix+"*")
	if err != nil {
		return 0, err
	}
	cleared := 0
	for _, k := range keys {
		if err := exec.Delete(ctx, k); err != nil {
			return cleared, err
		}
		cleared++
	}
	return cleared, nil
}

func providerConfigKey(backendType string) (string, bool) {
	switch modelrepo.CanonicalBackendType(backendType) {
	case "ollama":
		return OllamaKey, true
	case "openai":
		return OpenaiKey, true
	case "anthropic":
		return AnthropicKey, true
	case "bedrock":
		return BedrockKey, true
	case "gemini":
		return GeminiKey, true
	case "vllm":
		// vLLM reuses the OpenAI-compatible bearer token configuration.
		return OpenaiKey, true
	case "vertex-google":
		return VertexGoogleKey, true
	default:
		return "", false
	}
}

func BackendCredentialKey(backendType, backendID string) string {
	return ProviderKeyPrefix + strings.ToLower(backendType) + ":" + backendID
}

func (s *State) loadProviderAPIKey(ctx context.Context, backend *runtimetypes.Backend) (string, error) {
	key, ok := providerConfigKey(backend.Type)
	if !ok {
		return "", nil
	}

	store := runtimetypes.New(s.dbInstance.WithoutTransaction())
	if backend.ID != "" {
		var own ProviderConfig
		if err := store.GetKV(ctx, BackendCredentialKey(backend.Type, backend.ID), &own); err == nil {
			if cred := credentialFrom(own); cred != "" {
				return cred, nil
			}
		}
	}

	cfg := ProviderConfig{}
	if err := store.GetKV(ctx, key, &cfg); err != nil {
		return "", err
	}
	return credentialFrom(cfg), nil
}

func credentialFrom(cfg ProviderConfig) string {
	if cfg.APIKey == "" && strings.TrimSpace(cfg.APIKeyEnv) != "" {
		return os.Getenv(strings.TrimSpace(cfg.APIKeyEnv))
	}
	return cfg.APIKey
}

func (s *State) newCatalogProvider(backend *runtimetypes.Backend, apiKey string) (modelrepo.CatalogProvider, error) {
	return modelrepo.NewCatalogProvider(
		modelrepo.BackendSpec{
			Type:    backend.Type,
			BaseURL: backend.BaseURL,
			APIKey:  apiKey,
		},
		modelrepo.WithCatalogHTTPClient(modelrepo.SharedHTTPClient),
	)
}

func (s *State) loadObservedModelCache(ctx context.Context, backendID, apiKey string) ([]modelrepo.ObservedModel, bool) {
	if s.kvStore != nil {
		if exec, err := s.kvStore.Executor(ctx); err == nil {
			if raw, err := exec.Get(ctx, observedModelCacheKey(backendID)); err == nil {
				var entry providerCacheEntry
				if json.Unmarshal(raw, &entry) == nil && entry.APIKey == apiKey && len(entry.Models) > 0 {
					return entry.Models, true
				}
			}
		}
		return nil, false
	}

	if cached, ok := s.providerCache.Load(backendID); ok {
		if entry, ok := cached.(providerCacheEntry); ok && entry.APIKey == apiKey && len(entry.Models) > 0 {
			return entry.Models, true
		}
	}
	return nil, false
}

func (s *State) storeObservedModelCache(ctx context.Context, backendID, apiKey string, models []modelrepo.ObservedModel, catalog modelrepo.CatalogProvider) {
	policy := modelrepo.ModelCachePolicy{}
	if p, ok := catalog.(modelrepo.CachePolicyCatalog); ok {
		policy = p.CachePolicy()
	}
	ttl := ProviderCacheDuration
	if len(models) == 0 {
		if policy.Empty < 0 {
			return
		}
		if policy.Empty > 0 {
			ttl = policy.Empty
		}
	} else if policy.Healthy > 0 {
		ttl = policy.Healthy
	}
	entry := providerCacheEntry{Models: models, APIKey: apiKey}
	if s.kvStore != nil {
		if exec, err := s.kvStore.Executor(ctx); err == nil {
			if data, err := json.Marshal(entry); err == nil {
				_ = exec.SetWithTTL(ctx, observedModelCacheKey(backendID), data, ttl)
			}
		}
		return
	}
	s.providerCache.Store(backendID, entry)
}

func observedModelNames(models []modelrepo.ObservedModel) []string {
	names := make([]string, 0, len(models))
	for _, model := range models {
		names = append(names, model.Name)
	}
	return names
}

func (s *State) applyDeclaredModel(ctx context.Context, backend *runtimetypes.Backend, declared *runtimetypes.Model, observed modelrepo.ObservedModel) ModelPullStatus {
	model := pullStatusFromObservedModel(observed)
	if declared != nil {
		model = s.mergeDeclaredRow(ctx, declared, model)
	}
	return s.applyDeclarations(ctx, backend, model)
}

func (s *State) mergeDeclaredRow(ctx context.Context, declared *runtimetypes.Model, model ModelPullStatus) ModelPullStatus {
	if declared.ContextLength == 0 && model.ContextLength > 0 {
		learned := *declared
		learned.ContextLength = model.ContextLength
		learned.CanChat = model.CanChat
		learned.CanEmbed = model.CanEmbed
		learned.CanPrompt = model.CanPrompt
		learned.CanStream = model.CanStream
		_ = runtimetypes.New(s.dbInstance.WithoutTransaction()).UpdateModel(ctx, &learned)
	}
	if declared.ContextLength > 0 {
		model.ContextLength = declared.ContextLength
	}
	model.CanChat = model.CanChat || declared.CanChat
	model.CanEmbed = model.CanEmbed || declared.CanEmbed
	model.CanPrompt = model.CanPrompt || declared.CanPrompt
	model.CanStream = model.CanStream || declared.CanStream
	return model
}

func (s *State) applyDeclarations(ctx context.Context, backend *runtimetypes.Backend, model ModelPullStatus) ModelPullStatus {
	model = s.applyCapabilityOverrides(ctx, backend.Type, model)
	return s.applyModelFacts(ctx, backend.ID, model)
}

func (s *State) applyCapabilityOverrides(ctx context.Context, provider string, model ModelPullStatus) ModelPullStatus {
	provider = modelrepo.CanonicalBackendType(provider)
	name := declaredModelName(model)
	if name == "" {
		return model
	}
	override, ok, err := modelcapability.New(runtimetypes.New(s.dbInstance.WithoutTransaction())).Get(ctx, provider, name)
	if err != nil || !ok {
		return model
	}
	if override.CanThink != nil {
		model.CanThink = *override.CanThink
	}
	if override.CanVision != nil {
		model.CanVision = *override.CanVision
	}
	return model
}

func (s *State) applyModelFacts(ctx context.Context, providerID string, model ModelPullStatus) ModelPullStatus {
	name := declaredModelName(model)
	if providerID == "" || name == "" {
		return model
	}
	declared, err := runtimetypes.New(s.dbInstance.WithoutTransaction()).GetLLMProviderModelFacts(ctx, providerID)
	if err != nil {
		return model
	}
	facts, ok := declared[name]
	if !ok || facts.IsZero() {
		return model
	}
	if facts.ContextLength > 0 {
		model.ContextLength = facts.ContextLength
	}
	if facts.MaxOutputTokens > 0 {
		model.MaxOutputTokens = facts.MaxOutputTokens
	}
	if len(facts.Capabilities) > 0 {
		model.DeclaredCapabilities = facts.Capabilities
		applyDeclaredCapabilities(&model, facts.Capabilities)
	}
	if facts.Pricing != nil {
		model.Pricing = facts.Pricing
	}
	return model
}

func declaredModelName(model ModelPullStatus) string {
	if name := strings.TrimSpace(model.Model); name != "" {
		return name
	}
	return strings.TrimSpace(model.Name)
}

func applyDeclaredCapabilities(model *ModelPullStatus, capabilities []string) {
	model.CanChat = false
	model.CanEmbed = false
	model.CanPrompt = false
	model.CanStream = false
	model.CanThink = false
	model.CanVision = false
	model.CanAudio = false
	for _, capability := range capabilities {
		switch capability {
		case runtimetypes.CapabilityCompletion:
			model.CanChat = true
			model.CanPrompt = true
			model.CanStream = true
		case runtimetypes.CapabilityEmbedding:
			model.CanEmbed = true
		case runtimetypes.CapabilityTools:
			model.CanChat = true
		case runtimetypes.CapabilityVision:
			model.CanVision = true
		case runtimetypes.CapabilityThinking:
			model.CanThink = true
		case runtimetypes.CapabilityAudio:
			model.CanAudio = true
		}
	}
}

func storeBackendError(state *State, backend *runtimetypes.Backend, apiKey string, err error, models []string) {
	runtimeState := &BackendRuntimeState{
		ID:           backend.ID,
		Name:         backend.Name,
		Models:       models,
		PulledModels: []ModelPullStatus{},
		Backend:      *backend,
	}
	if err != nil {
		runtimeState.Error = err.Error()
	}
	runtimeState.SetAPIKey(apiKey)
	state.state.Store(backend.ID, runtimeState)
}

func declaredModelDebugMap(declaredModels map[string]*runtimetypes.Model) []string {
	declaredMap := make([]string, 0, len(declaredModels))
	for key, model := range declaredModels {
		payload := "model-data==nil"
		if model != nil {
			payload = model.ID + " " + model.Model
		}
		declaredMap = append(declaredMap, key+":"+payload)
	}
	return declaredMap
}

func declaredModelsUnavailableError(provider string, declaredModels map[string]*runtimetypes.Model, available []string) error {
	return fmt.Errorf(
		"None of the declared models are available in the %s API: declared models: %v \navailable models %s",
		provider,
		strings.Join(declaredModelDebugMap(declaredModels), ", "),
		available,
	)
}
