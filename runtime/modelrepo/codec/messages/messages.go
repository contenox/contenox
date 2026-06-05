// Package messages is a transport-agnostic codec for Anthropic's Messages API
// wire format (request, content-block response, and named-SSE-event streaming).
// It maps between contenox's neutral modelrepo types and Anthropic's JSON shape.
//
// It does NO I/O. The transport (api.anthropic.com) supplies the envelope:
// model in the body, version via the `anthropic-version` header, auth via
// `x-api-key`. This lets the direct Anthropic provider stay a thin transport
// wrapper around the shared codec.
package messages

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/agent/runtime/modelrepo"
)

// DefaultMaxTokens is used when the caller does not set ChatConfig.MaxTokens.
// Anthropic requires max_tokens; it has no "unlimited" sentinel.
const DefaultMaxTokens = 4096

// Request is the Anthropic Messages request body.
type Request struct {
	// Model is omitted for Vertex (model lives in the URL) and set for direct.
	Model string `json:"model,omitempty"`
	// AnthropicVersion is set by the Vertex transport ("vertex-2023-10-16");
	// empty for direct (sent as a header instead).
	AnthropicVersion string          `json:"anthropic_version,omitempty"`
	MaxTokens        int             `json:"max_tokens"`
	System           string          `json:"system,omitempty"`
	Messages         []wireMessage   `json:"messages"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	Tools            []wireTool      `json:"tools,omitempty"`
	Thinking         *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig     *OutputConfig   `json:"output_config,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

type OutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

// wireBlock is one content block. Only the fields relevant to its Type are set.
type wireBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type wireTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// Build converts neutral messages + config into an Anthropic Messages Request.
// The transport must still set Model and/or AnthropicVersion per hosting mode.
func Build(messages []modelrepo.Message, cfg *modelrepo.ChatConfig) Request {
	req := Request{MaxTokens: DefaultMaxTokens}
	if cfg != nil {
		if cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
			req.MaxTokens = *cfg.MaxTokens
		}
		req.Temperature = cfg.Temperature
		req.TopP = cfg.TopP
		for _, t := range cfg.Tools {
			if strings.ToLower(t.Type) != "function" || t.Function == nil {
				continue
			}
			req.Tools = append(req.Tools, wireTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: t.Function.Parameters,
			})
		}
	}

	var systemParts []string
	for _, m := range messages {
		switch m.Role {
		case "system":
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "tool":
			// A tool result becomes a user message carrying a tool_result block.
			req.Messages = append(req.Messages, wireMessage{
				Role: "user",
				Content: []wireBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		case "assistant", "model":
			var blocks []wireBlock
			if m.Content != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(strings.TrimSpace(tc.Function.Arguments)) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, wireBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			if len(blocks) == 0 {
				continue
			}
			req.Messages = append(req.Messages, wireMessage{Role: "assistant", Content: blocks})
		default: // "user" and anything else
			if m.Content == "" {
				continue
			}
			req.Messages = append(req.Messages, wireMessage{
				Role:    "user",
				Content: []wireBlock{{Type: "text", Text: m.Content}},
			})
		}
	}
	if len(systemParts) > 0 {
		req.System = strings.Join(systemParts, "\n\n")
	}
	return req
}

// Response is the non-streaming Anthropic Messages response body.
type Response struct {
	Role       string          `json:"role"`
	Content    []responseBlock `json:"content"`
	StopReason string          `json:"stop_reason"`
}

type responseBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
}

// DecodeResponse parses a non-streaming response into a neutral ChatResult.
func DecodeResponse(raw []byte) (modelrepo.ChatResult, error) {
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return modelrepo.ChatResult{}, fmt.Errorf("messages: decode response: %w", err)
	}
	if len(resp.Content) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("messages: empty content (stop_reason=%s)", resp.StopReason)
	}
	var text, thinking strings.Builder
	var toolCalls []modelrepo.ToolCall
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "thinking":
			thinking.WriteString(b.Thinking)
		case "tool_use":
			args := string(b.Input)
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, newToolCall(b.ID, b.Name, args))
		}
	}
	role := resp.Role
	if role == "" {
		role = "assistant"
	}
	return modelrepo.ChatResult{
		Message: modelrepo.Message{
			Role:     role,
			Content:  text.String(),
			Thinking: thinking.String(),
		},
		ToolCalls: toolCalls,
	}, nil
}

// newToolCall builds a neutral ToolCall (Function is an anonymous struct).
func newToolCall(id, name, args string) modelrepo.ToolCall {
	tc := modelrepo.ToolCall{ID: id, Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

// streamEvent is the JSON `data:` payload of any Messages SSE event; the `type`
// field discriminates. (The `event:` line is redundant and can be ignored.)
type streamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

// StreamDecoder assembles a streamed Messages response. Text and thinking
// deltas are emitted as parcels; tool_use blocks are accumulated per index
// (id/name from content_block_start, arguments from input_json_delta) and
// exposed via ToolCalls() once the stream ends.
type StreamDecoder struct {
	toolAcc  map[int]*accTool
	maxIndex int
}

type accTool struct {
	id   string
	name string
	args strings.Builder
}

func NewStreamDecoder() *StreamDecoder {
	return &StreamDecoder{toolAcc: map[int]*accTool{}, maxIndex: -1}
}

// DecodeLine parses one SSE `data:` payload (bytes after "data: "). It returns
// a parcel if the event carried visible text/thinking, or nil otherwise.
func (d *StreamDecoder) DecodeLine(payload []byte) (*modelrepo.StreamParcel, error) {
	var ev streamEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, fmt.Errorf("messages: decode stream event: %w", err)
	}
	switch ev.Type {
	case "content_block_start":
		if ev.ContentBlock.Type == "tool_use" {
			acc := &accTool{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			d.toolAcc[ev.Index] = acc
			if ev.Index > d.maxIndex {
				d.maxIndex = ev.Index
			}
		}
		return nil, nil
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text != "" {
				return &modelrepo.StreamParcel{Data: ev.Delta.Text}, nil
			}
		case "thinking_delta":
			if ev.Delta.Thinking != "" {
				return &modelrepo.StreamParcel{Thinking: ev.Delta.Thinking}, nil
			}
		case "input_json_delta":
			if acc := d.toolAcc[ev.Index]; acc != nil {
				acc.args.WriteString(ev.Delta.PartialJSON)
			}
		}
		return nil, nil
	default:
		// message_start, content_block_stop, message_delta, message_stop, ping
		return nil, nil
	}
}

// ToolCalls returns the tool calls assembled across the stream, in index order.
func (d *StreamDecoder) ToolCalls() []modelrepo.ToolCall {
	if d.maxIndex < 0 {
		return nil
	}
	var out []modelrepo.ToolCall
	for i := 0; i <= d.maxIndex; i++ {
		acc := d.toolAcc[i]
		if acc == nil {
			continue
		}
		args := acc.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out = append(out, newToolCall(acc.id, acc.name, args))
	}
	return out
}
