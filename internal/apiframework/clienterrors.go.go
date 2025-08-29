package apiframework

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// HandleAPIError processes error responses from the API
func HandleAPIError(resp *http.Response) error {
	// Read the entire response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API error with status %s (failed to read response body: %v)", resp.Status, err)
	}

	// Try to decode as OpenAI-style JSON error
	var apiErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Param   string `json:"param"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
		if apiErr.Error.Code != "" {
			return fmt.Errorf("%s (code: %s)", apiErr.Error.Message, apiErr.Error.Code)
		}
		return errors.New(apiErr.Error.Message)
	}

	// Fallback to generic error
	bodyStr := string(body)
	if len(bodyStr) > 100 {
		bodyStr = bodyStr[:100] + "..."
	}
	return fmt.Errorf("API error %d: %s", resp.StatusCode, bodyStr)
}
