package runtimesdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/contenox/runtime/chatservice"
	"github.com/contenox/runtime/internal/apiframework"
	"github.com/contenox/runtime/taskengine"
)

// HTTPChatService implements the chatservice.Service interface
// using HTTP calls to the API
type HTTPChatService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPChatService creates a new HTTP client that implements chatservice.Service
func NewHTTPChatService(baseURL, token string, client *http.Client) chatservice.Service {
	if client == nil {
		client = http.DefaultClient
	}

	// Ensure baseURL doesn't end with a slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &HTTPChatService{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}
}

// OpenAIChatCompletions implements chatservice.Service.OpenAIChatCompletions
func (s *HTTPChatService) OpenAIChatCompletions(ctx context.Context, chainID string, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	url := s.baseURL + "/" + chainID + "/v1/chat/completions"

	// Marshal the request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	// Create request
	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, nil, err
	}

	// Set headers
	reqHTTP.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		reqHTTP.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Execute request
	resp, err := s.client.Do(reqHTTP)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		return nil, nil, apiframework.HandleAPIError(resp)
	}

	// Decode response
	var response struct {
		ID                string                                `json:"id"`
		Object            string                                `json:"object"`
		Created           int64                                 `json:"created"`
		Model             string                                `json:"model"`
		Choices           []taskengine.OpenAIChatResponseChoice `json:"choices"`
		Usage             taskengine.OpenAITokenUsage           `json:"usage"`
		SystemFingerprint string                                `json:"system_fingerprint"`
		StackTrace        []taskengine.CapturedStateUnit        `json:"stackTrace"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, nil, fmt.Errorf("failed to decode chat response: %w", err)
	}

	// Convert to OpenAIChatResponse
	chatResponse := &taskengine.OpenAIChatResponse{
		ID:                response.ID,
		Object:            response.Object,
		Created:           response.Created,
		Model:             response.Model,
		Choices:           response.Choices,
		Usage:             response.Usage,
		SystemFingerprint: response.SystemFingerprint,
	}

	return chatResponse, response.StackTrace, nil
}
