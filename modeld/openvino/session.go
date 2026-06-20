package openvino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/modeld/internal/sessionkit"
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
// OpenVINO GenAI owns the tokenizer, chat template, and physical prefix cache
// inside ContinuousBatchingPipeline. The adapter keeps transport-level token
// accounting and feeds model-native prompt strings through the existing GenAI
// session API.
type genaiSession struct {
	backend genaiBackend
	numCtx  int

	mu        sync.Mutex
	closed    bool
	manifest  transport.ContextManifest
	stable    string // raw stable text from the runtime
	suffix    string // raw volatile text appended after stable
	tools     string // model-native tool definitions JSON, rendered via the chat template
	resident  []int  // logical token IDs resident by contract; GenAI owns physical KV
	prefixLen int    // how many resident tokens belong to the stable prefix
}

func newGenaiSession(backend genaiBackend, numCtx int) *genaiSession {
	return &genaiSession{backend: backend, numCtx: numCtx}
}

var newEmbedSession = func(modelPath, device string) (EmbedSessionBackend, error) {
	return ovsession.NewEmbed(modelPath, device)
}

var _ transport.Session = (*genaiSession)(nil)

func (s *genaiSession) residentTokens() int { return len(s.resident) }

func (s *genaiSession) available() int {
	if s.numCtx <= 0 {
		return 0
	}
	return s.numCtx - s.residentTokens()
}

