package apiframework

import "net/http"

// GetPathParam retrieves a URL path parameter by name and is used to enforce
// that all path parameters are documented for the OpenAPI generator.
func GetPathParam(r *http.Request, name string, description string) string {
	return r.PathValue(name)
}

// GetQueryParam retrieves a URL query parameter by name. If the parameter is not
// present, it returns the provided defaultValue.
func GetQueryParam(r *http.Request, name, defaultValue, description string) string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultValue
	}
	return val
}
