package apiframework

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// HandleAPIError processes OpenAI-compatible API error responses.
func HandleAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API error with status %s (failed to read response body: %v)", resp.Status, err)
	}

	var apiErr struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    string  `json:"code"`
		} `json:"error"`
	}

	if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
		param := ""
		if apiErr.Error.Param != nil {
			param = *apiErr.Error.Param
		}
		return &APIError{
			err:       errors.New(apiErr.Error.Message),
			message:   apiErr.Error.Message,
			param:     param,
			errorType: apiErr.Error.Type,
			errorCode: apiErr.Error.Code,
		}
	}

	bodyStr := string(body)
	if len(bodyStr) > 100 {
		bodyStr = bodyStr[:100] + "..."
	}
	return fmt.Errorf("API error %d: %s", resp.StatusCode, bodyStr)
}
