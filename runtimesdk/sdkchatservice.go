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
func (s *HTTPChatService) OpenAIChatCompletions(ctx context.Context, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	url := s.baseURL + "/v1/chat/completions"

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

// SetTaskChainID implements chatservice.Service.SetTaskChainID
func (s *HTTPChatService) SetTaskChainID(ctx context.Context, taskChainID string) error {
	url := s.baseURL + "/chat/taskchain"

	// Create request body
	request := struct {
		TaskChainID string `json:"taskChainID"`
	}{
		TaskChainID: taskChainID,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal task chain request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		return apiframework.HandleAPIError(resp)
	}

	return nil
}

// GetTaskChainID implements chatservice.Service.GetTaskChainID
func (s *HTTPChatService) GetTaskChainID(ctx context.Context) (string, error) {
	url := s.baseURL + "/chat/taskchain"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set headers
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		return "", apiframework.HandleAPIError(resp)
	}

	// Decode response
	var response struct {
		ChainID string `json:"taskChainID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode task chain ID response: %w", err)
	}

	return response.ChainID, nil
}
