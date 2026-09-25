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

// WithCode returns a copy of e whose machine-readable envelope code is code,
// leaving status, type, message and param as they were. It exists for the
// refusals a client maps by code — the seat and entitlement walls — where the
// sentinel-derived code would say only "conflict" or "forbidden".
func (e *APIError) WithCode(code string) *APIError {
	clone := *e
	clone.errorCode = code
	return &clone
}

// IsAPIError reports whether err is an *APIError and returns its components.
func IsAPIError(err error) (message, errorCode, errorType, param string, ok bool) {
	if e, ok := err.(*APIError); ok {
		return e.message, e.errorCode, e.errorType, e.param, true
	}
	return "", "", "", "", false
}

// GetErrorParam extracts the parameter from err when it carries one.
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

// Error writes an OpenAI-compatible error response for err, choosing the HTTP
// status from the operation and the error's identity. Components the error does
// not carry fall back to a status-derived mapping, and param is emitted as null
// when empty, which is the OpenAI convention.
func Error(w http.ResponseWriter, r *http.Request, err error, op Operation) error {
	status := mapErrorToStatus(op, err)

	message, errorCode, errorType, param, ok := IsAPIError(err)
	if !ok {
		message = err.Error()
		errorType, errorCode = getErrorTypeAndCode(status)
	} else if errorCode == "" || errorType == "" {
		et, ec := getErrorTypeAndCode(status)
		if errorType == "" {
			errorType = et
		}
		if errorCode == "" {
			errorCode = ec
		}
	}

	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

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
		return fmt.Errorf("encode error response: %w", err)
	}
	return nil
}
