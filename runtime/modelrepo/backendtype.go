package modelrepo

import "strings"

// CanonicalBackendType maps compatibility backend keywords to the implementation
// type used by the runtime. "local" canonicalizes to the "llama" provider.
func CanonicalBackendType(backendType string) string {
	normalized := strings.ToLower(strings.TrimSpace(backendType))
	if normalized == "local" {
		return "llama"
	}
	return normalized
}
