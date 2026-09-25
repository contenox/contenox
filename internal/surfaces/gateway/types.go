package gateway

import (
	"encoding/json"
	"fmt"
	"time"
)

// TagsResponse describes the models available for a caller.
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// UsageResponse is the authenticated client's metered usage and allowances for
// one model.
type UsageResponse struct {
	ClientID   string          `json:"client_id"`
	Model      string          `json:"model"`
	Windows    UsageWindows    `json:"windows"`
	Allowances UsageAllowances `json:"allowances"`
}

// UsageWindows carries each period enforced by the gateway meter.
type UsageWindows struct {
	FiveHour UsageWindow `json:"five_hour"`
	Week     UsageWindow `json:"week"`
	Month    UsageWindow `json:"month"`
	Total    UsageWindow `json:"total"`
}

// UsageWindow is one meter snapshot and the UTC boundary it started at.
type UsageWindow struct {
	Kind  string        `json:"kind"`
	Start time.Time     `json:"start,omitempty"`
	Usage UsageCounters `json:"usage"`
}

// UsageCounters separates provider-reported counts from the discounted counts
// used to enforce allowances.
type UsageCounters struct {
	PromptTokens          int64 `json:"prompt_tokens"`
	CompletionTokens      int64 `json:"completion_tokens"`
	ThinkingTokens        int64 `json:"thinking_tokens"`
	VisibleOutputTokens   int64 `json:"visible_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
	CacheReadTokens       int64 `json:"cache_read_tokens"`
	CacheWriteTokens      int64 `json:"cache_write_tokens"`
	EffectiveInputTokens  int64 `json:"effective_input_tokens"`
	EffectiveOutputTokens int64 `json:"effective_output_tokens"`
	EffectiveTotalTokens  int64 `json:"effective_total_tokens"`
	CostMicrodollars      int64 `json:"cost_microdollars"`
	ImageCount            int64 `json:"image_count"`
	AudioBytes            int64 `json:"audio_bytes"`
}

// UsageAllowances states the limits and multipliers carried by the calling key.
// Zero limits are unlimited.
type UsageAllowances struct {
	WeeklyOutputTokens         int64   `json:"weekly_output_tokens"`
	WeeklyInputTokens          int64   `json:"weekly_input_tokens"`
	FiveHourTokens             int64   `json:"five_hour_tokens"`
	MonthlyBudgetMicrodollars  int64   `json:"monthly_budget_microdollars"`
	WeeklyImages               int64   `json:"weekly_images"`
	WeeklyAudioMiB             int64   `json:"weekly_audio_mib"`
	CacheDiscountMultiplier    float64 `json:"cache_discount_multiplier"`
	ThinkingDiscountMultiplier float64 `json:"thinking_discount_multiplier"`
}

// ModelInfo describes one available model in TagsResponse.
type ModelInfo struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails describes model metadata.
type ModelDetails struct {
	ParentModel       string   `json:"parent_model,omitempty"`
	Format            string   `json:"format,omitempty"`
	Family            string   `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameter_size,omitempty"`
	QuantizationLevel string   `json:"quantization_level,omitempty"`
}

// ShowRequest requests details for a specific model.
type ShowRequest struct {
	Model    string `json:"model"`
	System   string `json:"system,omitempty"`
	Template string `json:"template,omitempty"`
	Verbose  bool   `json:"verbose,omitempty"`
}

