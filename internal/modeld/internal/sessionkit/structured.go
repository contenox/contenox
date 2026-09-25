package sessionkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/contenox/contenox/internal/transport"
)

// ParsedToolCall is the permissive wire shape of one model-emitted tool call.
// Models and native parsers disagree on field names (name/tool_name/function,
// arguments/parameters), so it accepts all spellings; TransportToolCalls
// normalizes them onto the backend-neutral transport.ToolCall. Shared by both
// backend adapters so structured tool-call output parses identically
// regardless of which engine constrained the generation.
type ParsedToolCall struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	ToolName   string          `json:"tool_name"`
	Arguments  json.RawMessage `json:"arguments"`
	Parameters json.RawMessage `json:"parameters"`
	Function   struct {
		Name       string          `json:"name"`
		Arguments  json.RawMessage `json:"arguments"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// TransportToolCalls normalizes permissive tool-call shapes onto
// transport.ToolCall, defaulting IDs and the "function" type.
func TransportToolCalls(in []ParsedToolCall) ([]transport.ToolCall, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]transport.ToolCall, 0, len(in))
	for i, tc := range in {
		call := transport.ToolCall{ID: tc.ID, Type: tc.Type}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", i+1)
		}
		if call.Type == "" {
			call.Type = "function"
		}
		call.Function.Name = tc.Function.Name
		if call.Function.Name == "" {
			call.Function.Name = tc.Name
		}
		if call.Function.Name == "" {
			call.Function.Name = tc.ToolName
		}
		rawArgs := tc.Function.Arguments
		if len(rawArgs) == 0 {
			rawArgs = tc.Function.Parameters
		}
		if len(rawArgs) == 0 {
			rawArgs = tc.Arguments
		}
		if len(rawArgs) == 0 {
			rawArgs = tc.Parameters
		}
		args, err := normalizeToolArguments(rawArgs)
		if err != nil {
			return nil, err
		}
		call.Function.Arguments = args
		out = append(out, call)
	}
	return out, nil
}

// StructuredToolCallChunk parses the complete text of a structured tool-call
// generation into a StreamChunk carrying transport tool calls. It accepts the
// shapes constrained decoding and model-native templates produce: a JSON envelope
// ({"content":…,"tool_calls":[…]} or a bare call object), Qwen-style
// <tool_call>…</tool_call> blocks in either dialect — the JSON payload some Qwen
// variants emit and the <function=…>/<parameter=…> XML the Qwen 3.x templates
// document and train the model to write — and MiniCPM5's bare
// <function name="…"><param name="…"> blocks, whose values may be CDATA-wrapped.
func StructuredToolCallChunk(text string) (transport.StreamChunk, error) {
	raw := bytes.TrimSpace([]byte(text))
	if len(raw) == 0 {
		return transport.StreamChunk{}, fmt.Errorf("structured tool call output is empty")
	}
	if raw[0] != '{' {
		return chunkFromTagDialects(text)
	}

	var envelope struct {
		Content   *string          `json:"content"`
		ToolCalls []ParsedToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transport.StreamChunk{}, fmt.Errorf("parse structured tool call envelope: %w", err)
	}
	chunk := transport.StreamChunk{}
	if envelope.Content != nil {
		chunk.Text = *envelope.Content
	}
	if len(envelope.ToolCalls) == 0 {
		// Bare objects without the envelope ({"function":…}) are deliberately
		// rejected: constrained schemas emit {"content":…} or {"tool_calls":…},
		// and silently accepting legacy shapes would mask schema drift.
		if envelope.Content == nil {
			return transport.StreamChunk{}, fmt.Errorf("structured tool call envelope contained neither content nor tool_calls")
		}
		return chunk, nil
	}
	calls, err := TransportToolCalls(envelope.ToolCalls)
	if err != nil {
		return transport.StreamChunk{}, err
	}
	chunk.ToolCalls = calls
	return chunk, nil
}

var (
	qwenToolCallBlockRE = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)
	qwenFunctionRE      = regexp.MustCompile(`(?s)<function=([^\s>]+)\s*>`)
	qwenParameterRE     = regexp.MustCompile(`(?s)<parameter=([^\s>]+)\s*>(.*?)</parameter>`)
	minicpmFunctionRE   = regexp.MustCompile(`(?s)<function\s+name="([^"]+)"\s*>(.*?)</function>`)
	minicpmParameterRE  = regexp.MustCompile(`(?s)<param\s+name="([^"]+)"\s*>(.*?)</param>`)
)

// ToolCallMarker opens the tool-call dialect a model-native template documents.
// Both model adapters read it, so it is defined once: a marker one adapter
// recognises and the other does not is a call that leaks as prose.
const ToolCallMarker = "<tool_call>"

