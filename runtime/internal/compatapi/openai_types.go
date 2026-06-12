package compatapi

// ChatCompletionRequest is the OpenAI /v1/chat/completions request body.
type ChatCompletionRequest struct {
	Model               string        `json:"model"`
	Messages            []ChatMessage `json:"messages"`
	Stream              bool          `json:"stream"`
	Temperature         *float64      `json:"temperature,omitempty"`
	MaxTokens           *int          `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int          `json:"max_completion_tokens,omitempty"`
	Stop                []string      `json:"stop,omitempty"`
}

// ChatMessage is a single message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// FIMCompletionRequest is the OpenAI /v1/fim/completions (and legacy /v1/completions) request body.
type FIMCompletionRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Suffix      string   `json:"suffix,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	Stream      bool     `json:"stream"`
	Stop        []string `json:"stop,omitempty"`
}

// chatCompletionChunk is one SSE frame for chat completions (delta.content style).
type chatCompletionChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []chatChunkChoice    `json:"choices"`
	Usage   *chatCompletionUsage `json:"usage,omitempty"`
}

type chatChunkChoice struct {
	Index        int       `json:"index"`
	Delta        chatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

type chatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatCompletionResponse is the non-streaming response for chat completions.
type chatCompletionResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []chatChoiceResponse `json:"choices"`
	Usage   chatCompletionUsage  `json:"usage"`
}

type chatChoiceResponse struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// fimChunk is one SSE frame for FIM/text completions (choices[0].text style).
type fimChunk struct {
	Choices []fimChunkChoice     `json:"choices"`
	Usage   *chatCompletionUsage `json:"usage,omitempty"`
}

type fimChunkChoice struct {
	Text         string  `json:"text"`
	FinishReason *string `json:"finish_reason"`
}

// fimCompletionResponse is the non-streaming response for FIM/text completions.
type fimCompletionResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []fimChoiceResponse `json:"choices"`
	Usage   chatCompletionUsage `json:"usage"`
}

type fimChoiceResponse struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}
