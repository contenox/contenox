// Package modeld is the split-out, transport-facing owner of the model
// repository. It defines the provider-facing contracts for LLM backends: the
// Provider interface (capabilities + client factories), the per-capability
// client interfaces (LLMPromptExecClient, LLMChatClient, LLMEmbedClient,
// LLMStreamClient), and the shared request/response types (Message,
// ChatResult, StreamParcel, Tool, ChatArgument).
//
// Concrete providers live in subpackages (openai, gemini, vertex, vllm,
// ollama, local). Higher-level code depends only on the interfaces declared
// here; provider subpackages are imported for their side effects to register
// catalogs.
//
// Daemon (see daemon.go) is the in-process singleton that owns backend state
// behind a mutex, and the transport subpackage exposes that state over the wire
// (gRPC today, HTTP later).
package modeld
