package compatapi

import (
	"net/http"

	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
)

// CompatDeps holds the dependencies required by the OpenAI/Ollama compat handlers.
type CompatDeps struct {
	Agent              agentservice.Agent
	Chains             taskchainservice.Service
	StateService       stateservice.Service
	DefaultChainRef    string
	DefaultFIMChainRef string
	DefaultModel       string
	DefaultProvider    string
	DefaultMaxTokens   string
	Auth               middleware.AuthZReader // nil = no auth required on compat routes
	Token              string                 // protects root-level mutating compat routes when set
}

// AddOpenAIRoutes registers the OpenAI-compatible routes on mux (apiMux, paths without /api prefix).
// Existing GET /openai/v1/models routes are NOT registered here — they already exist in backendapi.
func AddOpenAIRoutes(mux *http.ServeMux, deps CompatDeps) {
	chat := &chatHandler{deps: deps}
	fim := &fimHandler{deps: deps}

	mux.HandleFunc("POST /openai/v1/chat/completions", chat.handle)
	mux.HandleFunc("POST /openai/{chainID}/v1/chat/completions", chat.handle)
	mux.HandleFunc("POST /openai/v1/fim/completions", fim.handle)
	mux.HandleFunc("POST /openai/{chainID}/v1/fim/completions", fim.handle)
	// Legacy OpenAI /v1/completions alias — same as fim/completions.
	mux.HandleFunc("POST /openai/v1/completions", fim.handle)
}

// AddRootRoutes registers root-level /v1/* aliases on rootMux for clients that set
// api_url: http://host (e.g. tools pointed at port 11434 expecting OpenAI-compat routes).
func AddRootRoutes(mux *http.ServeMux, deps CompatDeps) {
	chat := &chatHandler{deps: deps}
	fim := &fimHandler{deps: deps}

	mux.HandleFunc("GET /v1/models", rootModels(deps))
	mux.HandleFunc("POST /v1/chat/completions", chat.handle)
	mux.HandleFunc("POST /v1/fim/completions", fim.handle)
	mux.HandleFunc("POST /v1/completions", fim.handle) // legacy alias
}
