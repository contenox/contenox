package llama

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// serializeToolDefs marshals neutral tool definitions to the JSON array the
// model's own chat template consumes. The daemon renders them model-natively via
// minja (the GGUF Jinja); the runtime never encodes the model's tool format.
func serializeToolDefs(tools []modelrepo.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("llama: serialize tool definitions: %w", err)
	}
	return string(b), nil
}

// toolCallParser extracts neutral tool calls from a model's raw output for a
// profile-declared protocol. There is no fallback: an unknown protocol is an
// error, and output from a model with no declared protocol is never guessed at.
type toolCallParser func(text string) (calls []modelrepo.ToolCall, content string, err error)

// toolCallProtocols maps a profile-declared protocol name to its parser. The
// protocol is the format the model's own chat template declares it will emit (so
// this is model-declared, not a Contenox-invented schema).
var toolCallProtocols = map[string]toolCallParser{
	// hermes/qwen emit <tool_call>{"name":..,"arguments":..}</tool_call> blocks,
	// exactly as the model's GGUF chat template instructs.
	"hermes": parseHermesToolCalls,
	"qwen":   parseHermesToolCalls,
}

// toolCallProtocolKnown reports whether a protocol name is registered (used by
// profile validation to reject typos at load time).
func toolCallProtocolKnown(protocol string) bool {
	_, ok := toolCallProtocols[protocol]
	return ok
}

// toolCallParserFor returns the parser for a declared protocol. A blank protocol
// returns (nil, nil): the caller must treat that as "tools not supported".
func toolCallParserFor(protocol string) (toolCallParser, error) {
	if protocol == "" {
		return nil, nil
	}
	p, ok := toolCallProtocols[protocol]
	if !ok {
		return nil, NewUnsupportedFeatureError("tool-call protocol " + protocol)
	}
	return p, nil
}

const (
	toolCallOpen  = "<tool_call>"
	toolCallClose = "</tool_call>"
)

// parseHermesToolCalls extracts <tool_call>{...}</tool_call> blocks (the Hermes/
// Qwen model-declared format) into neutral tool calls, returning the remaining
// visible text as content.
func parseHermesToolCalls(text string) ([]modelrepo.ToolCall, string, error) {
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
			return nil, "", fmt.Errorf("llama: parse tool call %q: %w", body, err)
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