// ToolCallAltMarker opens MiniCPM5's dialect, where a call is a bare
// <function name="…"> block rather than a <tool_call> payload. It is held and
// rescued alongside the primary marker, so output naming a tool the roster does
// not carry reads as a call that failed rather than as prose that ran nothing.
const ToolCallAltMarker = `<function name="`

// ToolCallMarkers are every opener a call can begin with. A streaming reader must
// hold from the first byte of any of them.
var ToolCallMarkers = []string{ToolCallMarker, ToolCallAltMarker}

// minMarkerHold is the shortest suffix of each marker a reader treats as the
// possible start of a call. <tool_call> is distinctive from its first byte;
// <function name=" is not — holding at "<f" would eat ordinary prose — so the
// attribute dialect is only held from its full keyword.
func minMarkerHold(marker string) int {
	if marker == ToolCallAltMarker {
		return len("<function")
	}
	return 1
}

// HasToolCallMarker reports whether text shows a tool-call attempt: a call block
// in either dialect, or an opening marker whose payload never arrived. A parser
// that produced no call from such output has either swallowed it as prose or lost
// it to a truncated generation, and the caller must not deliver it as an answer
// either way.
func HasToolCallMarker(text string) bool {
	for _, marker := range ToolCallMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// ToolCallMarkerPending reports whether text holds a tool-call marker or the start of
// one, including a marker split across two chunks. A streaming reader must hold back
// from the first byte this reports rather than deliver it: by the time the marker is
// complete the bytes are already gone, and a call delivered as prose is a call nobody
// runs.
func ToolCallMarkerPending(text string) bool {
	if HasToolCallMarker(text) {
		return true
	}
	return pendingMarkerLen(text) > 0
}

// ToolCallClosers are the dialect's closing tags. A turn that opens nothing but
// closes one is emitting the tail of a call it never opened — the tag is markup
// either way, and delivering it puts raw dialect in front of the reader.
var ToolCallClosers = []string{"</tool_call>", "</function>", "</parameter>"}

// MarkupGuard drops tool-call markup from the text and thinking deltas of one
// decode and remembers whether it ever had to. It is the adapter's last line of
// defence: a parser can be switched off, or a grammar can bypass it, but dialect
// markup must never reach a client as prose — and an attempt that produced no
// call is a failed turn, not an answer, whichever path let it through.
type MarkupGuard struct {
	withheld bool
	calls    bool
}

// Filter returns the two deltas with any marker from the first marker on
// removed, and records the call when one was delivered.
func (g *MarkupGuard) Filter(text, thinking string, toolCalls int) (string, string) {
	if toolCalls > 0 {
		g.calls = true
	}
	text, textCut := withholdFromMarker(text)
	thinking, thinkingCut := withholdFromMarker(thinking)
	g.withheld = g.withheld || textCut || thinkingCut
	return text, thinking
}

// AttemptWithoutCall reports a tool-call attempt that no call accompanied, which
// the caller must fail the turn with rather than deliver as an answer.
func (g *MarkupGuard) AttemptWithoutCall() bool { return g.withheld && !g.calls }

func withholdFromMarker(text string) (string, bool) {
	cut := len(text)
	found := false
	for _, marker := range ToolCallMarkers {
		if idx := strings.Index(text, marker); idx >= 0 && idx < cut {
			cut, found = idx, true
		}
	}
	if found {
		return text[:cut], true
	}
	if hold := pendingMarkerLen(text); hold > 0 {
		return text[:len(text)-hold], true
	}
	return text, false
}

func pendingMarkerLen(text string) int {
	longest := 0
	for _, marker := range ToolCallMarkers {
		for i := len(marker) - 1; i >= minMarkerHold(marker); i-- {
			if strings.HasSuffix(text, marker[:i]) {
				if i > longest {
					longest = i
				}
				break
			}
		}
	}
	return longest
}

// StripToolCallClosers removes closing tags of the tool-call dialect from text.
// Only closers are stripped: an opening marker is an attempt, which the caller
// either rescues as a call or reports as unreadable, never silently edits away.
// The whitespace a removed tag sat between is left as it was.
func StripToolCallClosers(text string) string {
	if !strings.Contains(text, "</") {
		return text
	}
	for _, closer := range ToolCallClosers {
		text = strings.ReplaceAll(text, closer, "")
	}
	return text
}

func chunkFromTagDialects(text string) (transport.StreamChunk, error) {
	type callSpan struct {
		start, end int
		call       ParsedToolCall
	}
	var spans []callSpan

	blocks := qwenToolCallBlockRE.FindAllStringSubmatchIndex(text, -1)
	for _, match := range blocks {
		call, ok, err := parseQwenToolCallBody(text[match[2]:match[3]])
		if err != nil {
			return transport.StreamChunk{}, err
		}
		if !ok {
			// A block in neither dialect stays content: an unrecognized payload
			// is a worse reason to lose the turn than to lose the call.
			continue
		}
		spans = append(spans, callSpan{start: match[0], end: match[1], call: call})
	}

	for _, match := range minicpmFunctionRE.FindAllStringSubmatchIndex(text, -1) {
		if insideSpan(blocks, match[0]) {
			continue
		}
		call, err := parseMiniCPM5Call(text[match[4]:match[5]], text[match[2]:match[3]])
		if err != nil {
			return transport.StreamChunk{}, err
		}
		spans = append(spans, callSpan{start: match[0], end: match[1], call: call})
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var remaining strings.Builder
	toolCalls := make([]ParsedToolCall, 0, len(spans))
	last := 0
	for _, span := range spans {
		remaining.WriteString(text[last:span.start])
		last = span.end
		span.call.ID = fmt.Sprintf("call_%d", len(toolCalls)+1)
		span.call.Type = "function"
		toolCalls = append(toolCalls, span.call)
	}
	remaining.WriteString(text[last:])

	calls, err := TransportToolCalls(toolCalls)
	if err != nil {
		return transport.StreamChunk{}, err
	}
	return transport.StreamChunk{
		Text:      strings.TrimSpace(remaining.String()),
		ToolCalls: calls,
	}, nil
}

// insideSpan reports whether offset falls inside one of the <tool_call> blocks,
// whose payload is read whole: a bare function block can only appear there as the
// text of an argument, never as a second call.
func insideSpan(spans [][]int, offset int) bool {
	for _, span := range spans {
		if offset >= span[0] && offset < span[1] {
			return true
		}
	}
	return false
}

// Every parameter is text in this dialect, so arguments are encoded as JSON
// strings and the tool layer's own argument coercion types them.
func parseMiniCPM5Call(body, name string) (ParsedToolCall, error) {
	parameters := map[string]string{}
	for _, parameter := range minicpmParameterRE.FindAllStringSubmatch(body, -1) {
		parameters[parameter[1]] = minicpmParameterValue(parameter[2])
	}
	encoded, err := encodeToolArguments(parameters)
	if err != nil {
		return ParsedToolCall{}, fmt.Errorf("encode minicpm5 tool_call parameters: %w", err)
	}
	return ParsedToolCall{Name: name, Arguments: encoded}, nil
}

// minicpmParameterValue unwraps the CDATA block the dialect uses for a value that
// carries markup — the template's own escape for a command or a file body — and
// otherwise trims the template's delimiter newlines like the Qwen dialect does.
func minicpmParameterValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "<![CDATA[") && strings.HasSuffix(trimmed, "]]>") {
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, "<![CDATA["), "]]>")
	}
	return trimParameterValue(value)
}

