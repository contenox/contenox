package openvino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/transport"
)

// genaiBackend is the subset of *ovsession.GenAISession the transport-session
// adapter depends on. Narrowing it to an interface keeps the warm-reuse mapping
// testable without compiling the CGO OpenVINO GenAI backend.
type genaiBackend interface {
	Generate(ctx context.Context, prompt string, opts ovsession.GenerateOptions) (ovsession.GenAIResult, error)
	Stream(ctx context.Context, prompt string, opts ovsession.GenerateOptions) (<-chan ovsession.StreamChunk, error)
	Tokenize(ctx context.Context, prompt string, addSpecial bool) ([]int, error)
	// ApplyChatTemplate renders role/content turns with the MODEL's own chat
	// template (held inside the IR tokenizer), producing the prompt string the
	// pipeline expects. This is why generation runs with apply_chat_template=false
	// in the shim: the caller templates first, with the model-native template.
	ApplyChatTemplate(messages []ovsession.ChatMessage, toolsJSON string) (string, error)
	Close() error
}

// The native OpenVINO GenAI session is the production backend; the assertion
// holds in every build because the no-CGO stub mirrors its method set.
var _ genaiBackend = (*ovsession.GenAISession)(nil)

type EmbedSessionBackend interface {
	Embed(ctx context.Context, prompt string) ([]float32, error)
	Close() error
}

var _ EmbedSessionBackend = (*ovsession.EmbedSession)(nil)

// genaiSession adapts OpenVINO GenAI to the runtime's transport.Session contract.
//
// OpenVINO GenAI is string-prompt based: its ContinuousBatchingPipeline holds
// the tokenizer and applies prefix caching INTERNALLY, keyed on the prompt
// string. This adapter does not manipulate KV the way the llama backend does.
// EnsurePrefix and PrefillSuffix record the stable prefix and volatile suffix
// text and gate reuse on the manifest; Decode concatenates them into one prompt
// and streams. The manifest is the correctness key: an incompatible
// profile/template/runtime drops the recorded prefix so stale warm state is not
// reused across a runtime change.
type genaiSession struct {
	backend genaiBackend
	numCtx  int

	mu           sync.Mutex
	closed       bool
	manifest     transport.ContextManifest
	stable       string
	suffix       string
	tools        string // model-native tool definitions JSON, rendered via the chat template
	stableTokens int
	suffixTokens int
}

func newGenaiSession(backend genaiBackend, numCtx int) *genaiSession {
	return &genaiSession{backend: backend, numCtx: numCtx}
}

var newEmbedSession = func(modelPath, device string) (EmbedSessionBackend, error) {
	return ovsession.NewEmbed(modelPath, device)
}

var _ transport.Session = (*genaiSession)(nil)

func (s *genaiSession) resident() int { return s.stableTokens + s.suffixTokens }

func (s *genaiSession) available() int {
	if s.numCtx <= 0 {
		return 0
	}
	return s.numCtx - s.resident()
}

// tokenCount tokenizes best-effort: the count is for the status report only, so
// a backend tokenize error degrades to zero rather than failing the operation.
func (s *genaiSession) tokenCount(ctx context.Context, text string, addSpecial bool) int {
	if text == "" {
		return 0
	}
	toks, err := s.backend.Tokenize(ctx, text, addSpecial)
	if err != nil {
		return 0
	}
	return len(toks)
}

