package apiframework

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	libauth "github.com/contenox/authz"
	libdb "github.com/contenox/dbexec"
)

var (
	ErrInvalidParameterValue = errors.New("serverops: invalid parameter value type")
	ErrBadPathValue          = errors.New("serverops: bad path value")
	ErrImmutableModel        = errors.New("serverops: immutable model")
	ErrImmutablePool         = errors.New("serverops: immutable pool")
	ErrMissingParameter      = errors.New("serverops: missing parameter")
	ErrEmptyRequest          = errors.New("serverops: empty request")
	ErrEmptyRequestBody      = errors.New("serverops: empty request body")
)

// The generic error types for common HTTP status codes
var (
	// ErrBadRequest is a generic error for a 400 Bad Request
	ErrBadRequest = errors.New("serverops: bad request")
	// ErrUnprocessableEntity is a generic error for a 422 Unprocessable Entity
	ErrUnprocessableEntity = errors.New("serverops: unprocessable entity")
	// ErrNotFound is a generic error for a 404 Not Found
	ErrNotFound = errors.New("serverops: not found")
	// ErrConflict is a generic error for a 409 Conflict
	ErrConflict = errors.New("serverops: conflict")
	// ErrForbidden is a generic error for a 403 Forbidden
	ErrForbidden = errors.New("serverops: forbidden")
	// ErrInternalServerError is a generic error for a 500 Internal Server Error
	ErrInternalServerError = errors.New("serverops: internal server error")
	// ErrUnsupportedMediaType is a generic error for a 415 Unsupported Media Type
	ErrUnsupportedMediaType = errors.New("serverops: unsupported media type")
	// ErrUnauthorized is a generic error for a 401 Unauthorized
	ErrUnauthorized = errors.New("serverops: unauthorized")
)

// ErrFileSizeLimitExceeded indicates the specific file exceeded its allowed size limit.
var ErrFileSizeLimitExceeded = errors.New("serverops: file size limit exceeded")

// ErrFileEmpty indicates an attempt to upload an empty file.
var ErrFileEmpty = errors.New("serverops: file cannot be empty")

var ErrInvalidChain = errors.New("invalid chain definition")

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

