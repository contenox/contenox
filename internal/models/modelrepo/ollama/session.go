package ollama

import "github.com/contenox/contenox/internal/models/modelrepo"

// sessionOf is the conversation to name on a request, or empty when there is
// nothing to name or the endpoint would not understand the field.
//
// The session reaches the provider as the cache-affinity key the model layer
// derived, which is the same key it routed on — so the backend this request was
// sent to is the backend that key pinned. Upstream Ollama has no such field, so
// it is sent only to an endpoint that declared the extension.
func sessionOf(config *modelrepo.ChatConfig, declared bool) string {
	if !declared || config == nil || config.CacheHints == nil {
		return ""
	}
	return config.CacheHints.SessionKey
}
