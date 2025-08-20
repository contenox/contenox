package llmresolver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/contenox/activitytracker"
	"github.com/contenox/bus"
	"github.com/contenox/modelprovider"
	"github.com/contenox/runtime/llmrepo/internal/llmresolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockActivityTracker is a test implementation of ActivityTracker that records calls
type MockActivityTracker struct {
	Started        bool
	Operation      string
	Subject        string
	KvArgs         []any
	ReportedError  error
	ReportedChange struct {
		ID   string
		Data any
	}
	Ended bool
}

func (m *MockActivityTracker) Start(
	ctx context.Context,
	operation string,
	subject string,
	kvArgs ...any,
) (
	func(error),
	func(string, any),
	func(),
) {
	m.Started = true
	m.Operation = operation
	m.Subject = subject
	m.KvArgs = kvArgs

	return func(err error) {
			m.ReportedError = err
		},
		func(id string, data any) {
			m.ReportedChange.ID = id
			m.ReportedChange.Data = data
		},
		func() {
			m.Ended = true
		}
}

func TestUnit_MeshResolver_CanBeCreated(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})
	assert.NotNil(t, resolver)
}

func TestUnit_MeshResolver_SendsCorrectRequestForChat(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler to capture the request
	var capturedRequest []byte
	sub, err := pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		capturedRequest = data

		// Create a proper response with capabilities
		response := llmresolver.ResolutionResponse{
			ProviderType:  "mock",
			ModelName:     "test-model",
			BackendURL:    "http://mock-backend",
			BackendID:     "backend-1",
			ContextLength: 4096,
			Capabilities: struct {
				CanChat   bool `json:"can_chat"`
				CanPrompt bool `json:"can_prompt"`
				CanEmbed  bool `json:"can_embed"`
				CanStream bool `json:"can_stream"`
			}{
				CanChat: true,
			},
		}

		// Marshal the response data
		responseData, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		// Create and return the envelope
		envelope := llmresolver.ResolutionResponseEnvelope{
			Data: responseData,
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	_, _, _, err = resolver.ResolveChat(context.Background(), req)
	require.NoError(t, err)

	// Verify the request was correctly formatted
	var request llmresolver.ResolutionRequest
	err = json.Unmarshal(capturedRequest, &request)
	require.NoError(t, err)
	assert.Equal(t, "chat", request.Operation)

	var serializableReq llmresolver.SerializableRequest
	err = json.Unmarshal(request.Request, &serializableReq)
	require.NoError(t, err)
	assert.Equal(t, []string{"mock"}, serializableReq.ProviderTypes)
	assert.Equal(t, []string{"test-model"}, serializableReq.ModelNames)
	assert.Equal(t, 4096, serializableReq.ContextLength)
}

func TestUnit_MeshResolver_HandlesValidChatResponse(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler to return a valid response
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		response := llmresolver.ResolutionResponse{
			ProviderType:  "mock",
			ModelName:     "test-model",
			BackendURL:    "http://mock-backend",
			BackendID:     "backend-1",
			ContextLength: 4096,
			Capabilities: struct {
				CanChat   bool `json:"can_chat"`
				CanPrompt bool `json:"can_prompt"`
				CanEmbed  bool `json:"can_embed"`
				CanStream bool `json:"can_stream"`
			}{
				CanChat: true,
			},
		}

		// Marshal the response data
		responseData, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		// Create and return the envelope
		envelope := llmresolver.ResolutionResponseEnvelope{
			Data: responseData,
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	client, provider, backendID, err := resolver.ResolveChat(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, provider)
	assert.Equal(t, "backend-1", backendID)
	assert.Equal(t, "test-model", provider.ModelName())
	assert.Equal(t, "mock", provider.GetType())
	assert.Equal(t, "mock-test-model", provider.GetID())
}

func TestUnit_MeshResolver_HandlesBusError(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler that returns an error message in the response
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		// Return a valid JSON response with an error field (more realistic)
		envelope := llmresolver.ResolutionResponseEnvelope{
			Error: "bus error",
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	_, _, _, err = resolver.ResolveChat(context.Background(), req)
	require.Error(t, err)

	// The error should indicate a remote service error
	assert.Contains(t, err.Error(), "remote error")
	assert.Contains(t, err.Error(), "bus error")
}

func TestUnit_MeshResolver_HandlesInvalidResponse(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler that returns invalid JSON
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		return []byte(`invalid json`), nil
	})
	require.NoError(t, err)

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	_, _, _, err = resolver.ResolveChat(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestUnit_MeshResolver_Tracking(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler to return a valid response
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		response := llmresolver.ResolutionResponse{
			ProviderType:  "mock",
			ModelName:     "test-model",
			BackendURL:    "http://mock-backend",
			BackendID:     "backend-1",
			ContextLength: 4096,
			Capabilities: struct {
				CanChat   bool `json:"can_chat"`
				CanPrompt bool `json:"can_prompt"`
				CanEmbed  bool `json:"can_embed"`
				CanStream bool `json:"can_stream"`
			}{
				CanChat: true,
			},
		}

		// Marshal the response data
		responseData, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		// Create and return the envelope
		envelope := llmresolver.ResolutionResponseEnvelope{
			Data: responseData,
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)

	// Create a mock tracker
	tracker := &MockActivityTracker{}

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, tracker)

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	_, provider, _, err := resolver.ResolveChat(context.Background(), req)
	require.NoError(t, err)

	// Verify tracking
	assert.True(t, tracker.Started)
	assert.Equal(t, "resolve", tracker.Operation)
	assert.Equal(t, "chat_model", tracker.Subject)
	assert.Equal(t, "test-model", provider.ModelName())
	assert.Equal(t, "test-model", tracker.ReportedChange.Data.(map[string]string)["model_name"])
	assert.Equal(t, "mock", tracker.ReportedChange.Data.(map[string]string)["provider_type"])
	assert.True(t, tracker.Ended)
}

func TestUnit_MeshResolver_ProviderCreation(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler to return a valid response for different provider types
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		var request llmresolver.ResolutionRequest
		err := json.Unmarshal(data, &request)
		require.NoError(t, err)

		// Return different responses based on the operation
		var providerType string
		switch request.Operation {
		case "chat":
			providerType = "gemini"
		case "prompt":
			providerType = "openai"
		case "embed":
			providerType = "ollama"
		case "stream":
			providerType = "vllm"
		default:
			providerType = "mock"
		}

		response := llmresolver.ResolutionResponse{
			ProviderType:  providerType,
			ModelName:     "test-model",
			BackendURL:    fmt.Sprintf("http://%s-backend", providerType),
			BackendID:     fmt.Sprintf("%s-backend-1", providerType),
			APIKey:        "test-api-key",
			ContextLength: 4096,
			Capabilities: struct {
				CanChat   bool `json:"can_chat"`
				CanPrompt bool `json:"can_prompt"`
				CanEmbed  bool `json:"can_embed"`
				CanStream bool `json:"can_stream"`
			}{
				CanChat:   true,
				CanPrompt: true,
				CanEmbed:  true,
				CanStream: true,
			},
		}

		// Marshal the response data
		responseData, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		// Create and return the envelope
		envelope := llmresolver.ResolutionResponseEnvelope{
			Data: responseData,
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Test each operation type
	tests := []struct {
		name      string
		operation func(context.Context) (any, modelprovider.Provider, string, error)
	}{
		{
			name: "chat",
			operation: func(ctx context.Context) (any, modelprovider.Provider, string, error) {
				req := llmresolver.Request{
					ProviderTypes: []string{"test"},
					ModelNames:    []string{"test-model"},
					ContextLength: 4096,
				}
				client, provider, backend, err := resolver.ResolveChat(ctx, req)
				return client, provider, backend, err
			},
		},
		{
			name: "prompt",
			operation: func(ctx context.Context) (any, modelprovider.Provider, string, error) {
				req := llmresolver.Request{
					ProviderTypes: []string{"test"},
					ModelNames:    []string{"test-model"},
					ContextLength: 4096,
				}
				client, provider, backend, err := resolver.ResolvePromptExecute(ctx, req)
				return client, provider, backend, err
			},
		},
		{
			name: "embed",
			operation: func(ctx context.Context) (any, modelprovider.Provider, string, error) {
				embedReq := llmresolver.EmbedRequest{
					ModelName:    "test-model",
					ProviderType: "test",
				}
				client, provider, backend, err := resolver.ResolveEmbed(ctx, embedReq)
				return client, provider, backend, err
			},
		},
		{
			name: "stream",
			operation: func(ctx context.Context) (any, modelprovider.Provider, string, error) {
				req := llmresolver.Request{
					ProviderTypes: []string{"test"},
					ModelNames:    []string{"test-model"},
					ContextLength: 4096,
				}
				client, provider, backend, err := resolver.ResolveStream(ctx, req)
				return client, provider, backend, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, provider, backendID, err := tt.operation(context.Background())
			require.NoError(t, err)
			assert.NotNil(t, client)
			assert.NotNil(t, provider)
			assert.NotEmpty(t, backendID)
			assert.Equal(t, "test-model", provider.ModelName())

			// Instead of checking if ID contains the operation name,
			// check that the provider ID makes sense for the type
			switch tt.name {
			case "chat":
				assert.Equal(t, "gemini-test-model", provider.GetID())
			case "prompt":
				assert.Equal(t, "openai-test-model", provider.GetID())
			case "embed":
				assert.Equal(t, "ollama:test-model", provider.GetID())
			case "stream":
				assert.Equal(t, "vllm:test-model", provider.GetID())
			}
		})
	}
}

func TestUnit_MeshResolver_HandlesContextCancellation(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler that takes too long to respond
	_, err = pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		// Wait for context cancellation
		select {
		case <-time.After(2 * time.Second):
			response := llmresolver.ResolutionResponse{
				ProviderType: "mock",
				ModelName:    "test-model",
				BackendURL:   "http://mock",
			}

			// Marshal the response data
			responseData, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}

			// Create and return the envelope
			envelope := llmresolver.ResolutionResponseEnvelope{
				Data: responseData,
			}
			return json.Marshal(envelope)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	require.NoError(t, err)

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Make a request
	req := llmresolver.Request{
		ProviderTypes: []string{"mock"},
		ModelNames:    []string{"test-model"},
		ContextLength: 4096,
	}
	_, _, _, err = resolver.ResolveChat(ctx, req)
	require.Error(t, err)
}

func TestUnit_MeshResolver_EmbedRequestHandling(t *testing.T) {
	// Setup bus
	pubsub, cleanup, err := bus.NewTestPubSub()
	require.NoError(t, err)
	defer cleanup()

	// Set up a handler to capture the request
	var capturedRequest []byte
	sub, err := pubsub.Serve(context.Background(), "llmresolver.resolve", func(ctx context.Context, data []byte) ([]byte, error) {
		capturedRequest = data
		// Return a valid response with embedding capability
		response := llmresolver.ResolutionResponse{
			ProviderType:  "ollama",
			ModelName:     "text-embed-model",
			BackendURL:    "http://ollama",
			BackendID:     "ollama-backend-1",
			ContextLength: 4096,
			Capabilities: struct {
				CanChat   bool `json:"can_chat"`
				CanPrompt bool `json:"can_prompt"`
				CanEmbed  bool `json:"can_embed"`
				CanStream bool `json:"can_stream"`
			}{
				CanEmbed: true,
			},
		}

		// Marshal the response data
		responseData, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}

		// Create and return the envelope
		envelope := llmresolver.ResolutionResponseEnvelope{
			Data: responseData,
		}
		return json.Marshal(envelope)
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Create resolver
	resolver := llmresolver.NewMeshResolver(pubsub, activitytracker.NoopTracker{})

	// Make an embed request
	embedReq := llmresolver.EmbedRequest{
		ModelName:    "text-embed-model",
		ProviderType: "ollama",
	}
	client, provider, backendID, err := resolver.ResolveEmbed(context.Background(), embedReq)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, provider)
	assert.Equal(t, "ollama-backend-1", backendID)
	assert.Equal(t, "text-embed-model", provider.ModelName())

	// Verify the request was correctly formatted
	var request llmresolver.ResolutionRequest
	err = json.Unmarshal(capturedRequest, &request)
	require.NoError(t, err)
	assert.Equal(t, "embed", request.Operation)

	var serializableReq llmresolver.SerializableEmbedRequest
	err = json.Unmarshal(request.Request, &serializableReq)
	require.NoError(t, err)
	assert.Equal(t, "text-embed-model", serializableReq.ModelName)
	assert.Equal(t, "ollama", serializableReq.ProviderType)
}
