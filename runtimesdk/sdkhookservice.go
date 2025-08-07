package runtimesdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/runtimetypes"
)

// HTTPRemoteHookService implements the hookproviderservice.Service interface
// using HTTP calls to the API
type HTTPRemoteHookService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPRemoteHookService creates a new HTTP client that implements hookproviderservice.Service
func NewHTTPRemoteHookService(baseURL, token string, client *http.Client) hookproviderservice.Service {
	if client == nil {
		client = http.DefaultClient
	}

	// Ensure baseURL doesn't end with a slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &HTTPRemoteHookService{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}
}

// Create implements hookproviderservice.Service.Create
func (s *HTTPRemoteHookService) Create(ctx context.Context, hook *runtimetypes.RemoteHook) error {
	url := s.baseURL + "/hooks/remote"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("X-API-Key", s.token)
	}

	// Encode request body
	body, err := json.Marshal(hook)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode != http.StatusCreated {
		return apiframework.HandleAPIError(resp)
	}

	// Decode response into the provided hook
	if err := json.NewDecoder(resp.Body).Decode(hook); err != nil {
		return err
	}

	return nil
}

// Get implements hookproviderservice.Service.Get
func (s *HTTPRemoteHookService) Get(ctx context.Context, id string) (*runtimetypes.RemoteHook, error) {
	url := fmt.Sprintf("%s/hooks/remote/%s", s.baseURL, url.PathEscape(id))

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
	var hook runtimetypes.RemoteHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, err
	}

	return &hook, nil
}

// GetByName implements hookproviderservice.Service.GetByName
func (s *HTTPRemoteHookService) GetByName(ctx context.Context, name string) (*runtimetypes.RemoteHook, error) {
	// The API doesn't have a direct endpoint for getting hooks by name,
	// so we'll need to list all hooks and find the one with the matching name
	hooks, err := s.List(ctx, nil, 1000)
	if err != nil {
		return nil, err
	}

	for _, hook := range hooks {
		if hook.Name == name {
			return hook, nil
		}
	}

	return nil, fmt.Errorf("hook not found with name: %s", name)
}

// Update implements hookproviderservice.Service.Update
func (s *HTTPRemoteHookService) Update(ctx context.Context, hook *runtimetypes.RemoteHook) error {
	url := fmt.Sprintf("%s/hooks/remote/%s", s.baseURL, url.PathEscape(hook.ID))

	req, err := http.NewRequestWithContext(ctx, "PUT", url, nil)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Encode request body
	body, err := json.Marshal(hook)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		return apiframework.HandleAPIError(resp)
	}

	// Decode response into the provided hook
	if err := json.NewDecoder(resp.Body).Decode(hook); err != nil {
		return err
	}

	return nil
}

// Delete implements hookproviderservice.Service.Delete
func (s *HTTPRemoteHookService) Delete(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/hooks/remote/%s", s.baseURL, url.PathEscape(id))

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	// Set headers
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		return apiframework.HandleAPIError(resp)
	}

	return nil
}

// List implements hookproviderservice.Service.List
func (s *HTTPRemoteHookService) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.RemoteHook, error) {
	// Build URL with query parameters
	rURL := fmt.Sprintf("%s/hooks/remote?limit=%d", s.baseURL, limit)
	if createdAtCursor != nil {
		rURL += "&cursor=" + url.QueryEscape(createdAtCursor.Format(time.RFC3339Nano))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rURL, nil)
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
	var hooks []*runtimetypes.RemoteHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, err
	}

	return hooks, nil
}
