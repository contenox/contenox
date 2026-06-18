// Package toolcalls is the shared, backend-neutral machinery for model-native
// tool calls: serializing tool definitions for the model's own chat template,
// and parsing the model's raw output back into structured tool calls per a
// profile-declared protocol. The protocol is the format the model's OWN chat
// template emits (model-declared), not a Contenox-invented schema — so the same
// model behaves identically whether served by the llama or OpenVINO backend.
package toolcalls

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// ErrUnknownProtocol is returned for a declared protocol with no registered parser.
var ErrUnknownProtocol = errors.New("toolcalls: unknown tool-call protocol")

// Parser extracts neutral tool calls from a model's raw output, returning the
// remaining visible text as content.
type Parser func(text string) (calls []modelrepo.ToolCall, content string, err error)

// protocols maps a profile-declared protocol name to its parser. hermes and qwen
// emit identical <tool_call>{...}</tool_call> blocks.
var protocols = map[string]Parser{
	"hermes": ParseHermes,
	"qwen":   ParseHermes,
}

// ProtocolKnown reports whether a protocol name has a registered parser (used by
// profile validation to reject typos at load time).
func ProtocolKnown(protocol string) bool {
	_, ok := protocols[protocol]
	return ok
}

// ParserFor returns the parser for a declared protocol. A blank protocol returns
// (nil, nil): the caller must treat that as "tools not supported for this model".
func ParserFor(protocol string) (Parser, error) {
	if protocol == "" {
		return nil, nil
	}
	p, ok := protocols[protocol]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProtocol, protocol)
	}
	return p, nil
}

// SerializeToolDefs marshals neutral tool definitions to the JSON array the
// model's own chat template consumes. The daemon renders them model-natively; the
// runtime never encodes the model's tool format.
func SerializeToolDefs(tools []modelrepo.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("toolcalls: serialize tool definitions: %w", err)
	}
	return string(b), nil
}

const (
	toolCallOpen  = "<tool_call>"
	toolCallClose = "</tool_call>"
)

// ParseHermes extracts <tool_call>{...}</tool_call> blocks (the Hermes/Qwen
// model-declared format) into neutral tool calls, returning the remaining visible
// text as content.
func ParseHermes(text string) ([]modelrepo.ToolCall, string, error) {
	var calls []modelrepo.ToolCall
	var content strings.Builder
	rest := text
	for {
		open := strings.Index(rest, toolCallOpen)
		if open < 0 {
			content.WriteString(rest)
			break
		}
		content.WriteString(rest[:open])
		after := rest[open+len(toolCallOpen):]
		end := strings.Index(after, toolCallClose)
		if end < 0 {
			// Unterminated block (e.g. truncated output): keep as content.
			content.WriteString(rest[open:])
			break
		}
		body := strings.TrimSpace(after[:end])
		rest = after[end+len(toolCallClose):]

		var parsed struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(body), &parsed); err != nil {
			return nil, "", fmt.Errorf("toolcalls: parse tool call %q: %w", body, err)
		}
		args := strings.TrimSpace(string(parsed.Arguments))
		if args == "" || args == "null" {
			args = "{}"
		}
		tc := modelrepo.ToolCall{Type: "function"}
		tc.Function.Name = parsed.Name
		tc.Function.Arguments = args
		calls = append(calls, tc)
	}
	return calls, strings.TrimSpace(content.String()), nil
}
