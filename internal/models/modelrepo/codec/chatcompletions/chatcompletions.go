// Package chatcompletions is a transport-agnostic codec for the OpenAI Chat
// Completions wire format. It maps between neutral modelrepo types and the
// OpenAI-compatible JSON shape, and does no I/O.
package chatcompletions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/contenox/internal/kernel/reasoning"
	"github.com/contenox/contenox/internal/models/modelrepo"
)

// Request is the OpenAI-compatible chat/completions request body. Build emits
// max_tokens; decoders also accept max_completion_tokens.
type Request struct {
	Model               string             `json:"model"`
	Messages            []Message          `json:"messages"`
	Temperature         *float64           `json:"temperature,omitempty"`
	MaxTokens           *int               `json:"max_tokens,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	Seed                *int               `json:"seed,omitempty"`
	Tools               []Tool             `json:"tools,omitempty"`
	ToolChoice          string             `json:"tool_choice,omitempty"`
	Format              *ResponseFormat    `json:"response_format,omitempty"`
	Logprobs            *bool              `json:"logprobs,omitempty"`
	TopLogprobs         *int               `json:"top_logprobs,omitempty"`
	Stream              *bool              `json:"stream,omitempty"`
	StreamOptions       *StreamOptions     `json:"stream_options,omitempty"`
	ReasoningEffort     string             `json:"reasoning_effort,omitempty"`
	MaxCompletionTokens *int               `json:"max_completion_tokens,omitempty"`
	Session             string             `json:"contenox_session,omitempty"`
	N                   *int               `json:"n,omitempty"`
	Stop                json.RawMessage    `json:"stop,omitempty"`
	FrequencyPenalty    *float64           `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64           `json:"presence_penalty,omitempty"`
	LogitBias           map[string]float64 `json:"logit_bias,omitempty"`
	ParallelToolCalls   *bool              `json:"parallel_tool_calls,omitempty"`
	Modalities          []string           `json:"modalities,omitempty"`
	Store               bool               `json:"store,omitempty"`
}

// StreamOptions requests the trailing usage chunk of a streamed completion.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ResponseFormat is the structured-output control of a request: any JSON
// object, or one conforming to JSONSchema.
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema names a response schema and carries it.
type JSONSchema struct {
	Name   string `json:"name"`
	Schema any    `json:"schema"`
}

// Message describes one chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ContentPart describes one text, image, or audio part of a message.
type ContentPart struct {
	Type       string      `json:"type"`
	Text       string      `json:"text,omitempty"`
	ImageURL   *ImageURL   `json:"image_url,omitempty"`
	InputAudio *InputAudio `json:"input_audio,omitempty"`
}

// InputAudio carries base64 audio and its format.
type InputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// ImageURL describes an image URL or data URI.
type ImageURL struct {
	URL string `json:"url"`
}

// ToolCall describes one function invocation in a message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function invocation and its JSON arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool describes one function available to the model.
type Tool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

