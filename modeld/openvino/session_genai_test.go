package openvino

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/transport"
)

// fakeGenAIBackend is a string-prompt GenAI stand-in: it records the prompt it
// is asked to stream and tokenizes by rune count, so the adapter's text mapping
// can be tested without the CGO OpenVINO backend.
type fakeGenAIBackend struct {
	streamPrompts   []string
	streamOptions   []ovsession.GenerateOptions
	generatePrompts []string
	generateOptions []ovsession.GenerateOptions
	templateCalls   [][]ovsession.ChatMessage
	templateTools   []string
	generateResult  ovsession.GenAIResult
	generateErr     error
	emit            []string
	emitChunks      []ovsession.StreamChunk
	closed          bool
}

func (f *fakeGenAIBackend) Generate(_ context.Context, prompt string, opts ovsession.GenerateOptions) (ovsession.GenAIResult, error) {
	f.generatePrompts = append(f.generatePrompts, prompt)
	f.generateOptions = append(f.generateOptions, opts)
	return f.generateResult, f.generateErr
}

func (f *fakeGenAIBackend) Stream(_ context.Context, prompt string, opts ovsession.GenerateOptions) (<-chan ovsession.StreamChunk, error) {
	f.streamPrompts = append(f.streamPrompts, prompt)
	f.streamOptions = append(f.streamOptions, opts)
	size := len(f.emit)
	if len(f.emitChunks) > 0 {
		size = len(f.emitChunks)
	}
	ch := make(chan ovsession.StreamChunk, size)
	if len(f.emitChunks) > 0 {
		for _, chunk := range f.emitChunks {
			ch <- chunk
		}
	} else {
		for _, t := range f.emit {
			ch <- ovsession.StreamChunk{Text: t}
		}
	}
	close(ch)
	return ch, nil
}

func (f *fakeGenAIBackend) Tokenize(_ context.Context, prompt string, _ bool) ([]int, error) {
	return make([]int, len([]rune(prompt))), nil
}

func (f *fakeGenAIBackend) ApplyChatTemplate(messages []ovsession.ChatMessage, tools string) (string, error) {
	cp := append([]ovsession.ChatMessage(nil), messages...)
	f.templateCalls = append(f.templateCalls, cp)
	f.templateTools = append(f.templateTools, tools)
	out := ""
	for _, m := range messages {
		out += "<|" + m.Role + "|>" + m.Content
	}
	return out, nil
}

func (f *fakeGenAIBackend) Close() error { f.closed = true; return nil }

func ovManifest(stableHash, runtimeDigest string) contextasm.ContextManifest {
	return contextasm.ContextManifest{
		Backend:              "openvino",
		ModelDigest:          "model-d1",
		PromptFormat:         "openvino_chat_template",
		PromptTemplateDigest: "model-d1",
		RuntimeDigest:        runtimeDigest,
		StableByteHash:       stableHash,
	}
}

func TestGenaiSessionDecodePreservesToolHistoryForChatTemplate(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{"ok"}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	fullText := "rulesaskcallingresult"
	stable := "rules"
	m := ovManifest(contextasm.HashString(stable), "r1")
	m.Segments = []contextasm.ManifestSegment{
		{Kind: "system", Stable: true, ByteStart: 0, ByteEnd: 5, ByteHash: contextasm.HashString("rules")},
		{Kind: "user", ByteStart: 5, ByteEnd: 8, ByteHash: contextasm.HashString("ask")},
		{Kind: "assistant", ByteStart: 8, ByteEnd: 15, ByteHash: contextasm.HashString("calling"), ToolCallsJSON: `[{"id":"call_123","type":"function","function":{"name":"lookup","arguments":"{}"}}]`},
		{Kind: "tool", ByteStart: 15, ByteEnd: len(fullText), ByteHash: contextasm.HashString("result"), ToolCallID: "call_123"},
	}

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: stable, Tools: `[{"type":"function"}]`, Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: strings.TrimPrefix(fullText, stable), Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 1})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for range ch {
	}

	if len(fake.templateCalls) != 1 {
		t.Fatalf("template calls = %d, want 1", len(fake.templateCalls))
	}
	msgs := fake.templateCalls[0]
	if len(msgs) != 4 {
		t.Fatalf("template messages = %+v, want 4", msgs)
	}
	if msgs[2].Role != "assistant" || !strings.Contains(msgs[2].ToolCalls, "call_123") {
		t.Fatalf("assistant tool_calls not preserved: %+v", msgs[2])
	}
	if msgs[3].Role != "tool" || msgs[3].ToolCallID != "call_123" || msgs[3].Content != "result" {
		t.Fatalf("tool result metadata not preserved: %+v", msgs[3])
	}
	if len(fake.templateTools) != 1 || fake.templateTools[0] != `[{"type":"function"}]` {
		t.Fatalf("template tools = %+v", fake.templateTools)
	}
}