func (s *genaiSession) EnsurePrefix(ctx context.Context, prefix transport.PrefixInput) (transport.PrefixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return transport.PrefixStatus{}, transport.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return transport.PrefixStatus{}, err
	}

	digest := prefix.Manifest.Digest()
	stableHash := prefix.Manifest.StableByteHash
	oldResident := s.resident()

	// Warm only when the resident prefix came from a compatible runtime identity
	// AND the same stable bytes. CompatibleRuntime deliberately ignores the stable
	// hash (token LCP stays correct under stable-text edits), so the stable hash
	// is compared explicitly here: the pipeline must not reuse a string prefix
	// across a profile/template/runtime change.
	compatible, _ := s.manifest.CompatibleRuntime(prefix.Manifest)
	warm := compatible && s.stable != "" && stableHash != "" && stableHash == s.manifest.StableByteHash

	tokens := s.tokenCount(ctx, prefix.Text, prefix.Manifest.AddBOS)
	if s.numCtx > 0 && tokens > s.numCtx {
		return transport.PrefixStatus{}, transport.ErrContextOverflow
	}

	reused, prefilled, dropped := 0, tokens, 0
	if warm {
		reused, prefilled = tokens, 0
	} else {
		dropped = oldResident
	}

	// EnsurePrefix replaces the stable prefix and drops any prior suffix. The tool
	// definitions ride on the prefix so Decode renders them via the model's own
	// chat template (model-native tool calls).
	s.stable = prefix.Text
	s.suffix = ""
	s.suffixTokens = 0
	s.stableTokens = tokens
	s.manifest = prefix.Manifest
	s.tools = prefix.Tools

	return transport.PrefixStatus{
		ReusedTokens:    reused,
		PrefilledTokens: prefilled,
		DroppedTokens:   dropped,
		PrefixTokens:    s.stableTokens,
		ResidentTokens:  s.resident(),
		AvailableTokens: s.available(),
		StableByteHash:  stableHash,
		StableTokenHash: prefix.Manifest.StableTokenHash,
		ManifestDigest:  digest,
	}, nil
}

func (s *genaiSession) PrefillSuffix(ctx context.Context, suffix transport.SuffixInput) (transport.SuffixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return transport.SuffixStatus{}, transport.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return transport.SuffixStatus{}, err
	}
	if ok, reason := s.manifest.CompatibleRuntime(suffix.Manifest); !ok {
		return transport.SuffixStatus{}, contextasm.NewManifestMismatchError(reason)
	}
	if !s.manifest.IsZero() && !suffix.Manifest.IsZero() && s.manifest.StableByteHash != suffix.Manifest.StableByteHash {
		return transport.SuffixStatus{}, contextasm.NewManifestMismatchError("stable prefix changed between EnsurePrefix and PrefillSuffix")
	}

	add := s.tokenCount(ctx, suffix.Text, false)
	if s.numCtx > 0 && s.resident()+add > s.numCtx {
		return transport.SuffixStatus{}, transport.ErrContextOverflow
	}
	s.suffix += suffix.Text
	s.suffixTokens += add

	return transport.SuffixStatus{
		SuffixTokens:    s.suffixTokens,
		PrefixTokens:    s.stableTokens,
		ResidentTokens:  s.resident(),
		AvailableTokens: s.available(),
		ManifestDigest:  suffix.Manifest.Digest(),
	}, nil
}

func (s *genaiSession) Decode(ctx context.Context, cfg transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, transport.ErrSessionClosed
	}
	fullText := s.stable + s.suffix
	manifest := s.manifest
	backend := s.backend
	tools := s.tools
	s.mu.Unlock()

	// Apply the model's own chat template when the manifest carries the role
	// structure, so the model sees its native format (including tool definitions
	// and tool-call history) and emits a clean EOS the pipeline stops on. Fall back
	// to the raw text when there are no role segments (e.g. direct callers without
	// an assembled manifest).
	prompt := fullText
	if msgs := chatMessagesFromManifest(fullText, manifest); len(msgs) > 0 {
		if templated, err := backend.ApplyChatTemplate(msgs, tools); err == nil && strings.TrimSpace(templated) != "" {
			prompt = templated
		}
	}

	opts := decodeOptions(cfg)
	out := make(chan transport.StreamChunk, 16)
	if cfg.StructuredOutput.Protocol != "" || usesCompleteParser(opts.ParserProtocols) {
		go func() {
			defer close(out)
			res, err := backend.Generate(ctx, prompt, opts)
			if err != nil {
				out <- transport.StreamChunk{Error: err}
				return
			}
			chunk, err := chunkFromGenAIResult(res, cfg.StructuredOutput)
			if err != nil {
				out <- transport.StreamChunk{Error: err}
				return
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				out <- transport.StreamChunk{Error: ctx.Err()}
			}
		}()
		return out, nil
	}

	src, err := backend.Stream(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(out)
		for chunk := range src {
			select {
			case out <- transport.StreamChunk{Text: chunk.Text, Thinking: chunk.Thinking, Error: chunk.Error}:
			case <-ctx.Done():
				out <- transport.StreamChunk{Error: ctx.Err()}
				return
			}
		}
	}()
	return out, nil
}

