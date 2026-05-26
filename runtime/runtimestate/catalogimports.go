package runtimestate

import (
	_ "github.com/contenox/agent/runtime/modelrepo/gemini"
	_ "github.com/contenox/agent/runtime/modelrepo/local"
	_ "github.com/contenox/agent/runtime/modelrepo/ollama"
	_ "github.com/contenox/agent/runtime/modelrepo/openai"
	_ "github.com/contenox/agent/runtime/modelrepo/vertex"
	_ "github.com/contenox/agent/runtime/modelrepo/vllm"
)

// Import vendor catalog providers for registry-based modelrepo catalog construction.
