package runtimestate

import (
	_ "github.com/contenox/runtime/modeld/anthropic"
	_ "github.com/contenox/runtime/modeld/bedrock"
	_ "github.com/contenox/runtime/modeld/gemini"
	_ "github.com/contenox/runtime/modeld/llama"
	_ "github.com/contenox/runtime/modeld/llama/llamasession"
	_ "github.com/contenox/runtime/modeld/mistral"
	_ "github.com/contenox/runtime/modeld/ollama"
	_ "github.com/contenox/runtime/modeld/openai"
	_ "github.com/contenox/runtime/modeld/openrouter"
	_ "github.com/contenox/runtime/modeld/openvino"
	_ "github.com/contenox/runtime/modeld/vertex"
	_ "github.com/contenox/runtime/modeld/vllm"
)

// Import vendor catalog providers for registry-based modelrepo catalog construction.
