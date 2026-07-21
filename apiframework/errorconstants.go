package apiframework

import (
	"errors"
	"net/http"

	"github.com/contenox/runtime/libauth"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
)

var (
	ErrInvalidParameterValue = errors.New("serverops: invalid parameter value type")
	ErrBadPathValue          = errors.New("serverops: bad path value")
	ErrBadQueryValue         = errors.New("serverops: bad query value")
	ErrImmutableModel        = errors.New("serverops: immutable model")
	ErrImmutableGroup        = errors.New("serverops: immutable group")
	ErrMissingParameter      = errors.New("serverops: missing parameter")
	ErrEmptyRequest          = errors.New("serverops: empty request")
	ErrEmptyRequestBody      = errors.New("serverops: empty request body")
	ErrInvalidChain          = errors.New("serverops: invalid chain definition")

	ErrBadRequest            = errors.New("serverops: bad request")
	ErrUnprocessableEntity   = errors.New("serverops: unprocessable entity")
	ErrNotFound              = errors.New("serverops: not found")
	ErrConflict              = errors.New("serverops: conflict")
	ErrForbidden             = errors.New("serverops: forbidden")
	ErrInternalServerError   = errors.New("serverops: internal server error")
	ErrUnsupportedMediaType  = errors.New("serverops: unsupported media type")
	ErrUnauthorized          = errors.New("serverops: unauthorized")
	ErrFileSizeLimitExceeded = errors.New("serverops: file size limit exceeded")
	ErrFileEmpty             = errors.New("serverops: file cannot be empty")
)

var errorMappings = map[error]struct {
	errorType string
	errorCode string
}{
	ErrInvalidParameterValue: {"invalid_request_error", "invalid_parameter_value"},
	ErrBadPathValue:          {"invalid_request_error", "bad_path_value"},
	ErrImmutableModel:        {"invalid_request_error", "immutable_model"},
	ErrImmutableGroup:        {"invalid_request_error", "immutable_group"},
	ErrMissingParameter:      {"invalid_request_error", "missing_parameter"},
	ErrEmptyRequest:          {"invalid_request_error", "empty_request"},
	ErrEmptyRequestBody:      {"invalid_request_error", "empty_request_body"},
	ErrBadRequest:            {"invalid_request_error", "bad_request"},
	ErrUnprocessableEntity:   {"invalid_request_error", "unprocessable_entity"},
	ErrNotFound:              {"invalid_request_error", "not_found"},
	ErrConflict:              {"invalid_request_error", "conflict"},
	ErrForbidden:             {"authorization_error", "forbidden"},
	ErrInternalServerError:   {"api_error", "internal_server_error"},
	ErrUnsupportedMediaType:  {"invalid_request_error", "unsupported_media_type"},
	ErrUnauthorized:          {"authentication_error", "unauthorized"},
	ErrFileSizeLimitExceeded: {"invalid_request_error", "file_size_limit_exceeded"},
	ErrFileEmpty:             {"invalid_request_error", "file_empty"},
	ErrInvalidChain:          {"invalid_request_error", "invalid_chain"},
}

func getErrorMapping(err error) (string, string) {
	for standardErr, mapping := range errorMappings {
		if errors.Is(err, standardErr) {
			return mapping.errorType, mapping.errorCode
		}
	}
	return "", ""
}

func getErrorTypeAndCode(status int) (string, string) {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error", "bad_request"
	case http.StatusUnauthorized:
		return "authentication_error", "unauthorized"
	case http.StatusForbidden:
		return "authorization_error", "forbidden"
	case http.StatusNotFound:
		return "invalid_request_error", "not_found"
	case http.StatusConflict:
		return "invalid_request_error", "conflict"
	case http.StatusRequestEntityTooLarge:
		return "invalid_request_error", "request_too_large"
	case http.StatusUnsupportedMediaType:
		return "invalid_request_error", "unsupported_media"
	case http.StatusUnprocessableEntity:
		return "invalid_request_error", "unprocessable_entity"
	case http.StatusTooManyRequests:
		return "rate_limit_error", "rate_limit_exceeded"
	case http.StatusInternalServerError:
		return "api_error", "internal_error"
	default:
		return "api_error", "unknown_error"
	}
}

type Operation uint16

const (
	CreateOperation Operation = iota
	GetOperation
	UpdateOperation
	DeleteOperation
	ListOperation
	AuthorizeOperation
	ServerOperation
	ExecuteOperation
)

