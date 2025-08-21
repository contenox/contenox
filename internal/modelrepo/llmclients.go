package modelrepo

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatOption interface {
	SetTemperature(float64)
	SetMaxTokens(int)
}

type StreamParcel struct {
	Data  string
	Error error
}

// Client interfaces for different capabilities
type LLMChatClient interface {
	Chat(ctx context.Context, Messages []Message, opts ...ChatOption) (Message, error)
}

type LLMEmbedClient interface {
	Embed(ctx context.Context, prompt string) ([]float64, error)
}

type LLMStreamClient interface {
	Stream(ctx context.Context, prompt string) (<-chan *StreamParcel, error)
}

type LLMPromptExecClient interface {
	Prompt(ctx context.Context, systeminstruction string, temperature float32, prompt string) (string, error)
}