func usesCompleteParser(protocols []string) bool {
	for _, protocol := range protocols {
		switch protocol {
		case "openvino:llama3_pythonic_tool_parser",
			"openvino:llama3_json_tool_parser",
			"openvino:reasoning_parser",
			"openvino:deepseek_r1_reasoning_parser",
			"openvino:phi4_reasoning_parser":
			return true
		}
	}
	return false
}

func chunkFromGenAIResult(res ovsession.GenAIResult, structured transport.StructuredOutputConfig) (transport.StreamChunk, error) {
	chunk := transport.StreamChunk{Text: res.Text}
	if strings.TrimSpace(res.ParsedJSON) == "" {
		switch structured.Protocol {
		case "":
			return chunk, nil
		case "openvino:json_schema_tool_calls":
			return chunkFromStructuredToolJSON(res.Text)
		default:
			return transport.StreamChunk{}, fmt.Errorf("openvino: unsupported structured output result protocol %q", structured.Protocol)
		}
	}

	var parsed struct {
		Content         *string          `json:"content"`
		Reasoning       *string          `json:"reasoning_content"`
		ToolCalls       []parsedToolCall `json:"tool_calls"`
		OpenAIToolCalls []parsedToolCall `json:"toolCalls"`
	}
	if err := json.Unmarshal([]byte(res.ParsedJSON), &parsed); err != nil {
		return transport.StreamChunk{}, fmt.Errorf("openvino: parse GenAI parsed output: %w", err)
	}
	if parsed.Content != nil {
		chunk.Text = *parsed.Content
	}
	if parsed.Reasoning != nil {
		chunk.Thinking = *parsed.Reasoning
	}
	toolCalls := parsed.ToolCalls
	if len(toolCalls) == 0 {
		toolCalls = parsed.OpenAIToolCalls
	}
	calls, err := transportToolCalls(toolCalls)
	if err != nil {
		return transport.StreamChunk{}, err
	}
	chunk.ToolCalls = calls
	return chunk, nil
}

type parsedToolCall struct {
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

func transportToolCalls(in []parsedToolCall) ([]transport.ToolCall, error) {
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

func chunkFromStructuredToolJSON(text string) (transport.StreamChunk, error) {
	raw := bytes.TrimSpace([]byte(text))
	if len(raw) == 0 {
		return transport.StreamChunk{}, fmt.Errorf("openvino: structured tool call output is empty")
	}

	var envelope struct {
		Content   *string          `json:"content"`
		ToolCalls []parsedToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transport.StreamChunk{}, fmt.Errorf("openvino: parse structured tool call envelope: %w", err)
	}
	if len(envelope.ToolCalls) == 0 {
		return transport.StreamChunk{}, fmt.Errorf("openvino: structured tool call envelope contained no tool_calls")
	}
	calls, err := transportToolCalls(envelope.ToolCalls)
	if err != nil {
		return transport.StreamChunk{}, err
	}
	chunk := transport.StreamChunk{ToolCalls: calls}
	if envelope.Content != nil {
		chunk.Text = *envelope.Content
	}
	return chunk, nil
}

func normalizeToolArguments(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "{}", nil
	}
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("openvino: parse tool arguments string: %w", err)
		}
		if strings.TrimSpace(s) == "" {
			return "{}", nil
		}
		return s, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", fmt.Errorf("openvino: compact tool arguments: %w", err)
	}
	return compact.String(), nil
}

func (s *genaiSession) ExplainContext() transport.ContextReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return transport.ContextReport{
		ResidentTokens:  s.resident(),
		PrefixTokens:    s.stableTokens,
		NumCtx:          s.numCtx,
		AvailableTokens: s.available(),
		StableByteHash:  s.manifest.StableByteHash,
		StableTokenHash: s.manifest.StableTokenHash,
		ManifestDigest:  s.manifest.Digest(),
		Manifest:        s.manifest,
		Closed:          s.closed,
	}
}

func (s *genaiSession) Snapshot(ctx context.Context) (transport.SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return transport.SessionSnapshot{}, transport.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return transport.SessionSnapshot{}, err
	}
	return transport.SessionSnapshot{
		ResidentTokens: s.resident(),
		PrefixTokens:   s.stableTokens,
		NumCtx:         s.numCtx,
		StableText:     s.stable,
		PrefixText:     s.stable + s.suffix,
		Tools:          s.tools,
		Manifest:       s.manifest,
	}, nil
}