// Map known error types to HTTP status codes
func mapErrorToStatus(op Operation, err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		// Error specifically from http.MaxBytesReader limit
		return http.StatusRequestEntityTooLarge // 413
	}
	if errors.Is(err, ErrFileSizeLimitExceeded) {
		// Error for specific file part exceeding header.Size limit
		return http.StatusRequestEntityTooLarge // 413
	}
	if errors.Is(err, http.ErrNotMultipart) {
		// Content-Type wasn't multipart/...
		return http.StatusUnsupportedMediaType // 415
	}
	if errors.Is(err, http.ErrMissingFile) {
		// The requested form field name was not found
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, ErrFileEmpty) {
		// Uploaded file had size 0
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, libauth.ErrNotAuthorized) {
		return http.StatusUnauthorized // 401
	}
	if op == AuthorizeOperation {
		return http.StatusForbidden // 403
	}
	if errors.Is(err, libauth.ErrTokenExpired) {
		return http.StatusUnauthorized // 401
	}
	// Token format/validation issues are client errors
	if errors.Is(err, libauth.ErrIssuedAtMissing) ||
		errors.Is(err, libauth.ErrIssuedAtInFuture) ||
		errors.Is(err, libauth.ErrIdentityMissing) ||
		errors.Is(err, libauth.ErrInvalidTokenClaims) ||
		errors.Is(err, libauth.ErrTokenMissing) ||
		errors.Is(err, libauth.ErrUnexpectedSigningMethod) ||
		errors.Is(err, libauth.ErrTokenParsingFailed) ||
		errors.Is(err, libauth.ErrTokenSigningFailed) {
		return http.StatusBadRequest // 400
	}

	if errors.Is(err, ErrEmptyRequest) {
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, ErrEmptyRequestBody) {
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, ErrBadRequest) {
		return http.StatusBadRequest // 400
	}

	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized // 401
	}
	if errors.Is(err, ErrForbidden) {
		return http.StatusForbidden // 403
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound // 404
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict // 409
	}
	if errors.Is(err, ErrUnsupportedMediaType) {
		return http.StatusUnsupportedMediaType // 415
	}
	if errors.Is(err, ErrInternalServerError) {
		return http.StatusInternalServerError // 500
	}
	if errors.Is(err, ErrUnprocessableEntity) {
		return http.StatusUnprocessableEntity // 422
	}

	if errors.Is(err, libdb.ErrNotFound) {
		return http.StatusNotFound // 404
	}
	// Constraint violations often mean client sent conflicting/invalid data
	if errors.Is(err, libdb.ErrUniqueViolation) ||
		errors.Is(err, libdb.ErrForeignKeyViolation) ||
		errors.Is(err, libdb.ErrNotNullViolation) ||
		errors.Is(err, libdb.ErrCheckViolation) ||
		errors.Is(err, libdb.ErrConstraintViolation) {
		return http.StatusConflict // 409
	}

	if errors.Is(err, libdb.ErrMaxRowsReached) {
		return http.StatusTooManyRequests // data-count limit reached scenario
	}
	// These DB errors might be client input or server issues, 409 or 422 are candidates
	if errors.Is(err, libdb.ErrDataTruncation) ||
		errors.Is(err, libdb.ErrNumericOutOfRange) ||
		errors.Is(err, libdb.ErrInvalidInputSyntax) ||
		errors.Is(err, libdb.ErrUndefinedColumn) || // Often server error (bad query, 500?)
		errors.Is(err, libdb.ErrUndefinedTable) { // Often server error (bad query, 500?)
		return http.StatusBadRequest // 400
	}
	// Concurrency/Server-side DB issues
	if errors.Is(err, libdb.ErrDeadlockDetected) ||
		errors.Is(err, libdb.ErrSerializationFailure) ||
		errors.Is(err, libdb.ErrLockNotAvailable) ||
		errors.Is(err, libdb.ErrQueryCanceled) { // Could be client cancel or server issue
		return http.StatusConflict // 409 (Maybe 503 Service Unavailable for some?)
	}

	// --- JSON Handling Errors ---
	if errors.Is(err, ErrEncodeInvalidJSON) {
		// Log this server-side, client gets generic 500
		fmt.Printf("SERVER ERROR: Failed to encode JSON response: %v\n", err)
		return http.StatusInternalServerError
	}
	if errors.Is(err, ErrDecodeInvalidJSON) {
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, ErrUnsupportedContentType) {
		return http.StatusUnsupportedMediaType // 415
	}
	if errors.Is(err, ErrMalformedContentType) {
		return http.StatusBadRequest // 400 (Malformed header)
	}
	if errors.Is(err, ErrReadingRequestBody) {
		// it can also represent server-side I/O problems unrelated to client input
		return http.StatusBadRequest // 400
	}
	if errors.Is(err, ErrInvalidParameterValue) || errors.Is(err, ErrBadPathValue) {
		return http.StatusBadRequest // 400
	}

	if errors.Is(err, ErrImmutableModel) {
		return http.StatusForbidden // 403
	}
	if errors.Is(err, ErrImmutablePool) {
		return http.StatusForbidden // 403
	}
	if errors.Is(err, ErrMissingParameter) {
		return http.StatusBadRequest // 400
	}

	if errors.Is(err, ErrInvalidChain) {
		return http.StatusBadRequest // 400
	}

	// if errors.Is(err, messagerepo.ErrMessageNotFound) {
	// 	return http.StatusNotFound // 404
	// }
	// if errors.Is(err, messagerepo.ErrSerializeMessage) ||
	// 	errors.Is(err, messagerepo.ErrDeserializeResponse) {
	// 	return http.StatusBadRequest // 400 (Assuming relates to bad input format)
	// }
	// if errors.Is(err, messagerepo.ErrIndexCreationFailed) ||
	// 	errors.Is(err, messagerepo.ErrIndexCheckFailed) {
	// 	return http.StatusInternalServerError // 500 (Internal index issue)
	// }
	// if errors.Is(err, messagerepo.ErrSearchFailed) {
	// 	// Could be bad query (400) or internal issue (500)
	// 	return http.StatusBadRequest // 400 (Assume bad search terms first)
	// }
	// if errors.Is(err, messagerepo.ErrUpdateFailed) ||
	// 	errors.Is(err, messagerepo.ErrDeleteFailed) {
	// 	// Failed operation on existing resource, 422 or 400?
	// 	return http.StatusUnprocessableEntity // 422 (The request was understood, but couldn't process instructions)
	// }

	// fallbacks if no specific error matched above.
	switch op {
	case CreateOperation, UpdateOperation:
		// If it wasn't a specific client error above (400, 409, 413, 415),
		return http.StatusUnprocessableEntity // 422
	case GetOperation, ListOperation:
		// Default to 404 if not found, otherwise maybe 500?
		return http.StatusNotFound // 404
	case DeleteOperation:
		// Default for delete could be 404 if not found, or 500?
		return http.StatusNotFound // 404
	case AuthorizeOperation:
		return http.StatusForbidden // 403
	case ServerOperation: // Explicitly marked as server-side issue
		return http.StatusInternalServerError // 500
	case ExecuteOperation:
		return http.StatusInternalServerError // 500
	default:
		// Catch-all for unknown operations or uncategorized errors
		fmt.Printf("SERVER ERROR: Unmapped error for operation %v: %v\n", op, err) // Replace with proper logging
		return http.StatusInternalServerError                                      // 500
	}
}

// Error sends a JSON-encoded error response with an appropriate status code
func Error(w http.ResponseWriter, r *http.Request, err error, op Operation) error {
	status := mapErrorToStatus(op, err)

	// Avoid writing body for 204 No Content
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	// Ensure Content-Type header is written before body in case of errors during Encode
	w.WriteHeader(status)

	response := map[string]string{"error": err.Error()}
	encodeErr := json.NewEncoder(w).Encode(response)

	// Log encoding errors server-side, as we can't send a response anymore.
	if encodeErr != nil {
		fmt.Printf("SERVER ERROR: Failed to encode error JSON response after writing header: %v (Original error: %v)\n", encodeErr, err)
		return fmt.Errorf("encode json: %w (original error: %v)", encodeErr, err)
	}

	return nil
}
