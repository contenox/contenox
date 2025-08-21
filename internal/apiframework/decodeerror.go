package apiframework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError represents an error response from the API
type APIError struct {
	ErrorProperty string `json:"error"`
}

func (e *APIError) Error() string {
	return e.ErrorProperty
}

// IsAPIError checks if an error is an APIError
func IsAPIError(err error) bool {
	_, ok := err.(*APIError)
	return ok
}

// HandleAPIError processes error responses from the API
func HandleAPIError(resp *http.Response) error {
	// Read the entire response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API error with status %s (failed to read response body: %v)", resp.Status, err)
	}

	// Try to decode as JSON API error
	var apiErr APIError
	if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.ErrorProperty != "" {
		return &apiErr
	}

	// If not valid JSON error format, return a generic error with response body
	bodyStr := string(body)
	if len(bodyStr) > 100 {
		bodyStr = bodyStr[:100] + "..."
	}
	return fmt.Errorf("API error %d: %s", resp.StatusCode, bodyStr)
}
