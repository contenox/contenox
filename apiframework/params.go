package apiframework

import (
	"net/http"
	"strconv"
	"time"
)

// GetPathParam retrieves a URL path parameter by name. The call is also the
// documentation source: the OpenAPI generator (internal/openapigen) derives
// the path parameter itself from the route template, and attaches the
// description argument of a GetPathParam call found in the handler's own body
// to it. name must be a string literal (a non-literal name fails generation);
// naming a parameter that appears in no route template bound to the handler
// is likewise a generation error.
func GetPathParam(r *http.Request, name string, description string) string {
	return r.PathValue(name)
}

// GetQueryParam retrieves a URL query parameter by name. If the parameter is
// not present, it returns the provided defaultValue.
//
// The call is also the documentation source: the OpenAPI generator emits a
// string query parameter for every GetQueryParam call in the handler's own
// body, carrying this description and — when the defaultValue literal is
// non-empty — a schema default. name must be a string literal (a non-literal
// name fails generation); a non-literal defaultValue or description is
// consumed as absent. Calls the generator cannot see (nested closures, shared
// helpers called from the handler) are documented with the `// @param name
// type description...` escape-hatch annotation in the handler body instead.
func GetQueryParam(r *http.Request, name, defaultValue, description string) string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultValue
	}
	return val
}

// CursorParamDescription and LimitParamDescription are the canonical
// documentation for the two shared pagination query parameters. The OpenAPI
// generator emits them verbatim for every CursorParam/LimitParam/ListParams
// call site.
const (
	CursorParamDescription = "An optional RFC3339Nano timestamp to fetch the next page of results."
	LimitParamDescription  = "The maximum number of items to return per page."
)

// ListParams parses the two pagination query parameters shared by every
// keyset-paginated list endpoint: an optional RFC3339Nano `cursor` and an
// optional integer `limit`.
//
// Absent and invalid are deliberately different answers:
//
//   - absent cursor -> nil cursor, no error (start at the newest page)
//   - absent limit  -> defaultLimit, no error (the call site's own default,
//     which several stores in turn read as "apply my own default")
//   - malformed cursor, unparseable limit, or limit < 1 -> a classified
//     ErrInvalidParameterValue naming the offending parameter, which
//     mapErrorToStatus turns into 400 Bad Request regardless of the Operation
//     the handler passes to Error.
//
// The last point is the reason this lives here rather than in each handler: a
// bare fmt.Errorf parse error is unclassified, and the ListOperation fallback
// in mapErrorToStatus renders unclassified errors as 404 — so a garbage cursor
// used to read as "no such collection" on some routes and 422 on others.
//
// defaultLimit is a parameter rather than a package constant so that call
// sites keep owning their own default; this helper unifies the parsing and the
// error behavior, not the defaults.
//
// The cursor is validated before the limit, so a request that gets both wrong
// is told about the cursor first.
func ListParams(r *http.Request, defaultLimit int) (*time.Time, int, error) {
	cursor, err := CursorParam(r)
	if err != nil {
		return nil, 0, err
	}
	limit, err := LimitParam(r, defaultLimit)
	if err != nil {
		return nil, 0, err
	}
	return cursor, limit, nil
}

// CursorParam parses the optional RFC3339Nano `cursor` query parameter. An
// absent (or empty) cursor yields a nil pointer and no error; a malformed one
// yields ErrInvalidParameterValue.
func CursorParam(r *http.Request) (*time.Time, error) {
	raw := GetQueryParam(r, "cursor", "", CursorParamDescription)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, InvalidParameterValue("cursor", "invalid cursor format, expected an RFC3339Nano timestamp")
	}
	return &t, nil
}

// LimitParam parses the optional integer `limit` query parameter, returning
// defaultLimit when it is absent. An unparseable limit, or one below 1, yields
// ErrInvalidParameterValue; defaultLimit is trusted and returned unchecked, so
// a call site that wants "let the store decide" can still pass 0 or a negative
// value of its own.
func LimitParam(r *http.Request, defaultLimit int) (int, error) {
	raw := GetQueryParam(r, "limit", "", LimitParamDescription)
	if raw == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, InvalidParameterValue("limit", "invalid limit format, expected an integer")
	}
	if limit < 1 {
		return 0, InvalidParameterValue("limit", "limit must be greater than zero")
	}
	return limit, nil
}
