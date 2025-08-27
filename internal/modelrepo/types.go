package modelrepo

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatArgument interface {
	Apply(config *ChatConfig)
}

type StreamParcel struct {
	Data  string
	Error error
}

type ChatConfig struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Seed        *int     `json:"seed,omitempty"`
}

// Client interfaces
type LLMChatClient interface {
	Chat(ctx context.Context, messages []Message, args ...ChatArgument) (Message, error)
}

type LLMEmbedClient interface {
	Embed(ctx context.Context, prompt string) ([]float64, error)
}

type LLMStreamClient interface {
	Stream(ctx context.Context, prompt string, args ...ChatArgument) (<-chan *StreamParcel, error)
}

type LLMPromptExecClient interface {
	Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error)
}