// ShowResponse returns detailed model parameters and template information.
type ShowResponse struct {
	License   string       `json:"license,omitempty"`
	Modelfile string       `json:"modelfile,omitempty"`
	Template  string       `json:"template,omitempty"`
	System    string       `json:"system,omitempty"`
	Details   ModelDetails `json:"details"`
	// Capabilities is what the model can do, in the names Ollama reports from
	// /api/show: completion, tools, vision, thinking, embedding. A client reads
	// them to decide whether a model may take a turn at all, so an absent list
	// is a model that can do nothing.
	Capabilities []string `json:"capabilities,omitempty"`
	// ModelInfo carries the model's own numbers under the keys Ollama uses,
	// which is "<architecture>.context_length" for the context window.
	ModelInfo map[string]any `json:"model_info,omitempty"`
	// Parameters is the Modelfile-style parameter list; num_predict is the
	// output ceiling and num_ctx the context the model was loaded with.
	Parameters string    `json:"parameters,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ChatMessage represents a single message in a chat history. Content is text
// only: attachments travel in Images and tool traffic in ToolCalls and
// ToolCallID.
type ChatMessage struct {
	Role     string   `json:"role"`
	Content  string   `json:"content"`
	Thinking string   `json:"thinking,omitempty"`
	Images   []string `json:"images,omitempty"`
	// Audios is raw WAV, base64 in JSON, on the field the upstream contract
	// proposes; the format is named by its magic bytes. A payload that is not WAV
	// is refused rather than forwarded as something a provider would misread.
	Audios     []string   `json:"audios,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest is the payload sent to POST /api/chat, mirroring the Ollama chat
// contract. An option this service cannot honour is accepted rather than
// refused, so a client's turn is not rejected for carrying one — but Format and
// Logprobs change what a caller receives rather than merely how it is routed,
// and no provider in the tree implements either, so a client that needs them
// must be answered by a provider that does.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   *bool         `json:"stream,omitempty"`
	// Format asks for structured output (a JSON schema or the string "json").
	// The Ollama provider carries the field on its own request and never
	// populates it, so this is accepted and dropped.
	Format json.RawMessage `json:"format,omitempty"`
	Tools  []Tool          `json:"tools,omitempty"`
	// Options is the Ollama option bag. temperature, top_p, seed and
	// num_predict reach the provider; num_ctx does not, because a provider takes
	// it as a load option and nothing in the runtime's provider layer sets one.
	// Options the gateway does not know are ignored rather than refused.
	Options map[string]any `json:"options,omitempty"`
	Think   *ThinkValue    `json:"think,omitempty"`
	// Logprobs and TopLogprobs ask for per-token alternatives. No provider
	// config carries them, so both are accepted and dropped.
	Logprobs    bool `json:"logprobs,omitempty"`
	TopLogprobs int  `json:"top_logprobs,omitempty"`
	// KeepAlive is how long the client asks the upstream to hold the model
	// resident, as an Ollama duration string or a number of seconds. The
	// upstream is OpenAI-compatible and holds no local model, so it is not
	// forwarded.
	KeepAlive any `json:"keep_alive,omitempty"`
	// Truncate and Shift ask the upstream to shorten overflowing history rather
	// than fail the turn; both reach the provider's chat config.
	Truncate *bool `json:"truncate,omitempty"`
	Shift    *bool `json:"shift,omitempty"`
	// DebugRenderOnly asks for the rendered prompt instead of a completion,
	// which the proxy cannot answer; it is accepted so the request shape is the
	// runtime's, and ignored.
	DebugRenderOnly bool `json:"_debug_render_only,omitempty"`
	// Session names the conversation this turn belongs to, so successive turns
	// of one conversation reach the same backend and its prefix cache stays
	// warm. It is a gateway extension rather than part of the Ollama contract:
	// unstated, the conversation's opening identifies it instead, and a request
	// that carries neither resolves randomly.
	Session string `json:"contenox_session,omitempty"`
}

// ThinkValue is the reasoning toggle of a chat turn: a bool enabling or
// disabling thinking, or one of "high", "medium" and "low". A nil pointer
// leaves the upstream's own reasoning default in place.
type ThinkValue struct {
	Value any
}

// MarshalJSON emits the underlying bool or level string.
func (t *ThinkValue) MarshalJSON() ([]byte, error) {
	if t == nil || t.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.Value)
}

// UnmarshalJSON accepts a bool or a reasoning level string.
func (t *ThinkValue) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v.(type) {
	case bool, string, nil:
		t.Value = v
		return nil
	}
	return fmt.Errorf("think must be a bool or a level string, got %s", string(data))
}

