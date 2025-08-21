package modelrepo

import (
	"context"
)

// Provider is a provider of backend instances capable of executing requests with this Model.
type Provider interface {
	GetBackendIDs() []string // Available backend instances
	ModelName() string       // Model name (e.g., "llama2:latest")
	GetID() string           // unique identifier for the model provider
	GetType() string         // Type of the model provider
	GetContextLength() int   // Maximum context length supported
	CanChat() bool           // Supports chat interactions
	CanEmbed() bool          // Supports embeddings
	CanStream() bool         // Supports streaming
	CanPrompt() bool         // Supports prompting
	CanThink() bool          // Supports thinking
	GetChatConnection(ctx context.Context, backendID string) (LLMChatClient, error)
	GetPromptConnection(ctx context.Context, backendID string) (LLMPromptExecClient, error)
	GetEmbedConnection(ctx context.Context, backendID string) (LLMEmbedClient, error)
	GetStreamConnection(ctx context.Context, backendID string) (LLMStreamClient, error)
}
