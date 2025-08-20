package llmresolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/contenox/activitytracker"
	"github.com/contenox/bus"
	"github.com/contenox/modelprovider"
)

// SerializableRequest is a transport-safe version of Request without unserializable fields
type SerializableRequest struct {
	ProviderTypes []string `json:"provider_types"`
	ModelNames    []string `json:"model_names"`
	ContextLength int      `json:"context_length"`
}

// SerializableEmbedRequest is a transport-safe version of EmbedRequest
type SerializableEmbedRequest struct {
	ModelName    string `json:"model_name"`
	ProviderType string `json:"provider_type"`
}

// ResolutionResponse contains the information needed to connect directly to a resolved model
type ResolutionResponse struct {
	ProviderType  string            `json:"provider_type"` // "gemini", "openai", "ollama", "vllm"
	ModelName     string            `json:"model_name"`
	BackendURL    string            `json:"backend_url"` // Base URL for the backend
	BackendID     string            `json:"backend_id"`  // Identifier for the specific backend instance
	APIKey        string            `json:"api_key,omitempty"`
	ExtraConfig   map[string]string `json:"extra_config,omitempty"`
	ContextLength int               `json:"context_length"`
	Capabilities  struct {
		CanChat   bool `json:"can_chat"`
		CanPrompt bool `json:"can_prompt"`
		CanEmbed  bool `json:"can_embed"`
		CanStream bool `json:"can_stream"`
	} `json:"capabilities"`
}

// ResolutionRequest is the format sent to the resolver service
type ResolutionRequest struct {
	Operation string          `json:"operation"`
	Request   json.RawMessage `json:"request"`
}

// ResolutionResponseEnvelope standardizes the response format
type ResolutionResponseEnvelope struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// MeshResolver implements Resolver using the messaging bus for model resolution only.
//
// This resolver delegates the model selection process to a dedicated service,
// while maintaining a direct connection to the selected model backend.
type MeshResolver struct {
	bus     bus.Messenger
	tracker activitytracker.ActivityTracker
	client  *http.Client // For direct connections after resolution
}

