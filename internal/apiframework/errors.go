package apiframework

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIError wraps an error with parameter context for API responses.
type APIError struct {
	err       error
	message   string
	param     string
	errorType string
	errorCode string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.message
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.err
}

// Param returns the parameter associated with the error.
func (e *APIError) Param() string {
	return e.param
}

// Code returns the error code.
func (e *APIError) Code() string {
	return e.errorCode
}

// IsAPIError checks if an error is an APIError and returns its components.
func IsAPIError(err error) (message, errorCode, errorType, param string, ok bool) {
	if e, ok := err.(*APIError); ok {
		return e.message, e.errorCode, e.errorType, e.param, true
	}
	return "", "", "", "", false
}

// GetErrorParam extracts parameter from error if available.
func GetErrorParam(err error) string {
	if paramErr, ok := err.(interface{ Param() string }); ok {
		return paramErr.Param()
	}
	return ""
}

type apiErrorPayload struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param,omitempty"`
	Code    string  `json:"code"`
}

type apiErrorResponse struct {
	Error apiErrorPayload `json:"error"`
}

func Error(w http.ResponseWriter, r *http.Request, err error, op Operation) error {
	status := mapErrorToStatus(op, err)

	// Get error components with fallback to status-based mapping
	message, errorCode, errorType, param, ok := IsAPIError(err)
	if !ok {
		message = err.Error()
		errorType, errorCode = getErrorTypeAndCode(status)
	} else if errorCode == "" || errorType == "" {
		// Fallback for partially populated APIErrors
		et, ec := getErrorTypeAndCode(status)
		if errorType == "" {
			errorType = et
		}
		if errorCode == "" {
			errorCode = ec
		}
	}

	// Skip body for 204
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return nil
	}

	// Prepare OpenAI-compatible error response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// Handle param as null when empty (OpenAI convention)
	var paramField *string
	if param != "" {
		paramField = &param
	}

	response := apiErrorResponse{
		Error: apiErrorPayload{
			Message: message,
			Type:    errorType,
			Param:   paramField,
			Code:    errorCode,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Printf("SERVER ERROR: Failed to encode error JSON: %v\n", err)
		return fmt.Errorf("encode error response: %w", err)
	}
	return nil
}
