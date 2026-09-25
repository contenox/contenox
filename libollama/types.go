package libollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Capability is a model capability reported by /api/show.
type Capability string

const (
	CapabilityCompletion = Capability("completion")
	CapabilityTools      = Capability("tools")
	CapabilityVision     = Capability("vision")
	CapabilityEmbedding  = Capability("embedding")
	CapabilityThinking   = Capability("thinking")
)

// ImageData is the raw binary content of an image, rendered as base64 in JSON.
type ImageData []byte

// Duration is a time.Duration that marshals as an Ollama duration string
// ("10m0s") and unmarshals from either that or a number of seconds.
type Duration struct {
	time.Duration
}

// MarshalJSON emits -1 for negative durations and a duration string otherwise.
func (d Duration) MarshalJSON() ([]byte, error) {
	if d.Duration < 0 {
		return []byte("-1"), nil
	}
	return []byte("\"" + d.Duration.String() + "\""), nil
}

// UnmarshalJSON accepts a number of seconds or a Go duration string.
func (d *Duration) UnmarshalJSON(b []byte) (err error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	d.Duration = 5 * time.Minute

	switch t := v.(type) {
	case float64:
		if t < 0 {
			d.Duration = time.Duration(math.MaxInt64)
		} else {
			d.Duration = time.Duration(t * float64(time.Second))
		}
	case string:
		d.Duration, err = time.ParseDuration(t)
		if err != nil {
			return err
		}
		if d.Duration < 0 {
			d.Duration = time.Duration(math.MaxInt64)
		}
	default:
		return fmt.Errorf("unsupported duration type: '%T'", v)
	}

	return nil
}

// ThinkValue is the think request field: a bool, or one of "high", "medium", "low".
type ThinkValue struct {
	Value any
}

// MarshalJSON emits the underlying bool or string; a nil value emits null.
func (t *ThinkValue) MarshalJSON() ([]byte, error) {
	if t == nil || t.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.Value)
}

// UnmarshalJSON accepts a bool or one of "high", "medium", "low".
func (t *ThinkValue) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		t.Value = b
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "high" && s != "medium" && s != "low" {
			return fmt.Errorf("invalid think value: %q (must be \"high\", \"medium\", \"low\", true, or false)", s)
		}
		t.Value = s
		return nil
	}

	return fmt.Errorf("think must be a boolean or string (\"high\", \"medium\", \"low\", true, or false)")
}

type orderedMap[V any] struct {
	keys   []string
	values map[string]V
}

func newOrderedMap[V any]() *orderedMap[V] {
	return &orderedMap[V]{values: make(map[string]V)}
}

func (m *orderedMap[V]) set(key string, value V) {
	if m.values == nil {
		m.values = make(map[string]V)
	}
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *orderedMap[V]) MarshalJSON() ([]byte, error) {
	if m == nil || m.values == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *orderedMap[V]) UnmarshalJSON(data []byte) error {
	m.keys = nil
	m.values = nil
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected JSON object, got %v", tok)
	}

	m.values = make(map[string]V)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %v", keyTok)
		}
		var value V
		if err := dec.Decode(&value); err != nil {
			return err
		}
		m.set(key, value)
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	return nil
}

// Message is one entry in a chat sequence.
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Thinking   string      `json:"thinking,omitempty"`
	Images     []ImageData `json:"images,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolName   string      `json:"tool_name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

// UnmarshalJSON lowercases Role for case-insensitive matching.
func (m *Message) UnmarshalJSON(b []byte) error {
	type Alias Message
	var a Alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}

	*m = Message(a)
	m.Role = strings.ToLower(m.Role)
	return nil
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the tool and carries its arguments.
type ToolCallFunction struct {
	Index     int                       `json:"index"`
	Name      string                    `json:"name"`
	Arguments ToolCallFunctionArguments `json:"arguments"`
}

// ToolCallFunctionArguments holds tool-call arguments in the order emitted.
type ToolCallFunctionArguments struct {
	om *orderedMap[any]
}

// MarshalJSON encodes arguments preserving key order.
func (t ToolCallFunctionArguments) MarshalJSON() ([]byte, error) {
	if t.om == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(t.om)
}

// UnmarshalJSON decodes arguments preserving key order.
func (t *ToolCallFunctionArguments) UnmarshalJSON(data []byte) error {
	t.om = newOrderedMap[any]()
	return json.Unmarshal(data, t.om)
}

