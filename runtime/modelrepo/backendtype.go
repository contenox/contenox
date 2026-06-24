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

// IsLocalBackendType reports whether backendType denotes the local modeld
// provider family (llama / openvino / local / modeld). modeld is a single daemon
// that autodetects its inference engine from the hardware, so these are not
// independent routes like ollama/openai — they are one logical local provider
// whose live engine the runtime observes. Resolution treats any local alias as a
// request for "whatever modeld is currently serving", which is why a user's
// llama-vs-openvino pick does not have to match the autodetected engine.
func IsLocalBackendType(backendType string) bool {
	switch CanonicalBackendType(backendType) {
	case "llama", "openvino", "modeld":
		return true
	default:
		return false
	}
}