func TestGenaiSessionDecodeConcatenatesStableAndSuffix(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{"hel", "lo"}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 8})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var out string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		out += chunk.Text
	}
	if out != "hello" {
		t.Errorf("decoded text = %q, want %q", out, "hello")
	}
	if len(fake.streamPrompts) != 1 || fake.streamPrompts[0] != "SYSTEMUSER" {
		t.Errorf("streamed prompt = %v, want one prompt %q", fake.streamPrompts, "SYSTEMUSER")
	}
}

func TestGenaiSessionDecodePassesParserProtocolsAndThinking(t *testing.T) {
	fake := &fakeGenAIBackend{emitChunks: []ovsession.StreamChunk{
		{Thinking: "reason "},
		{Text: "answer"},
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{
		MaxTokens:       8,
		ParserProtocols: []string{"openvino:deepseek_r1_reasoning_incremental_parser"},
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var text, thinking string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		text += chunk.Text
		thinking += chunk.Thinking
	}
	if text != "answer" || thinking != "reason " {
		t.Fatalf("decoded text/thinking = %q/%q, want answer/reason", text, thinking)
	}
	if len(fake.streamOptions) != 1 {
		t.Fatalf("stream options = %d, want 1", len(fake.streamOptions))
	}
	if got := fake.streamOptions[0].ParserProtocols; len(got) != 1 || got[0] != "openvino:deepseek_r1_reasoning_incremental_parser" {
		t.Fatalf("parser protocols = %+v", got)
	}
}

func TestGenaiSessionDecodeCompleteParserEmitsParsedToolCalls(t *testing.T) {
	fake := &fakeGenAIBackend{
		generateResult: ovsession.GenAIResult{
			Text:       "raw model output",
			ParsedJSON: `{"content":"preface","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup_weather","arguments":{"city":"Berlin"}}}]}`,
		},
	}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{
		MaxTokens:       8,
		ParserProtocols: []string{"openvino:llama3_json_tool_parser"},
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var chunks []transport.StreamChunk
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("decode error: %v", chunk.Error)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0].Text != "preface" {
		t.Fatalf("text = %q, want parsed content", chunks[0].Text)
	}
	if len(chunks[0].ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want 1", chunks[0].ToolCalls)
	}
	call := chunks[0].ToolCalls[0]
	if call.ID != "call_1" || call.Type != "function" || call.Function.Name != "lookup_weather" || call.Function.Arguments != `{"city":"Berlin"}` {
		t.Fatalf("tool call = %+v", call)
	}
	if len(fake.generateOptions) != 1 || fake.generateOptions[0].ParserProtocols[0] != "openvino:llama3_json_tool_parser" {
		t.Fatalf("generate options = %+v", fake.generateOptions)
	}
	if len(fake.streamPrompts) != 0 {
		t.Fatalf("complete parser should use Generate, got stream prompts %+v", fake.streamPrompts)
	}
}

func TestGenaiSessionDecodeStructuredJSONSchemaToolCalls(t *testing.T) {
	fake := &fakeGenAIBackend{
		generateResult: ovsession.GenAIResult{
			Text: `{"tool_calls":[{"function":{"name":"echo.echo","arguments":{"input":"hello"}}}]}`,
		},
	}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{
		MaxTokens: 8,
		StructuredOutput: transport.StructuredOutputConfig{
			Protocol: "openvino:json_schema_tool_calls",
			Payload:  `{"type":"object"}`,
		},
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var chunks []transport.StreamChunk
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("decode error: %v", chunk.Error)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 || len(chunks[0].ToolCalls) != 1 {
		t.Fatalf("chunks = %+v, want one parsed tool call", chunks)
	}
	call := chunks[0].ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "echo.echo" || call.Function.Arguments != `{"input":"hello"}` {
		t.Fatalf("tool call = %+v", call)
	}
	if len(fake.generateOptions) != 1 {
		t.Fatalf("generate options = %+v", fake.generateOptions)
	}
	if got := fake.generateOptions[0].StructuredOutput; got.Protocol != "openvino:json_schema" || got.Payload != `{"type":"object"}` {
		t.Fatalf("structured output = %+v", got)
	}
	if len(fake.streamPrompts) != 0 {
		t.Fatalf("structured output should use Generate, got stream prompts %+v", fake.streamPrompts)
	}
}

func TestGenaiSessionDecodeStructuredJSONSchemaRequiresToolCallsEnvelope(t *testing.T) {
	_, err := chunkFromStructuredToolJSON(`[{"function":{"name":"echo.echo","arguments":{"input":"hello"}}}]`)
	if err == nil || !strings.Contains(err.Error(), "structured tool call envelope") {
		t.Fatalf("array fallback error = %v", err)
	}

	_, err = chunkFromStructuredToolJSON(`{"function":{"name":"echo.echo","arguments":{"input":"hello"}}}`)
	if err == nil || !strings.Contains(err.Error(), "contained no tool_calls") {
		t.Fatalf("single-object fallback error = %v", err)
	}
}

func TestGenaiSessionDecodeRejectsQwenXMLParametersStructuredOutput(t *testing.T) {
	fake := &fakeGenAIBackend{
		generateResult: ovsession.GenAIResult{
			Text: `<parameter=input>hello</parameter><parameter=count>2</parameter>`,
		},
	}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{
		MaxTokens: 8,
		StructuredOutput: transport.StructuredOutputConfig{
			Protocol: "openvino:qwen_xml_parameters",
			Payload:  `{"type":"object"}`,
			ToolName: "echo.echo",
		},
	})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for chunk := range ch {
		if chunk.Error == nil {
			t.Fatalf("chunk error = nil, want unsupported structured output protocol")
		}
		if !strings.Contains(chunk.Error.Error(), `unsupported structured output result protocol "openvino:qwen_xml_parameters"`) {
			t.Fatalf("chunk error = %v", chunk.Error)
		}
		return
	}
	t.Fatal("decode produced no chunk")
}

func TestGenaiSessionWarmReuseReportedOnRepeatedPrefix(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	cold, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix cold: %v", err)
	}
	if cold.ReusedTokens != 0 || cold.PrefilledTokens != len("STABLE") {
		t.Errorf("cold status = %+v, want reused=0 prefilled=%d", cold, len("STABLE"))
	}
	warm, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: m})
	if err != nil {
		t.Fatalf("EnsurePrefix warm: %v", err)
	}
	if warm.ReusedTokens != len("STABLE") || warm.PrefilledTokens != 0 {
		t.Errorf("warm status = %+v, want reused=%d prefilled=0", warm, len("STABLE"))
	}
}

