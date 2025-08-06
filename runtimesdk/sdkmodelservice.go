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
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/runtimetypes"
)

// HTTPModelService implements the modelservice.Service interface
// using HTTP calls to the API
type HTTPModelService struct {
	client  *http.Client
	baseURL string
	token   string
}

// NewHTTPModelService creates a new HTTP client that implements modelservice.Service
func NewHTTPModelService(baseURL, token string, client *http.Client) modelservice.Service {
	if client == nil {
		client = http.DefaultClient
	}

	// Ensure baseURL doesn't end with a slash
	strings.TrimSuffix(baseURL, "/")

	return &HTTPModelService{
		client:  client,
		baseURL: baseURL,
		token:   token,
	}
}

// Append implements modelservice.Service.Append
func (s *HTTPModelService) Append(ctx context.Context, model *runtimetypes.Model) error {
	url := s.baseURL + "/models"

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
	body, err := json.Marshal(model)
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

	// Decode response into the provided model
	// Note: API sets model.ID = model.Model, so we'll get the ID back
	if err := json.NewDecoder(resp.Body).Decode(model); err != nil {
		return err
	}

	return nil
}

// List implements modelservice.Service.List
func (s *HTTPModelService) List(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*runtimetypes.Model, error) {
	// Build URL with query parameters
	rUrl := fmt.Sprintf("%s/models?limit=%d", s.baseURL, limit)
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

	// The API returns OpenAI-compatible format, but we need to convert to store.Model
	type OpenAIModel struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	type ListResponse struct {
		Object string        `json:"object"`
		Data   []OpenAIModel `json:"data"`
	}

	var response ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	// Convert to []*store.Model
	models := make([]*runtimetypes.Model, 0, len(response.Data))
	for _, openAIModel := range response.Data {
		createdAt := time.Unix(openAIModel.Created, 0)
		models = append(models, &runtimetypes.Model{
			ID:        openAIModel.ID,
			Model:     openAIModel.ID,
			CreatedAt: createdAt,
			UpdatedAt: createdAt, // API doesn't provide separate UpdatedAt
		})
	}

	return models, nil
}

// Delete implements modelservice.Service.Delete
func (s *HTTPModelService) Delete(ctx context.Context, modelName string) error {
	// Properly escape the model name for the URL path
	url := fmt.Sprintf("%s/models/%s", s.baseURL, url.PathEscape(modelName))

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