func (s *genaiSession) tokenize(ctx context.Context, text string, addSpecial bool) ([]int, error) {
	if text == "" {
		return nil, nil
	}
	return s.backend.Tokenize(ctx, text, addSpecial)
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
	oldResident := s.residentTokens()

	tokens, err := s.tokenize(ctx, prefix.Text, prefix.Manifest.AddBOS)
	if err != nil {
		return transport.PrefixStatus{}, fmt.Errorf("openvino: tokenize stable prefix: %w", err)
	}
	if s.numCtx > 0 && len(tokens) > s.numCtx {
		return transport.PrefixStatus{}, transport.ErrContextOverflow
	}

	reuse := 0
	if ok, _ := s.manifest.CompatibleRuntime(prefix.Manifest); ok {
		reuse = sessionkit.CommonPrefixLen(s.resident, tokens)
	}
	stableTokenHash := contextasm.HashTokenIDs(tokens)

	// EnsurePrefix replaces the stable prefix and drops any prior suffix.
	s.stable = prefix.Text
	s.suffix = ""
	s.tools = prefix.Tools
	s.resident = append(s.resident[:0], tokens...)
	s.prefixLen = len(tokens)
	s.manifest = prefix.Manifest
	s.manifest.StableTokenHash = stableTokenHash

	return transport.PrefixStatus{
		ReusedTokens:    reuse,
		PrefilledTokens: len(tokens) - reuse,
		DroppedTokens:   oldResident - reuse,
		PrefixTokens:    s.prefixLen,
		ResidentTokens:  s.residentTokens(),
		AvailableTokens: s.available(),
		StableByteHash:  stableHash,
		StableTokenHash: s.manifest.StableTokenHash,
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

	suffixText := suffix.Text
	fullText := s.stable + suffix.Text
	if msgs := chatMessagesFromManifest(fullText, suffix.Manifest); len(msgs) > 0 {
		if _, err := s.backend.ApplyChatTemplate(msgs, s.tools); err != nil {
			return transport.SuffixStatus{}, fmt.Errorf("openvino: apply full chat template: %w", err)
		}
	}

	addSpecial := s.prefixLen == 0 && suffix.Manifest.AddBOS
	tokens, err := s.tokenize(ctx, suffixText, addSpecial)
	if err != nil {
		return transport.SuffixStatus{}, fmt.Errorf("openvino: tokenize suffix: %w", err)
	}
	if s.numCtx > 0 && s.residentTokens()+len(tokens) > s.numCtx {
		return transport.SuffixStatus{}, transport.ErrContextOverflow
	}
	resident := append(append([]int(nil), s.resident...), tokens...)
	s.suffix += suffix.Text
	s.resident = resident

	return transport.SuffixStatus{
		SuffixTokens:    len(tokens),
		PrefixTokens:    s.prefixLen,
		ResidentTokens:  s.residentTokens(),
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
	resident := s.residentTokens()
	numCtx := s.numCtx
	s.mu.Unlock()

	opts := decodeOptions(cfg)
	out := make(chan transport.StreamChunk, 16)
	if numCtx > 0 && resident >= numCtx {
		go func() {
			defer close(out)
			_ = sessionkit.Send(ctx, out, transport.StreamChunk{Error: transport.ErrContextOverflow})
		}()
		return out, nil
	}
	if numCtx > 0 && resident+opts.MaxNewTokens > numCtx {
		opts.MaxNewTokens = numCtx - resident
	}

	prompt := fullText
	if msgs := chatMessagesFromManifest(fullText, manifest); len(msgs) > 0 {
		if templated, err := backend.ApplyChatTemplate(msgs, tools); err == nil && strings.TrimSpace(templated) != "" {
			prompt = templated
		}
	}
	return s.decodePrompt(ctx, backend, prompt, opts, cfg, out)
}

func (s *genaiSession) decodePrompt(ctx context.Context, backend genaiBackend, prompt string, opts ovsession.GenerateOptions, cfg transport.DecodeConfig, out chan transport.StreamChunk) (<-chan transport.StreamChunk, error) {
	if cfg.StructuredOutput.Protocol != "" || usesCompleteParser(opts.ParserProtocols) {
		go func() {
			defer close(out)
			res, err := backend.Generate(ctx, prompt, opts)
			if err != nil {
				_ = sessionkit.Send(ctx, out, transport.StreamChunk{Error: err})
				return
			}
			chunk, err := chunkFromGenAIResult(res, cfg.StructuredOutput)
			if err != nil {
				_ = sessionkit.Send(ctx, out, transport.StreamChunk{Error: err})
				return
			}
			if !sessionkit.Send(ctx, out, chunk) {
				sessionkit.TrySend(out, transport.StreamChunk{Error: ctx.Err()})
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
				sessionkit.TrySend(out, transport.StreamChunk{Error: ctx.Err()})
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
		ResidentTokens:  s.residentTokens(),
		PrefixTokens:    s.prefixLen,
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
		ResidentTokens:   s.residentTokens(),
		PrefixTokens:     s.prefixLen,
		NumCtx:           s.numCtx,
		ResidentTokenIDs: append([]int(nil), s.resident...),
		StableText:       s.stable,
		PrefixText:       s.stable + s.suffix,
		Tools:            s.tools,
		Manifest:         s.manifest,
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
	resident := append([]int(nil), snap.ResidentTokenIDs...)
	if len(resident) == 0 && snap.ResidentTokens > 0 {
		var err error
		resident, err = s.tokenize(ctx, prefixText, snap.Manifest.AddBOS)
		if err != nil {
			return fmt.Errorf("openvino: tokenize snapshot: %w", err)
		}
		if len(resident) != snap.ResidentTokens {
			return contextasm.NewManifestMismatchError("snapshot resident token count changed under tokenizer")
		}
	}
	if len(resident) != snap.ResidentTokens {
		return contextasm.NewManifestMismatchError("snapshot resident token ids do not match resident token count")
	}
	s.stable = snap.StableText
	s.suffix = strings.TrimPrefix(prefixText, snap.StableText)
	s.prefixLen = snap.PrefixTokens
	s.resident = resident
	s.tools = snap.Tools
	s.manifest = snap.Manifest
	if s.manifest.StableTokenHash == "" && s.prefixLen <= len(s.resident) {
		s.manifest.StableTokenHash = contextasm.HashTokenIDs(s.resident[:s.prefixLen])
	}
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
		role := sessionkit.ChatRole(seg.Kind)
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
