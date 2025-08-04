package llmrepo

import (
	"context"
	"fmt"

	libmodelprovider "github.com/contenox/modelprovider"
	"github.com/contenox/runtime/llmresolver"
)

type MockModelRepo struct {
	Provider libmodelprovider.Provider
}

func (m *MockModelRepo) GetDefaultSystemProvider(ctx context.Context) (libmodelprovider.Provider, error) {
	if m.Provider == nil {
		return nil, fmt.Errorf("provider is nil for prompt execution")
	}
	return m.Provider, nil
}

// TODO: Implement GetRuntime method
func (m *MockModelRepo) GetRuntime(ctx context.Context) llmresolver.ProviderFromRuntimeState {
	return func(_ context.Context, providerTypes ...string) ([]libmodelprovider.Provider, error) {
		return []libmodelprovider.Provider{
			m.Provider,
		}, nil
	}
}

// Fixed: Added modelName parameter to match interface
func (m *MockModelRepo) GetTokenizer(ctx context.Context, modelName string) (Tokenizer, error) {
	if m.Provider == nil {
		return nil, fmt.Errorf("provider is nil for prompt execution")
	}

	// Create a mock tokenizer implementation
	mockTokenizer := &mockOllamaTokenizer{
		modelName: modelName,
	}

	// Return an adapter that implements llmrepo.Tokenizer
	return &tokenizerAdapter{
		tokenizer: mockTokenizer,
		modelName: modelName,
	}, nil
}

func (m *MockModelRepo) GetAvailableProviders(ctx context.Context) ([]libmodelprovider.Provider, error) {
	return []libmodelprovider.Provider{
		m.Provider,
	}, nil
}

// mockOllamaTokenizer implements ollamatokenizer.Tokenizer
type mockOllamaTokenizer struct {
	modelName string
}

func (m *mockOllamaTokenizer) Tokenize(ctx context.Context, modelName string, prompt string) ([]int, error) {
	// Simple mock implementation - return token count as a slice of indices
	tokens := make([]int, len(prompt)/5+1) // Rough estimate
	for i := range tokens {
		tokens[i] = i
	}
	return tokens, nil
}

func (m *mockOllamaTokenizer) CountTokens(ctx context.Context, modelName string, prompt string) (int, error) {
	return len(prompt)/5 + 1, nil // Rough estimate
}

func (m *mockOllamaTokenizer) OptimalModel(ctx context.Context, baseModel string) (string, error) {
	return m.modelName, nil
}