// String renders the arguments as a JSON object string.
func (t *ToolCallFunctionArguments) String() string {
	if t == nil || t.om == nil {
		return "{}"
	}
	bts, _ := json.Marshal(t.om)
	return string(bts)
}

// Tools is the tool list attached to a chat request.
type Tools []Tool

// Tool is one tool offered to the model.
type Tool struct {
	Type     string       `json:"type"`
	Items    any          `json:"items,omitempty"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable tool.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  ToolFunctionParameters `json:"parameters"`
}

// ToolFunctionParameters describes a tool's JSON Schema parameters.
type ToolFunctionParameters struct {
	Type       string             `json:"type"`
	Defs       any                `json:"$defs,omitempty"`
	Items      any                `json:"items,omitempty"`
	Required   []string           `json:"required,omitempty"`
	Properties *ToolPropertiesMap `json:"properties"`
}

// ToolPropertiesMap holds schema properties in declaration order.
type ToolPropertiesMap struct {
	om *orderedMap[ToolProperty]
}

// MarshalJSON encodes properties preserving declaration order.
func (t ToolPropertiesMap) MarshalJSON() ([]byte, error) {
	if t.om == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.om)
}

// UnmarshalJSON decodes properties preserving declaration order.
func (t *ToolPropertiesMap) UnmarshalJSON(data []byte) error {
	t.om = newOrderedMap[ToolProperty]()
	return json.Unmarshal(data, t.om)
}

// ToolProperty is one JSON Schema property of a tool's parameters.
type ToolProperty struct {
	AnyOf       []ToolProperty     `json:"anyOf,omitempty"`
	Type        PropertyType       `json:"type,omitempty"`
	Items       any                `json:"items,omitempty"`
	Description string             `json:"description,omitempty"`
	Enum        []any              `json:"enum,omitempty"`
	Properties  *ToolPropertiesMap `json:"properties,omitempty"`
}

// PropertyType is a JSON Schema type as a single string or an array of strings.
type PropertyType []string

// MarshalJSON collapses single-item types to a bare string.
func (pt PropertyType) MarshalJSON() ([]byte, error) {
	if len(pt) == 1 {
		return json.Marshal(pt[0])
	}
	return json.Marshal([]string(pt))
}

// UnmarshalJSON parses a bare string or array of strings.
func (pt *PropertyType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*pt = []string{s}
		return nil
	}

	var a []string
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*pt = a
	return nil
}

// GenerateRequest is the body of POST /api/generate.
type GenerateRequest struct {
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt"`
	Suffix          string          `json:"suffix,omitempty"`
	System          string          `json:"system,omitempty"`
	Template        string          `json:"template,omitempty"`
	Context         []int           `json:"context,omitempty"`
	Stream          *bool           `json:"stream,omitempty"`
	Raw             bool            `json:"raw,omitempty"`
	Format          json.RawMessage `json:"format,omitempty"`
	KeepAlive       *Duration       `json:"keep_alive,omitempty"`
	Images          []ImageData     `json:"images,omitempty"`
	Options         map[string]any  `json:"options,omitempty"`
	Think           *ThinkValue     `json:"think,omitempty"`
	Truncate        *bool           `json:"truncate,omitempty"`
	Shift           *bool           `json:"shift,omitempty"`
	DebugRenderOnly bool            `json:"_debug_render_only,omitempty"`
	Logprobs        bool            `json:"logprobs,omitempty"`
	TopLogprobs     int             `json:"top_logprobs,omitempty"`
}

// ChatRequest is the body of POST /api/chat.
type ChatRequest struct {
	Model           string          `json:"model"`
	Messages        []Message       `json:"messages"`
	Stream          *bool           `json:"stream,omitempty"`
	Format          json.RawMessage `json:"format,omitempty"`
	KeepAlive       *Duration       `json:"keep_alive,omitempty"`
	Tools           `json:"tools,omitempty"`
	Options         map[string]any `json:"options,omitempty"`
	Think           *ThinkValue    `json:"think,omitempty"`
	Truncate        *bool          `json:"truncate,omitempty"`
	Shift           *bool          `json:"shift,omitempty"`
	DebugRenderOnly bool           `json:"_debug_render_only,omitempty"`
	Logprobs        bool           `json:"logprobs,omitempty"`
	TopLogprobs     int            `json:"top_logprobs,omitempty"`
}