// Tool is one callable tool offered to the model.
type Tool struct {
	Type     string       `json:"type"`
	Items    any          `json:"items,omitempty"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable tool and the JSON Schema of its arguments.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

// ToolCall is a tool invocation in a message: requested by the model on an
// assistant turn, or reported back on a tool turn.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the invoked tool and carries its arguments as a JSON
// object, opaque here because the schema is the caller's, not the proxy's.
type ToolCallFunction struct {
	Index     int    `json:"index,omitempty"`
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

// TokenLogprob is the log probability of one token alternative.
type TokenLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []int   `json:"bytes,omitempty"`
}

// Logprob is the log probability of a generated token plus its alternatives.
type Logprob struct {
	Token       string         `json:"token"`
	Logprob     float64        `json:"logprob"`
	Bytes       []int          `json:"bytes,omitempty"`
	TopLogprobs []TokenLogprob `json:"top_logprobs,omitempty"`
}

// ChatResponse is the payload returned by POST /api/chat. Error carries a
// mid-stream failure: the status line is already sent by then, so the failure
// travels as the last frame, which is how upstream reports one too.
type ChatResponse struct {
	Model              string      `json:"model"`
	RemoteModel        string      `json:"remote_model,omitempty"`
	RemoteHost         string      `json:"remote_host,omitempty"`
	CreatedAt          time.Time   `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	DoneReason         string      `json:"done_reason,omitempty"`
	Error              string      `json:"error,omitempty"`
	Logprobs           []Logprob   `json:"logprobs,omitempty"`
	TotalDuration      int64       `json:"total_duration,omitempty"`
	PeakMemory         int64       `json:"peak_memory,omitempty"`
	LoadDuration       int64       `json:"load_duration,omitempty"`
	PromptEvalCount    int         `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64       `json:"prompt_eval_duration,omitempty"`
	EvalCount          int         `json:"eval_count,omitempty"`
	EvalDuration       int64       `json:"eval_duration,omitempty"`
}

// GenerateRequest is the payload sent to POST /api/generate.
type GenerateRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	System string   `json:"system,omitempty"`
	Stream *bool    `json:"stream,omitempty"`
	Audios []string `json:"audios,omitempty"`
	// Session names the conversation this turn belongs to, on the same terms as
	// ChatRequest.Session.
	Session string `json:"contenox_session,omitempty"`
}

// GenerateResponse is the payload returned by POST /api/generate. Error travels
// with a mid-stream failure for the same reason ChatResponse.Error does.
type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Thinking           string    `json:"thinking,omitempty"`
	Done               bool      `json:"done"`
	DoneReason         string    `json:"done_reason,omitempty"`
	Error              string    `json:"error,omitempty"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// VersionResponse is the reply to GET /api/version, and it is the vanilla shape
// on purpose: a client that never handshakes is a client that should not have to
// know what this is. What this endpoint is beyond Ollama lives on the contenox
// route, which vanilla Ollama does not serve.
type VersionResponse struct {
	Version string `json:"version"`
}

// ContenoxResponse is the reply to GET /api/contenox: the extensions this
// endpoint understands, named rather than implied by the product.
type ContenoxResponse struct {
	Product       string   `json:"product"`
	Version       string   `json:"version"`
	Build         string   `json:"build,omitempty"`
	OllamaVersion string   `json:"ollama_version,omitempty"`
	Extensions    []string `json:"extensions,omitempty"`
}

// EmbedRequest is the body of POST /api/embed. Input is one string or an array
// of them, which is the shape Ollama accepts; the reply carries one vector per
// input in the order they were sent.
type EmbedRequest struct {
	Model   string         `json:"model"`
	Input   any            `json:"input"`
	Options map[string]any `json:"options,omitempty"`
	// Truncate is accepted for wire compatibility and left to the provider, which
	// truncates by its own default.
	Truncate *bool `json:"truncate,omitempty"`
	// Dimensions is refused rather than ignored: an index built from one
	// dimension cannot be searched with another, so answering with the model's
	// own vectors instead of the requested size would fail far from here.
	Dimensions int `json:"dimensions,omitempty"`
}

// EmbedResponse is the reply to POST /api/embed.
type EmbedResponse struct {
	Model           string        `json:"model"`
	Embeddings      [][]float32   `json:"embeddings"`
	TotalDuration   time.Duration `json:"total_duration,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
}