// ToolDefinition describes a callable function.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Build converts neutral messages and config into a chat/completions Request.
// It returns a map of sanitized tool name to original so decoders can translate
// tool-call names back.
func Build(model string, messages []modelrepo.Message, cfg *modelrepo.ChatConfig) (Request, map[string]string) {
	req := Request{Model: model}
	if cfg != nil {
		req.Temperature = cfg.Temperature
		req.MaxTokens = cfg.MaxTokens
		req.TopP = cfg.TopP
		req.Seed = cfg.Seed
		req.ReasoningEffort = ReasoningEffort(cfg.Think)
	}

	nameMap := make(map[string]string) // sanitized -> original
	origToSanitized := make(map[string]string)
	if cfg != nil && len(cfg.Tools) > 0 {
		seen := map[string]int{}
		tools := make([]Tool, 0, len(cfg.Tools))
		for i, t := range cfg.Tools {
			if strings.ToLower(t.Type) != "function" || t.Function == nil {
				continue
			}
			orig := t.Function.Name
			name := sanitizeToolName(orig)
			if name == "" {
				name = fmt.Sprintf("tool_%d", i)
			}
			name = uniquifyToolName(seen, name)
			nameMap[name] = orig
			origToSanitized[orig] = name
			tools = append(tools, Tool{
				Type: "function",
				Function: ToolDefinition{
					Name:        name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			})
		}
		if len(tools) > 0 {
			req.Tools = tools
		}
	}

	req.Messages = make([]Message, 0, len(messages))
	for _, msg := range messages {
		wm := Message{
			Role:       msg.Role,
			ToolCallID: msg.ToolCallID,
		}
		switch {
		case len(msg.Images) > 0:
			// Image attachments force the content-parts array form.
			wm.Content = wireImageContent(msg)
		case msg.Content == "" && len(msg.ToolCalls) > 0:
			// Assistant messages that carry only tool calls send null content.
			wm.Content = nil
		default:
			wm.Content = msg.Content
		}
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			if san, ok := origToSanitized[name]; ok {
				name = san
			} else {
				name = sanitizeToolName(name)
			}
			wm.ToolCalls = append(wm.ToolCalls, ToolCall{
				ID:       tc.ID,
				Type:     tc.Type,
				Function: ToolFunction{Name: name, Arguments: tc.Function.Arguments},
			})
		}
		req.Messages = append(req.Messages, wm)
	}

	return req, nameMap
}

func wireImageContent(msg modelrepo.Message) []ContentPart {
	parts := make([]ContentPart, 0, len(msg.Images)+1)
	if msg.Content != "" {
		parts = append(parts, ContentPart{Type: "text", Text: msg.Content})
	}
	for _, img := range msg.Images {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: imageDataURI(img.MimeType, img.Data)},
		})
	}
	return parts
}