// parseQwenToolCallBody reads one <tool_call> payload. ok is false when the body is
// in neither dialect, which the caller treats as content.
func parseQwenToolCallBody(body string) (ParsedToolCall, bool, error) {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") {
		var parsed struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return ParsedToolCall{}, false, fmt.Errorf("parse qwen tool_call payload: %w", err)
		}
		if parsed.Name == "" {
			return ParsedToolCall{}, false, fmt.Errorf("qwen tool_call payload missing name")
		}
		return ParsedToolCall{Name: parsed.Name, Arguments: parsed.Arguments}, true, nil
	}

	function := qwenFunctionRE.FindStringSubmatch(trimmed)
	if function == nil {
		return ParsedToolCall{}, false, nil
	}
	parameters := map[string]string{}
	for _, parameter := range qwenParameterRE.FindAllStringSubmatch(trimmed, -1) {
		parameters[parameter[1]] = trimParameterValue(parameter[2])
	}
	encoded, err := encodeToolArguments(parameters)
	if err != nil {
		return ParsedToolCall{}, false, fmt.Errorf("encode qwen tool_call parameters: %w", err)
	}
	return ParsedToolCall{Name: function[1], Arguments: encoded}, true, nil
}

// encodeToolArguments renders a call's text parameters as a JSON object. HTML
// escaping stays off: these arguments are shown to whoever approves the call, and
// an escaped "&&" in a shell command is a command the operator cannot read.
func encodeToolArguments(parameters map[string]string) (json.RawMessage, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(parameters); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// trimParameterValue removes the single newline the template puts around a
// parameter value — models usually reproduce it — and nothing else: indentation
// inside a value is content. Values stay JSON strings, matching the dialect's
// text semantics and the tool layer's own argument coercion.
func trimParameterValue(value string) string {
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	value = strings.TrimPrefix(value, "\r")
	value = strings.TrimPrefix(value, "\n")
	return value
}

func normalizeToolArguments(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "{}", nil
	}
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("parse tool arguments string: %w", err)
		}
		if strings.TrimSpace(s) == "" {
			return "{}", nil
		}
		return s, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", fmt.Errorf("compact tool arguments: %w", err)
	}
	return compact.String(), nil
}
