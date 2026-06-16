package modeld

import "strings"

// CanonicalBackendType maps compatibility backend keywords to the implementation
// type used by the runtime. "local" is the historical embedded GGUF keyword; the
// implementation now lives under the feature-complete "llama" provider.
func CanonicalBackendType(backendType string) string {
	normalized := strings.ToLower(strings.TrimSpace(backendType))
	if normalized == "local" {
		return "llama"
	}
	return normalized
}
