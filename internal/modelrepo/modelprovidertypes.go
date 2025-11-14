package modelrepo

import "context"

type ChatResult struct {
	Message   Message
	ToolCalls []ToolCall
}

type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"` // only "function" for now
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Message now supports OpenAI-style tool calling:
// - assistant messages can carry tool_calls
// - tool messages can carry tool_call_id
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// For tool calling (OpenAI / vLLM compatible). These are optional and
	// simply serialized into the JSON payload. Providers that don't care
	// can ignore them, but OpenAI will now see valid tool call history.
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ChatArgument interface {
	Apply(config *ChatConfig)
}

type StreamParcel struct {
	Data  string
	Error error
}

type Tool struct {
	Type     string        `json:"type"`
	Function *FunctionTool `json:"function,omitempty"`
}

type FunctionTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type ChatConfig struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Seed        *int     `json:"seed,omitempty"`
	Tools       []Tool   `json:"tools,omitempty"`
}

// Client interfaces
type LLMChatClient interface {
	Chat(ctx context.Context, messages []Message, args ...ChatArgument) (ChatResult, error)
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
