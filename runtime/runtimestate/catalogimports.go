package runtimestate

import (
	_ "github.com/contenox/runtime/runtime/modelrepo/anthropic"
	_ "github.com/contenox/runtime/runtime/modelrepo/bedrock"
	_ "github.com/contenox/runtime/runtime/modelrepo/gemini"
	_ "github.com/contenox/runtime/runtime/modelrepo/llama"
	_ "github.com/contenox/runtime/runtime/modelrepo/llama/llamasession"
	_ "github.com/contenox/runtime/runtime/modelrepo/mistral"
	_ "github.com/contenox/runtime/runtime/modelrepo/ollama"
	_ "github.com/contenox/runtime/runtime/modelrepo/openai"
	_ "github.com/contenox/runtime/runtime/modelrepo/openrouter"
	_ "github.com/contenox/runtime/runtime/modelrepo/openvino"
	_ "github.com/contenox/runtime/runtime/modelrepo/vertex"
	_ "github.com/contenox/runtime/runtime/modelrepo/vllm"
)

// Import vendor catalog providers for registry-based modelrepo catalog construction.