func imageDataURI(mimeType string, data []byte) string {
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// Response is the non-streaming chat/completions response body.
type Response struct {
	Choices []struct {
		Index        int         `json:"index"`
		Message      responseMsg `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

type wireUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u *wireUsage) neutralUsage() *modelrepo.TokenUsage {
	if u == nil {
		return nil
	}
	total := u.TotalTokens
	if total == 0 {
		total = u.PromptTokens + u.CompletionTokens
	}
	return &modelrepo.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		ThinkingTokens:   u.CompletionTokensDetails.ReasoningTokens,
		TotalTokens:      total,
		CacheReadTokens:  u.PromptTokensDetails.CachedTokens,
	}
}

type responseMsg struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content"`
	ToolCalls        []ToolCall `json:"tool_calls"`
}

// DecodeResponse parses a non-streaming response into a neutral ChatResult,
// translating sanitized tool-call names back via nameMap.
func DecodeResponse(raw []byte, nameMap map[string]string) (modelrepo.ChatResult, error) {
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return modelrepo.ChatResult{}, fmt.Errorf("chatcompletions: decode response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("chatcompletions: no choices in response")
	}
	choice := resp.Choices[0]
	if choice.Message.Content == "" && len(choice.Message.ToolCalls) == 0 && choice.Message.ReasoningContent == "" {
		return modelrepo.ChatResult{}, fmt.Errorf("chatcompletions: empty content (finish_reason=%s)", choice.FinishReason)
	}
	result := modelrepo.ChatResult{
		Message: modelrepo.Message{
			Role:     choice.Message.Role,
			Content:  choice.Message.Content,
			Thinking: choice.Message.ReasoningContent,
		},
		Usage:        resp.Usage.neutralUsage(),
		FinishReason: choice.FinishReason,
	}
	result.ToolCalls = decodeToolCalls(choice.Message.ToolCalls, nameMap)
	return result, nil
}

func decodeToolCalls(in []ToolCall, nameMap map[string]string) []modelrepo.ToolCall {
	var out []modelrepo.ToolCall
	for _, tc := range in {
		name := tc.Function.Name
		if orig, ok := nameMap[name]; ok && orig != "" {
			name = orig
		}
		out = append(out, newToolCall(tc.ID, tc.Type, name, tc.Function.Arguments))
	}
	return out
}

func newToolCall(id, typ, name, args string) modelrepo.ToolCall {
	tc := modelrepo.ToolCall{ID: id, Type: typ}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

type streamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// StreamDecoder translates streamed chat/completions chunks into raw-delta
// parcels. It does not assemble tool calls; that is left to
// modelrepo.StreamAssembler. Finish reason and usage are surfaced by Finish.
type StreamDecoder struct {
	nameMap      map[string]string
	finishReason string
	usage        *modelrepo.TokenUsage
}

// NewStreamDecoder returns a decoder. nameMap is the sanitized->original map
// from Build (may be nil if no tools).
func NewStreamDecoder(nameMap map[string]string) *StreamDecoder {
	return &StreamDecoder{nameMap: nameMap}
}

// DecodeLine parses one SSE data payload (the bytes AFTER the "data: " prefix,
// excluding the "[DONE]" sentinel which the caller should skip) and returns
// the raw-delta parcels it carries, in wire order.
func (d *StreamDecoder) DecodeLine(payload []byte) ([]*modelrepo.StreamParcel, error) {
	var chunk streamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return nil, fmt.Errorf("chatcompletions: decode stream chunk: %w", err)
	}
	if chunk.Usage != nil {
		d.usage = chunk.Usage.neutralUsage()
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}
	choice := chunk.Choices[0]
	if choice.FinishReason != "" {
		d.finishReason = choice.FinishReason
	}

	var parcels []*modelrepo.StreamParcel
	if choice.Delta.Content != "" {
		parcels = append(parcels, &modelrepo.StreamParcel{Data: choice.Delta.Content})
	}
	if thinking := firstNonEmpty(choice.Delta.Reasoning, choice.Delta.ReasoningContent); thinking != "" {
		parcels = append(parcels, &modelrepo.StreamParcel{Thinking: thinking})
	}
	for _, tc := range choice.Delta.ToolCalls {
		name := tc.Function.Name
		if orig, ok := d.nameMap[name]; ok && orig != "" {
			name = orig
		}
		parcels = append(parcels, &modelrepo.StreamParcel{ToolCall: &modelrepo.ToolCallDelta{
			Index:        tc.Index,
			ID:           tc.ID,
			Type:         tc.Type,
			Name:         name,
			ArgsFragment: tc.Function.Arguments,
		}})
	}
	return parcels, nil
}

// Finish returns the typed terminal parcel: the finish reason last seen on the
// wire plus the usage report when the provider sent one. Callers emit it after
// the SSE stream ends cleanly ([DONE] or EOF without error).
func (d *StreamDecoder) Finish() *modelrepo.StreamParcel {
	return &modelrepo.StreamParcel{Terminal: &modelrepo.StreamTerminal{
		FinishReason: d.finishReason,
		Usage:        d.usage,
	}}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ReasoningEffort renders a think setting as the wire's reasoning_effort. Auto
// and an unset setting say nothing, sending an empty string; an invalid level
// is refused upstream rather than silently flipped to a default.
func ReasoningEffort(think *string) string {
	if think == nil {
		return ""
	}
	level, ok, err := reasoning.NormalizeOptional(*think)
	if err != nil || !ok || level == reasoning.Auto {
		return ""
	}
	return level
}

func sanitizeToolName(in string) string {
	if in == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_-")
}

func uniquifyToolName(seen map[string]int, name string) string {
	if _, ok := seen[name]; !ok {
		seen[name] = 1
		return name
	}
	i := seen[name]
	for {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if _, ok := seen[candidate]; !ok {
			seen[name] = i + 1
			seen[candidate] = 1
			return candidate
		}
		i++
	}
}