func mapErrorToStatus(op Operation, err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, ErrFileSizeLimitExceeded) {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, http.ErrNotMultipart) {
		return http.StatusUnsupportedMediaType
	}
	if errors.Is(err, http.ErrMissingFile) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrFileEmpty) {
		return http.StatusBadRequest
	}
	if errors.Is(err, libauth.ErrNotAuthorized) {
		return http.StatusUnauthorized
	}
	if op == AuthorizeOperation {
		return http.StatusForbidden
	}
	if errors.Is(err, libauth.ErrTokenExpired) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, libauth.ErrIssuedAtMissing) ||
		errors.Is(err, libauth.ErrIssuedAtInFuture) ||
		errors.Is(err, libauth.ErrIdentityMissing) ||
		errors.Is(err, libauth.ErrInvalidTokenClaims) ||
		errors.Is(err, libauth.ErrTokenMissing) ||
		errors.Is(err, libauth.ErrUnexpectedSigningMethod) ||
		errors.Is(err, libauth.ErrTokenParsingFailed) ||
		errors.Is(err, libauth.ErrTokenSigningFailed) {
		return http.StatusBadRequest
	}

	if errors.Is(err, ErrEmptyRequest) ||
		errors.Is(err, ErrEmptyRequestBody) ||
		errors.Is(err, ErrBadRequest) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, ErrForbidden) {
		return http.StatusForbidden
	}
	// The vfs control-plane refusal (a path at/under ~/.contenox: config, DB,
	// policies, agents) is a security boundary — 403 Forbidden carrying the
	// teaching text, not a mystery 404/500. Surfaces on every /files, search, and
	// fs-tool path that resolves through vfs. See runtime/vfs/controlplane.go.
	if errors.Is(err, vfs.ErrControlPlane) {
		return http.StatusForbidden
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, ErrUnsupportedMediaType) {
		return http.StatusUnsupportedMediaType
	}
	if errors.Is(err, ErrInternalServerError) {
		return http.StatusInternalServerError
	}
	if errors.Is(err, ErrUnprocessableEntity) {
		return http.StatusUnprocessableEntity
	}

	if errors.Is(err, libdb.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, libdb.ErrUniqueViolation) ||
		errors.Is(err, libdb.ErrForeignKeyViolation) ||
		errors.Is(err, libdb.ErrNotNullViolation) ||
		errors.Is(err, libdb.ErrCheckViolation) ||
		errors.Is(err, libdb.ErrConstraintViolation) {
		return http.StatusConflict
	}
	if errors.Is(err, libdb.ErrMaxRowsReached) {
		return http.StatusTooManyRequests
	}
	if errors.Is(err, libdb.ErrDataTruncation) ||
		errors.Is(err, libdb.ErrNumericOutOfRange) ||
		errors.Is(err, libdb.ErrInvalidInputSyntax) ||
		errors.Is(err, libdb.ErrUndefinedColumn) ||
		errors.Is(err, libdb.ErrUndefinedTable) {
		return http.StatusBadRequest
	}
	if errors.Is(err, libdb.ErrDeadlockDetected) ||
		errors.Is(err, libdb.ErrSerializationFailure) ||
		errors.Is(err, libdb.ErrLockNotAvailable) ||
		errors.Is(err, libdb.ErrQueryCanceled) {
		return http.StatusConflict
	}
	if errors.Is(err, runtimetypes.ErrLimitParamExceeded) ||
		errors.Is(err, runtimetypes.ErrAppendLimitExceeded) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrInvalidParameterValue) ||
		errors.Is(err, ErrBadPathValue) ||
		errors.Is(err, ErrMissingParameter) ||
		errors.Is(err, ErrInvalidChain) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrImmutableModel) || errors.Is(err, ErrImmutableGroup) {
		return http.StatusForbidden
	}

	switch op {
	case CreateOperation, UpdateOperation:
		return http.StatusUnprocessableEntity
	case GetOperation, ListOperation, DeleteOperation:
		return http.StatusNotFound
	case AuthorizeOperation:
		return http.StatusForbidden
	case ServerOperation, ExecuteOperation:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func NewAPIError(err error, message, param string) *APIError {
	errorType, errorCode := getErrorMapping(err)
	if message == "" {
		message = err.Error()
	}
	return &APIError{
		err:       err,
		message:   message,
		param:     param,
		errorType: errorType,
		errorCode: errorCode,
	}
}

func InvalidParameterValue(param string, message ...string) *APIError {
	return NewAPIError(ErrInvalidParameterValue, messageOrDefault("Invalid parameter value", message), param)
}

func MissingParameter(param string, message ...string) *APIError {
	return NewAPIError(ErrMissingParameter, messageOrDefault("Missing required parameter", message), param)
}

func Unauthorized(message ...string) *APIError {
	return NewAPIError(ErrUnauthorized, messageOrDefault("Unauthorized access", message), "")
}

func Forbidden(message ...string) *APIError {
	return NewAPIError(ErrForbidden, messageOrDefault("Forbidden access", message), "")
}

func NotFound(message ...string) *APIError {
	return NewAPIError(ErrNotFound, messageOrDefault("Resource not found", message), "")
}

func BadRequest(message ...string) *APIError {
	return NewAPIError(ErrBadRequest, messageOrDefault("Bad request", message), "")
}

func UnprocessableEntity(message ...string) *APIError {
	return NewAPIError(ErrUnprocessableEntity, messageOrDefault("Unprocessable entity", message), "")
}

func Conflict(message ...string) *APIError {
	return NewAPIError(ErrConflict, messageOrDefault("Conflict", message), "")
}

func InternalServerError(message ...string) *APIError {
	return NewAPIError(ErrInternalServerError, messageOrDefault("Internal server error", message), "")
}

func UnsupportedMediaType(message ...string) *APIError {
	return NewAPIError(ErrUnsupportedMediaType, messageOrDefault("Unsupported media type", message), "")
}

func FileSizeLimitExceeded(message ...string) *APIError {
	return NewAPIError(ErrFileSizeLimitExceeded, messageOrDefault("File size limit exceeded", message), "")
}

func FileEmpty(message ...string) *APIError {
	return NewAPIError(ErrFileEmpty, messageOrDefault("File cannot be empty", message), "")
}

func InvalidChain(message ...string) *APIError {
	return NewAPIError(ErrInvalidChain, messageOrDefault("Invalid chain definition", message), "")
}

func BadPathValue(param string, message ...string) *APIError {
	return NewAPIError(ErrBadPathValue, messageOrDefault("Bad path value", message), param)
}

func ImmutableModel(message ...string) *APIError {
	return NewAPIError(ErrImmutableModel, messageOrDefault("Model is immutable", message), "")
}

func ImmutableGroup(message ...string) *APIError {
	return NewAPIError(ErrImmutableGroup, messageOrDefault("Group is immutable", message), "")
}

func messageOrDefault(def string, message []string) string {
	if len(message) > 0 && message[0] != "" {
		return message[0]
	}
	return def
}