// Metrics are execution metrics in completion responses.
type Metrics struct {
	TotalDuration      time.Duration `json:"total_duration,omitempty"`
	PeakMemory         uint64        `json:"peak_memory,omitempty"`
	LoadDuration       time.Duration `json:"load_duration,omitempty"`
	PromptEvalCount    int           `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration time.Duration `json:"prompt_eval_duration,omitempty"`
	EvalCount          int           `json:"eval_count,omitempty"`
	EvalDuration       time.Duration `json:"eval_duration,omitempty"`
}

// TokenLogprob is the log probability of one token alternative.
type TokenLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []int   `json:"bytes,omitempty"`
}

// Logprob is the log probability of a generated token plus its alternatives.
type Logprob struct {
	TokenLogprob
	TopLogprobs []TokenLogprob `json:"top_logprobs,omitempty"`
}

// ChatResponse is one frame of POST /api/chat.
type ChatResponse struct {
	Model       string    `json:"model"`
	RemoteModel string    `json:"remote_model,omitempty"`
	RemoteHost  string    `json:"remote_host,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Message     Message   `json:"message"`
	Done        bool      `json:"done"`
	DoneReason  string    `json:"done_reason,omitempty"`
	Logprobs    []Logprob `json:"logprobs,omitempty"`

	Metrics
}

// GenerateResponse is one frame of POST /api/generate.
type GenerateResponse struct {
	Model       string     `json:"model"`
	RemoteModel string     `json:"remote_model,omitempty"`
	RemoteHost  string     `json:"remote_host,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Response    string     `json:"response"`
	Thinking    string     `json:"thinking,omitempty"`
	Done        bool       `json:"done"`
	DoneReason  string     `json:"done_reason,omitempty"`
	Context     []int      `json:"context,omitempty"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	Logprobs    []Logprob  `json:"logprobs,omitempty"`

	Metrics
}

// EmbedRequest is the body of POST /api/embed.
type EmbedRequest struct {
	Model      string         `json:"model"`
	Input      any            `json:"input"`
	KeepAlive  *Duration      `json:"keep_alive,omitempty"`
	Truncate   *bool          `json:"truncate,omitempty"`
	Dimensions int            `json:"dimensions,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

// EmbedResponse is the response to POST /api/embed.
type EmbedResponse struct {
	Model           string        `json:"model"`
	Embeddings      [][]float32   `json:"embeddings"`
	TotalDuration   time.Duration `json:"total_duration,omitempty"`
	LoadDuration    time.Duration `json:"load_duration,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
}

// ShowRequest is the body of POST /api/show.
type ShowRequest struct {
	Model    string         `json:"model"`
	System   string         `json:"system,omitempty"`
	Template string         `json:"template,omitempty"`
	Verbose  bool           `json:"verbose,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

// ShowResponse is the response to POST /api/show.
type ShowResponse struct {
	License      string         `json:"license,omitempty"`
	Modelfile    string         `json:"modelfile,omitempty"`
	Parameters   string         `json:"parameters,omitempty"`
	Template     string         `json:"template,omitempty"`
	System       string         `json:"system,omitempty"`
	Details      ModelDetails   `json:"details,omitempty"`
	Messages     []Message      `json:"messages,omitempty"`
	RemoteModel  string         `json:"remote_model,omitempty"`
	RemoteHost   string         `json:"remote_host,omitempty"`
	ModelInfo    map[string]any `json:"model_info,omitempty"`
	Capabilities []Capability   `json:"capabilities,omitempty"`
	ModifiedAt   time.Time      `json:"modified_at,omitempty"`
}

// ListResponse is the response to GET /api/tags.
type ListResponse struct {
	Models []ListModelResponse `json:"models"`
}

// ListModelResponse is one model in a ListResponse.
type ListModelResponse struct {
	Name        string       `json:"name"`
	Model       string       `json:"model"`
	RemoteModel string       `json:"remote_model,omitempty"`
	RemoteHost  string       `json:"remote_host,omitempty"`
	ModifiedAt  time.Time    `json:"modified_at"`
	Size        int64        `json:"size"`
	Digest      string       `json:"digest"`
	Details     ModelDetails `json:"details,omitempty"`
}

// ModelDetails describes a model's format and provenance.
type ModelDetails struct {
	ParentModel       string   `json:"parent_model,omitempty"`
	Format            string   `json:"format,omitempty"`
	Family            string   `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameter_size,omitempty"`
	QuantizationLevel string   `json:"quantization_level,omitempty"`
}