// NewMeshResolver creates a new BusResolver instance.
//
// Parameters:
//   - bus: The messaging bus implementation to use for resolution requests
//   - tracker: Optional activity tracker for monitoring resolution operations
//
// If no tracker is provided, a NoopTracker will be used.
func NewMeshResolver(bus bus.Messenger, tracker activitytracker.ActivityTracker) Resolver {
	if tracker == nil {
		tracker = activitytracker.NoopTracker{}
	}

	return &MeshResolver{
		bus:     bus,
		tracker: tracker,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxConnsPerHost:     10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

// convertToSerializableRequest converts Request to a serializable form
func convertToSerializableRequest(req Request) SerializableRequest {
	return SerializableRequest{
		ProviderTypes: req.ProviderTypes,
		ModelNames:    req.ModelNames,
		ContextLength: req.ContextLength,
	}
}

// convertToSerializableEmbedRequest converts EmbedRequest to a serializable form
func convertToSerializableEmbedRequest(req EmbedRequest) SerializableEmbedRequest {
	return SerializableEmbedRequest{
		ModelName:    req.ModelName,
		ProviderType: req.ProviderType,
	}
}

func (b *MeshResolver) ResolvePromptExecute(ctx context.Context, req Request) (modelprovider.LLMPromptExecClient, modelprovider.Provider, string, error) {
	reportErr, reportChange, endFn := b.tracker.Start(
		ctx,
		"resolve",
		"prompt_model",
		"model_names", req.ModelNames,
		"provider_types", req.ProviderTypes,
		"context_length", req.ContextLength,
	)
	defer endFn()

	// Convert to serializable form
	serializableReq := convertToSerializableRequest(req)

	// Create request payload
	request := struct {
		Operation string              `json:"operation"`
		Request   SerializableRequest `json:"request"`
	}{
		Operation: "prompt",
		Request:   serializableReq,
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		reportErr(fmt.Errorf("failed to serialize request: %w", err))
		return nil, nil, "", fmt.Errorf("failed to serialize request: %w", err)
	}

	// Send request to bus
	respData, err := b.bus.Request(ctx, "llmresolver.resolve", reqData)
	if err != nil {
		reportErr(fmt.Errorf("bus request failed: %w", err))
		return nil, nil, "", fmt.Errorf("bus request failed: %w", err)
	}

	// Parse response envelope
	var envelope ResolutionResponseEnvelope
	if err := json.Unmarshal(respData, &envelope); err != nil {
		reportErr(fmt.Errorf("failed to parse response envelope: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response envelope: %w", err)
	}

	// Check for application error
	if envelope.Error != "" {
		reportErr(fmt.Errorf("remote error: %s", envelope.Error))
		return nil, nil, "", fmt.Errorf("remote error: %s", envelope.Error)
	}

	// Parse the actual response data
	var response ResolutionResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		reportErr(fmt.Errorf("failed to parse response data: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response data: %w", err)
	}

	// Create the appropriate provider and client based on provider type
	provider, err := createProvider(response, b.client)
	if err != nil {
		reportErr(fmt.Errorf("failed to create provider: %w", err))
		return nil, nil, "", fmt.Errorf("failed to create provider: %w", err)
	}

	// Get the prompt connection
	client, err := provider.GetPromptConnection(ctx, response.BackendURL)
	if err != nil {
		reportErr(fmt.Errorf("failed to get prompt connection: %w", err))
		return nil, nil, "", fmt.Errorf("failed to get prompt connection: %w", err)
	}

	// Report selected provider
	reportChange("selected_provider", map[string]string{
		"model_name":    response.ModelName,
		"provider_id":   provider.GetID(),
		"provider_type": response.ProviderType,
		"backend_id":    response.BackendID,
	})

	return client, provider, response.BackendID, nil
}

// ResolveChat implements Resolver by using the bus for resolution only.
func (b *MeshResolver) ResolveChat(ctx context.Context, req Request) (modelprovider.LLMChatClient, modelprovider.Provider, string, error) {
	reportErr, reportChange, endFn := b.tracker.Start(
		ctx,
		"resolve",
		"chat_model",
		"provider_types", req.ProviderTypes,
		"model_names", req.ModelNames,
		"context_length", req.ContextLength,
	)
	defer endFn()

	// Convert to serializable form
	serializableReq := convertToSerializableRequest(req)

	// Create request payload
	request := struct {
		Operation string              `json:"operation"`
		Request   SerializableRequest `json:"request"`
	}{
		Operation: "chat",
		Request:   serializableReq,
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		reportErr(fmt.Errorf("failed to serialize request: %w", err))
		return nil, nil, "", fmt.Errorf("failed to serialize request: %w", err)
	}

	// Send request to bus
	respData, err := b.bus.Request(ctx, "llmresolver.resolve", reqData)
	if err != nil {
		reportErr(fmt.Errorf("bus request failed: %w", err))
		return nil, nil, "", fmt.Errorf("bus request failed: %w", err)
	}

	// Parse response envelope
	var envelope ResolutionResponseEnvelope
	if err := json.Unmarshal(respData, &envelope); err != nil {
		reportErr(fmt.Errorf("failed to parse response envelope: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response envelope: %w", err)
	}

	// Check for application error
	if envelope.Error != "" {
		reportErr(fmt.Errorf("remote error: %s", envelope.Error))
		return nil, nil, "", fmt.Errorf("remote error: %s", envelope.Error)
	}

	// Parse the actual response data
	var response ResolutionResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		reportErr(fmt.Errorf("failed to parse response data: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response data: %w", err)
	}

	// Create the appropriate provider and client based on provider type
	provider, err := createProvider(response, b.client)
	if err != nil {
		reportErr(fmt.Errorf("failed to create provider: %w", err))
		return nil, nil, "", fmt.Errorf("failed to create provider: %w", err)
	}

	// Get the chat connection
	client, err := provider.GetChatConnection(ctx, response.BackendURL)
	if err != nil {
		reportErr(fmt.Errorf("failed to get chat connection: %w", err))
		return nil, nil, "", fmt.Errorf("failed to get chat connection: %w", err)
	}

	// Report selected provider
	reportChange("selected_provider", map[string]string{
		"model_name":    response.ModelName,
		"provider_id":   provider.GetID(),
		"provider_type": response.ProviderType,
		"backend_id":    response.BackendID,
	})

	return client, provider, response.BackendID, nil
}

// ResolveEmbed implements Resolver by using the bus for resolution only.
func (b *MeshResolver) ResolveEmbed(ctx context.Context, req EmbedRequest) (modelprovider.LLMEmbedClient, modelprovider.Provider, string, error) {
	reportErr, reportChange, endFn := b.tracker.Start(
		ctx,
		"resolve",
		"embed_model",
		"model_name", req.ModelName,
		"provider_type", req.ProviderType,
	)
	defer endFn()

	// Convert to serializable form
	serializableReq := convertToSerializableEmbedRequest(req)

	// Create request payload
	request := struct {
		Operation string                   `json:"operation"`
		Request   SerializableEmbedRequest `json:"request"`
	}{
		Operation: "embed",
		Request:   serializableReq,
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		reportErr(fmt.Errorf("failed to serialize request: %w", err))
		return nil, nil, "", fmt.Errorf("failed to serialize request: %w", err)
	}

	// Send request to bus
	respData, err := b.bus.Request(ctx, "llmresolver.resolve", reqData)
	if err != nil {
		reportErr(fmt.Errorf("bus request failed: %w", err))
		return nil, nil, "", fmt.Errorf("bus request failed: %w", err)
	}

	// Parse response envelope
	var envelope ResolutionResponseEnvelope
	if err := json.Unmarshal(respData, &envelope); err != nil {
		reportErr(fmt.Errorf("failed to parse response envelope: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response envelope: %w", err)
	}

	// Check for application error
	if envelope.Error != "" {
		reportErr(fmt.Errorf("remote error: %s", envelope.Error))
		return nil, nil, "", fmt.Errorf("remote error: %s", envelope.Error)
	}

	// Parse the actual response data
	var response ResolutionResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		reportErr(fmt.Errorf("failed to parse response data: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response data: %w", err)
	}

	// Create the appropriate provider and client based on provider type
	provider, err := createProvider(response, b.client)
	if err != nil {
		reportErr(fmt.Errorf("failed to create provider: %w", err))
		return nil, nil, "", fmt.Errorf("failed to create provider: %w", err)
	}

	// Get the embed connection
	client, err := provider.GetEmbedConnection(ctx, response.BackendURL)
	if err != nil {
		reportErr(fmt.Errorf("failed to get embed connection: %w", err))
		return nil, nil, "", fmt.Errorf("failed to get embed connection: %w", err)
	}

	// Report selected provider
	reportChange("selected_provider", map[string]string{
		"model_name":    response.ModelName,
		"provider_id":   provider.GetID(),
		"provider_type": response.ProviderType,
		"backend_id":    response.BackendID,
	})

	return client, provider, response.BackendID, nil
}

// createProvider creates the appropriate provider instance based on the resolution response
func createProvider(response ResolutionResponse, httpClient *http.Client) (modelprovider.Provider, error) {
	capConfig := modelprovider.CapabilityConfig{
		ContextLength: response.ContextLength,
		CanChat:       response.Capabilities.CanChat,
		CanPrompt:     response.Capabilities.CanPrompt,
		CanEmbed:      response.Capabilities.CanEmbed,
		CanStream:     response.Capabilities.CanStream,
	}

	switch response.ProviderType {
	case "gemini":
		return modelprovider.NewGeminiProvider(
			response.APIKey,
			response.ModelName,
			[]string{response.BackendURL},
			capConfig,
			httpClient,
		), nil
	case "openai":
		return modelprovider.NewOpenAIProvider(
			response.APIKey,
			response.ModelName,
			[]string{response.BackendURL},
			capConfig,
			httpClient,
		), nil
	case "ollama":
		// For Ollama, backendURL is the URL to the Ollama server
		return modelprovider.NewOllamaModelProvider(
			response.ModelName,
			[]string{response.BackendURL},
			httpClient,
			capConfig,
		), nil
	case "vllm":
		return modelprovider.NewVLLMModelProvider(
			response.ModelName,
			[]string{response.BackendURL},
			httpClient,
			capConfig,
			response.APIKey,
		), nil
	case "mock":
		// For testing only
		return &modelprovider.MockProvider{
			ID:            "mock-" + response.ModelName,
			Name:          response.ModelName,
			ContextLength: response.ContextLength,
			CanChatFlag:   response.Capabilities.CanChat,
			CanEmbedFlag:  response.Capabilities.CanEmbed,
			CanStreamFlag: response.Capabilities.CanStream,
			CanPromptFlag: response.Capabilities.CanPrompt,
			Backends:      []string{response.BackendID},
		}, nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", response.ProviderType)
	}
}

func (b *MeshResolver) ResolveStream(ctx context.Context, req Request) (modelprovider.LLMStreamClient, modelprovider.Provider, string, error) {
	reportErr, reportChange, endFn := b.tracker.Start(
		ctx,
		"resolve",
		"stream_model",
		"provider_types", req.ProviderTypes,
		"model_names", req.ModelNames,
		"context_length", req.ContextLength,
	)
	defer endFn()

	// Convert to serializable form
	serializableReq := convertToSerializableRequest(req)

	// Create request payload
	request := struct {
		Operation string              `json:"operation"`
		Request   SerializableRequest `json:"request"`
	}{
		Operation: "stream",
		Request:   serializableReq,
	}

	// Serialize request
	reqData, err := json.Marshal(request)
	if err != nil {
		reportErr(fmt.Errorf("failed to serialize request: %w", err))
		return nil, nil, "", fmt.Errorf("failed to serialize request: %w", err)
	}

	// Send request to bus
	respData, err := b.bus.Request(ctx, "llmresolver.resolve", reqData)
	if err != nil {
		reportErr(fmt.Errorf("bus request failed: %w", err))
		return nil, nil, "", fmt.Errorf("bus request failed: %w", err)
	}

	// Parse response envelope
	var envelope ResolutionResponseEnvelope
	if err := json.Unmarshal(respData, &envelope); err != nil {
		reportErr(fmt.Errorf("failed to parse response envelope: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response envelope: %w", err)
	}

	// Check for application error
	if envelope.Error != "" {
		reportErr(fmt.Errorf("remote error: %s", envelope.Error))
		return nil, nil, "", fmt.Errorf("remote error: %s", envelope.Error)
	}

	// Parse the actual response data
	var response ResolutionResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		reportErr(fmt.Errorf("failed to parse response data: %w", err))
		return nil, nil, "", fmt.Errorf("failed to parse response data: %w", err)
	}

	// Create the appropriate provider and client based on provider type
	provider, err := createProvider(response, b.client)
	if err != nil {
		reportErr(fmt.Errorf("failed to create provider: %w", err))
		return nil, nil, "", fmt.Errorf("failed to create provider: %w", err)
	}

	// Get the stream connection
	client, err := provider.GetStreamConnection(ctx, response.BackendURL)
	if err != nil {
		reportErr(fmt.Errorf("failed to get stream connection: %w", err))
		return nil, nil, "", fmt.Errorf("failed to get stream connection: %w", err)
	}

	// Report selected provider
	reportChange("selected_provider", map[string]string{
		"model_name":    response.ModelName,
		"provider_id":   provider.GetID(),
		"provider_type": response.ProviderType,
		"backend_id":    response.BackendID,
	})

	return client, provider, response.BackendID, nil
}
