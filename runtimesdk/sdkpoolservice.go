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
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/runtimetypes"
)

// HTTPPoolService implements the poolservice.Service interface
// using HTTP calls to the API
type HTTPPoolService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPPoolService creates a new HTTP client that implements poolservice.Service
func NewHTTPPoolService(baseURL, token string, client *http.Client) poolservice.Service {
	if client == nil {
		client = http.DefaultClient
	}

	// Ensure baseURL doesn't end with a slash
	strings.TrimSuffix(baseURL, "/")

	return &HTTPPoolService{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}
}

// Create implements poolservice.Service.Create
func (s *HTTPPoolService) Create(ctx context.Context, pool *runtimetypes.Pool) error {
	url := s.baseURL + "/pools"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	// Encode request body
	body, err := json.Marshal(pool)
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

	// Decode response into the provided pool
	if err := json.NewDecoder(resp.Body).Decode(pool); err != nil {
		return err
	}

	return nil
}

// GetByID implements poolservice.Service.GetByID
func (s *HTTPPoolService) GetByID(ctx context.Context, id string) (*runtimetypes.Pool, error) {
	url := fmt.Sprintf("%s/pools/%s", s.baseURL, url.PathEscape(id))

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
	var pool runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pool); err != nil {
		return nil, err
	}

	return &pool, nil
}

// GetByName implements poolservice.Service.GetByName
func (s *HTTPPoolService) GetByName(ctx context.Context, name string) (*runtimetypes.Pool, error) {
	url := fmt.Sprintf("%s/pool-by-name/%s", s.baseURL, url.PathEscape(name))

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
	var pool runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pool); err != nil {
		return nil, err
	}

	return &pool, nil
}

// Update implements poolservice.Service.Update
func (s *HTTPPoolService) Update(ctx context.Context, pool *runtimetypes.Pool) error {
	url := fmt.Sprintf("%s/pools/%s", s.baseURL, url.PathEscape(pool.ID))

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
	body, err := json.Marshal(pool)
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

	// Decode response into the provided pool
	if err := json.NewDecoder(resp.Body).Decode(pool); err != nil {
		return err
	}

	return nil
}

// Delete implements poolservice.Service.Delete
func (s *HTTPPoolService) Delete(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/pools/%s", s.baseURL, url.PathEscape(id))

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
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return apiframework.HandleAPIError(resp)
	}

	return nil
}

// ListAll implements poolservice.Service.ListAll
func (s *HTTPPoolService) ListAll(ctx context.Context) ([]*runtimetypes.Pool, error) {
	url := s.baseURL + "/pools"

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
	var pools []*runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		return nil, err
	}

	return pools, nil
}

// ListByPurpose implements poolservice.Service.ListByPurpose
func (s *HTTPPoolService) ListByPurpose(ctx context.Context, purpose string, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Pool, error) {
	// Build URL with query parameters
	rUrl := fmt.Sprintf("%s/pool-by-purpose/%s?limit=%d", s.baseURL, url.PathEscape(purpose), limit)
	if createdAtCursor != nil {
		rUrl += "&cursor=" + url.QueryEscape(createdAtCursor.Format(time.RFC3339Nano))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rUrl, nil)
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
	var pools []*runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		return nil, err
	}

	return pools, nil
}

// AssignBackend implements poolservice.Service.AssignBackend
func (s *HTTPPoolService) AssignBackend(ctx context.Context, poolID, backendID string) error {
	url := fmt.Sprintf("%s/backend-associations/%s/backends/%s",
		s.baseURL, url.PathEscape(poolID), url.PathEscape(backendID))

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
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
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return apiframework.HandleAPIError(resp)
	}

	return nil
}

// RemoveBackend implements poolservice.Service.RemoveBackend
func (s *HTTPPoolService) RemoveBackend(ctx context.Context, poolID, backendID string) error {
	url := fmt.Sprintf("%s/backend-associations/%s/backends/%s",
		s.baseURL, url.PathEscape(poolID), url.PathEscape(backendID))

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

// ListBackends implements poolservice.Service.ListBackends
func (s *HTTPPoolService) ListBackends(ctx context.Context, poolID string) ([]*runtimetypes.Backend, error) {
	url := fmt.Sprintf("%s/backend-associations/%s/backends", s.baseURL, url.PathEscape(poolID))

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
	var backends []*runtimetypes.Backend
	if err := json.NewDecoder(resp.Body).Decode(&backends); err != nil {
		return nil, err
	}

	return backends, nil
}

// ListPoolsForBackend implements poolservice.Service.ListPoolsForBackend
func (s *HTTPPoolService) ListPoolsForBackend(ctx context.Context, backendID string) ([]*runtimetypes.Pool, error) {
	url := fmt.Sprintf("%s/backend-associations/%s/pools", s.baseURL, url.PathEscape(backendID))

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
	var pools []*runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		return nil, err
	}

	return pools, nil
}

// AssignModel implements poolservice.Service.AssignModel
func (s *HTTPPoolService) AssignModel(ctx context.Context, poolID, modelID string) error {
	url := fmt.Sprintf("%s/model-associations/%s/models/%s",
		s.baseURL, url.PathEscape(poolID), url.PathEscape(modelID))

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
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

// RemoveModel implements poolservice.Service.RemoveModel
func (s *HTTPPoolService) RemoveModel(ctx context.Context, poolID, modelID string) error {
	url := fmt.Sprintf("%s/model-associations/%s/models/%s",
		s.baseURL, url.PathEscape(poolID), url.PathEscape(modelID))

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

// ListModels implements poolservice.Service.ListModels
func (s *HTTPPoolService) ListModels(ctx context.Context, poolID string) ([]*runtimetypes.Model, error) {
	url := fmt.Sprintf("%s/model-associations/%s/models", s.baseURL, url.PathEscape(poolID))

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
	var models []*runtimetypes.Model
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}

	return models, nil
}

// ListPoolsForModel implements poolservice.Service.ListPoolsForModel
func (s *HTTPPoolService) ListPoolsForModel(ctx context.Context, modelID string) ([]*runtimetypes.Pool, error) {
	url := fmt.Sprintf("%s/model-associations/%s/pools", s.baseURL, url.PathEscape(modelID))

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
	var pools []*runtimetypes.Pool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		return nil, err
	}

	return pools, nil
}