func TestGenaiSessionIncompatibleManifestDropsPrefix(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r1")}); err != nil {
		t.Fatalf("EnsurePrefix first: %v", err)
	}
	// Different runtime digest => incompatible => the resident string prefix must
	// be dropped, not reused.
	got, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r2")})
	if err != nil {
		t.Fatalf("EnsurePrefix second: %v", err)
	}
	if got.ReusedTokens != 0 {
		t.Errorf("reused tokens = %d across an incompatible runtime, want 0", got.ReusedTokens)
	}
	if got.DroppedTokens != len("STABLE") {
		t.Errorf("dropped tokens = %d, want %d", got.DroppedTokens, len("STABLE"))
	}
}

func TestGenaiSessionSuffixManifestMismatchRejected(t *testing.T) {
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	ctx := context.Background()

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r1")}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	_, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: ovManifest("hash-AAA", "r2")})
	if !errors.Is(err, contextasm.ErrManifestMismatch) {
		t.Fatalf("PrefillSuffix mismatch error = %v, want ErrManifestMismatch", err)
	}
}

func TestGenaiSessionSnapshotRestoreLogicalState(t *testing.T) {
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Tools: `[{"type":"function"}]`, Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	snap, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.State) != 0 {
		t.Fatalf("OpenVINO logical snapshot should not claim opaque KV bytes, got %d", len(snap.State))
	}
	if snap.ResidentTokens != len("STABLEUSER") || snap.PrefixTokens != len("STABLE") || snap.StableText != "STABLE" || snap.PrefixText != "STABLEUSER" {
		t.Fatalf("snapshot did not capture logical state: %+v", snap)
	}

	fake := &fakeGenAIBackend{}
	restored := newGenaiSession(fake, 4096)
	if err := restored.Restore(ctx, snap); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := restored.ExplainContext(); got.ResidentTokens != len("STABLEUSER") || got.PrefixTokens != len("STABLE") || got.ManifestDigest == "" {
		t.Fatalf("restored context = %+v", got)
	}
	ch, err := restored.Decode(ctx, transport.DecodeConfig{MaxTokens: 1})
	if err != nil {
		t.Fatalf("Decode restored: %v", err)
	}
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("decode chunk error: %v", chunk.Error)
		}
	}
	if len(fake.streamPrompts) != 1 || fake.streamPrompts[0] != "STABLEUSER" {
		t.Fatalf("restored prompt = %v, want STABLEUSER", fake.streamPrompts)
	}
}

func TestGenaiSessionRestoreRejectsIncompatibleRuntime(t *testing.T) {
	ctx := context.Background()
	s := newGenaiSession(&fakeGenAIBackend{}, 4096)
	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "STABLE", Manifest: ovManifest("hash-AAA", "r1")}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	err := s.Restore(ctx, transport.SessionSnapshot{
		ResidentTokens: len("STABLE"),
		PrefixTokens:   len("STABLE"),
		NumCtx:         4096,
		StableText:     "STABLE",
		PrefixText:     "STABLE",
		Manifest:       ovManifest("hash-AAA", "r2"),
	})
	if !errors.Is(err, contextasm.ErrManifestMismatch) {
		t.Fatalf("Restore mismatch error = %v, want ErrManifestMismatch", err)
	}
}

func TestGenaiSessionCloseStopsUse(t *testing.T) {
	fake := &fakeGenAIBackend{}
	s := newGenaiSession(fake, 4096)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.closed {
		t.Error("Close did not reach the backend")
	}
	if _, err := s.EnsurePrefix(context.Background(), transport.PrefixInput{Text: "x"}); !errors.Is(err, transport.ErrSessionClosed) {
		t.Fatalf("EnsurePrefix after close = %v, want ErrSessionClosed", err)
	}
}
