package runtimesdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/taskengine"
)

type HTTPTasksEnvService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPTasksEnvService creates a new HTTP client that implements execservice.TasksEnvService
func NewHTTPTasksEnvService(baseURL, token string, client *http.Client) execservice.TasksEnvService {
	if client == nil {
		client = http.DefaultClient
	}

	// Ensure baseURL doesn't end with a slash
	strings.TrimSuffix(baseURL, "/")

	return &HTTPTasksEnvService{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}
}

// Execute implements execservice.TasksEnvService.Execute
func (s *HTTPTasksEnvService) Execute(ctx context.Context, chain *taskengine.ChainDefinition, input any, inputType taskengine.DataType) (any, taskengine.DataType, []taskengine.CapturedStateUnit, error) {
	url := s.baseURL + "/tasks"

	// Create request payload
	request := map[string]interface{}{
		"input":     input,
		"inputType": inputType.String(),
		"chain":     chain,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, taskengine.DataTypeAny, nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Encode request body
	body, err := json.Marshal(request)
	if err != nil {
		return nil, taskengine.DataTypeAny, nil, err
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, taskengine.DataTypeAny, nil, err
	}
	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		return nil, taskengine.DataTypeAny, nil, apiframework.HandleAPIError(resp)
	}

	// Decode response
	var response struct {
		Output     any                            `json:"output"`
		OutputType string                         `json:"outputType"`
		State      []taskengine.CapturedStateUnit `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, taskengine.DataTypeAny, response.State, err
	}
	dt, err := taskengine.DataTypeFromString(response.OutputType)
	if err != nil {
		return nil, taskengine.DataTypeAny, response.State, err
	}
	converted, err := taskengine.ConvertToType(response.Output, dt)
	if err != nil {
		return nil, dt, response.State, fmt.Errorf("type conversion failed: %w", err)
	}
	return converted, dt, response.State, nil
}

// Supports implements execservice.TasksEnvService.Supports (via taskengine.HookRegistry)
func (s *HTTPTasksEnvService) Supports(ctx context.Context) ([]string, error) {
	url := s.baseURL + "/supported"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		return nil, apiframework.HandleAPIError(resp)
	}

	// Decode response
	var hooks []string
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, err
	}

	return hooks, nil
}