func (s *genaiSession) Restore(ctx context.Context, snap transport.SessionSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return transport.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap.NumCtx > 0 && s.numCtx > 0 && snap.NumCtx != s.numCtx {
		return contextasm.NewManifestMismatchError("snapshot context window changed")
	}
	if snap.ResidentTokens < 0 || snap.PrefixTokens < 0 || snap.PrefixTokens > snap.ResidentTokens {
		return transport.ErrContextOverflow
	}
	if s.numCtx > 0 && snap.ResidentTokens > s.numCtx {
		return transport.ErrContextOverflow
	}
	if !s.manifest.IsZero() && !snap.Manifest.IsZero() {
		if ok, reason := s.manifest.CompatibleRuntime(snap.Manifest); !ok {
			return contextasm.NewManifestMismatchError(reason)
		}
	}
	prefixText := snap.PrefixText
	if prefixText == "" {
		prefixText = snap.StableText
	}
	if snap.StableText != "" && !strings.HasPrefix(prefixText, snap.StableText) {
		return contextasm.NewManifestMismatchError("snapshot prefix text does not contain stable text")
	}
	s.stable = snap.StableText
	s.suffix = strings.TrimPrefix(prefixText, snap.StableText)
	s.stableTokens = snap.PrefixTokens
	s.suffixTokens = snap.ResidentTokens - snap.PrefixTokens
	if s.suffixTokens < 0 {
		s.suffixTokens = 0
	}
	s.tools = snap.Tools
	s.manifest = snap.Manifest
	return nil
}

func (s *genaiSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.backend != nil {
		return s.backend.Close()
	}
	return nil
}

// chatMessagesFromManifest reconstructs the role/content turns from the
// assembled manifest's segments, so the adapter applies the model's own chat
// template instead of a generic render. Each non-control segment's byte range
// slices the full (stable+suffix) text; control segments (BOS, the assistant
// generation cue) are skipped because apply_chat_template adds them.
func chatMessagesFromManifest(fullText string, m transport.ContextManifest) []ovsession.ChatMessage {
	var msgs []ovsession.ChatMessage
	for _, seg := range m.Segments {
		role := chatRole(seg.Kind)
		if role == "" {
			continue
		}
		if seg.ByteStart < 0 || seg.ByteEnd > len(fullText) || seg.ByteStart > seg.ByteEnd {
			continue
		}
		msgs = append(msgs, ovsession.ChatMessage{
			Role:       role,
			Content:    fullText[seg.ByteStart:seg.ByteEnd],
			ToolCalls:  seg.ToolCallsJSON,
			ToolCallID: seg.ToolCallID,
		})
	}
	return msgs
}

func chatRole(kind string) string {
	switch kind {
	case "system", "user", "assistant", "tool":
		return kind
	default:
		return ""
	}
}

// decodeOptions maps the backend-neutral decode config onto OpenVINO GenAI's
// generate options. TopK and Seed have no GenAI GenerateOptions equivalent and
// are intentionally dropped here.
func decodeOptions(cfg transport.DecodeConfig) ovsession.GenerateOptions {
	opts := ovsession.GenerateOptions{MaxNewTokens: cfg.MaxTokens, ParserProtocols: cfg.ParserProtocols}
	if opts.MaxNewTokens <= 0 {
		opts.MaxNewTokens = 256
	}
	if cfg.StructuredOutput.Protocol != "" {
		opts.StructuredOutput = ovsession.StructuredOutput{
			Protocol: openvinoStructuredProtocol(cfg.StructuredOutput.Protocol),
			Payload:  cfg.StructuredOutput.Payload,
		}
	}
	if cfg.Temperature != nil {
		v := *cfg.Temperature
		opts.Temperature = &v
	}
	if cfg.TopP != nil {
		v := *cfg.TopP
		opts.TopP = &v
	}
	return opts
}

func openvinoStructuredProtocol(protocol string) string {
	switch protocol {
	case "openvino:json_schema_tool_calls":
		return "openvino:json_schema"
	default:
		return protocol
	}
}
