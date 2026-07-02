package llama

import (
	"context"
	"errors"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func TestUnit_LlamaReasoningProtocol_RejectsUnknownProtocol(t *testing.T) {
	err := validateReasoningProtocol("llama:nope")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestUnit_LlamaClient_ReasoningChatSurfacesThinkingWhenRequested(t *testing.T) {
	fake := &fakeReasoningSession{
		chunks: []StreamChunk{{Text: "visible", Thinking: "modeld-thinking"}},
	}
	resetLlamaReasoningClientTest(t, fake)

	c := &client{
		modelName:         "qwen3",
		modelPath:         "/models/qwen3/model.gguf",
		modelDigest:       "digest",
		backendVersion:    "llama.cpp@test",
		cfg:               Config{NumCtx: 4096, ReasoningFormat: "deepseek"},
		reasoningProtocol: reasoningProtocolCommonChat,
	}
	res, err := c.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "why?"}}, modelrepo.WithThink("high"))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Message.Content != "visible" || res.Message.Thinking != "modeld-thinking" {
		t.Fatalf("message = %+v, want visible content and modeld thinking", res.Message)
	}
	if got := fake.decode.ParserProtocols; len(got) != 1 || got[0] != reasoningProtocolCommonChat {
		t.Fatalf("decode parser protocols = %#v", got)
	}
	if fake.decode.ReasoningFormat != "deepseek" {
		t.Fatalf("decode reasoning format = %q", fake.decode.ReasoningFormat)
	}
	if fake.suffix.EnableThinking == nil || !*fake.suffix.EnableThinking {
		t.Fatalf("suffix enable_thinking = %v, want true", fake.suffix.EnableThinking)
	}
	if fake.suffix.ReasoningEffort != "high" {
		t.Fatalf("suffix reasoning_effort = %q, want high", fake.suffix.ReasoningEffort)
	}
}

func TestUnit_LlamaClient_ReasoningDropsThinkingWhenThinkOff(t *testing.T) {
	fake := &fakeReasoningSession{
		chunks: []StreamChunk{{Text: "visible", Thinking: "modeld-thinking"}},
	}
	resetLlamaReasoningClientTest(t, fake)

	c := &client{
		modelName:         "qwen3",
		modelPath:         "/models/qwen3/model.gguf",
		modelDigest:       "digest",
		backendVersion:    "llama.cpp@test",
		cfg:               Config{NumCtx: 4096, ReasoningFormat: "deepseek"},
		reasoningProtocol: reasoningProtocolCommonChat,
	}
	res, err := c.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "why?"}}, modelrepo.WithThink("off"))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Message.Content != "visible" || res.Message.Thinking != "" {
		t.Fatalf("message = %+v, want visible content and no displayed thinking", res.Message)
	}
	if fake.suffix.EnableThinking == nil || *fake.suffix.EnableThinking {
		t.Fatalf("suffix enable_thinking = %v, want false", fake.suffix.EnableThinking)
	}
	if fake.suffix.ReasoningEffort != "" {
		t.Fatalf("suffix reasoning_effort = %q, want empty for think=off", fake.suffix.ReasoningEffort)
	}
}

func TestUnit_LlamaStream_ReasoningStreamsThinkingFromModeld(t *testing.T) {
	fake := &fakeReasoningSession{
		chunks: []StreamChunk{
			{Thinking: "thought-"},
			{Thinking: "delta", Text: "visible"},
		},
	}
	resetLlamaReasoningClientTest(t, fake)

	c := &client{
		modelName:         "qwen3",
		modelPath:         "/models/qwen3/model.gguf",
		modelDigest:       "digest",
		backendVersion:    "llama.cpp@test",
		cfg:               Config{NumCtx: 4096, ReasoningFormat: "deepseek"},
		reasoningProtocol: reasoningProtocolCommonChat,
	}
	stream, err := c.Stream(context.Background(), []modelrepo.Message{{Role: "user", Content: "why?"}}, modelrepo.WithThink("high"))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var text, thinking string
	for parcel := range stream {
		if parcel.Error != nil {
			t.Fatalf("stream error: %v", parcel.Error)
		}
		text += parcel.Data
		thinking += parcel.Thinking
	}
	if text != "visible" || thinking != "thought-delta" {
		t.Fatalf("stream text/thinking = %q/%q, want visible/thought-delta", text, thinking)
	}
}

func TestUnit_LlamaClient_ToolCallsComeFromModeldChunks(t *testing.T) {
	call := StreamChunk{Text: "visible answer"}
	call.ToolCalls = []ToolCall{newLlamaToolCall("call_1", "lookup", `{"query":"x"}`)}
	fake := &fakeReasoningSession{chunks: []StreamChunk{call}}
	resetLlamaReasoningClientTest(t, fake)

	c := &client{
		modelName:      "qwen3",
		modelPath:      "/models/qwen3/model.gguf",
		modelDigest:    "digest",
		backendVersion: "llama.cpp@test",
		cfg:            Config{NumCtx: 4096},
		toolProtocol:   toolParserProtocolCommonChat,
	}
	res, err := c.Chat(
		context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "lookup"}},
		modelrepo.WithTools(modelrepo.Tool{
			Type: "function",
			Function: &modelrepo.FunctionTool{
				Name:       "lookup",
				Parameters: map[string]any{"type": "object"},
			},
		}),
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Message.Content != call.Text {
		t.Fatalf("content = %q, want modeld text unchanged", res.Message.Content)
	}
	if len(res.Message.ToolCalls) != 1 || res.Message.ToolCalls[0].ID != "call_1" || res.Message.ToolCalls[0].Function.Name != "lookup" {
		t.Fatalf("tool calls = %+v", res.Message.ToolCalls)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ID != "call_1" || res.ToolCalls[0].Function.Name != "lookup" {
		t.Fatalf("top-level tool calls = %+v", res.ToolCalls)
	}
	if got := fake.decode.ParserProtocols; len(got) != 1 || got[0] != toolParserProtocolCommonChat {
		t.Fatalf("decode parser protocols = %#v", got)
	}
}

func newLlamaToolCall(id, name, arguments string) ToolCall {
	var tc ToolCall
	tc.ID = id
	tc.Type = "function"
	tc.Function.Name = name
	tc.Function.Arguments = arguments
	return tc
}

type fakeReasoningSession struct {
	chunks []StreamChunk
	suffix SuffixInput
	decode DecodeConfig
}

func (s *fakeReasoningSession) EnsurePrefix(context.Context, PrefixInput) (PrefixStatus, error) {
	return PrefixStatus{}, nil
}

func (s *fakeReasoningSession) PrefillSuffix(_ context.Context, suffix SuffixInput) (SuffixStatus, error) {
	s.suffix = suffix
	return SuffixStatus{}, nil
}

func (s *fakeReasoningSession) Decode(_ context.Context, cfg DecodeConfig) (<-chan StreamChunk, error) {
	s.decode = cfg
	out := make(chan StreamChunk, len(s.chunks))
	for _, chunk := range s.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

func (s *fakeReasoningSession) ExplainContext() ContextReport { return ContextReport{} }

func (s *fakeReasoningSession) Snapshot(context.Context) (SessionSnapshot, error) {
	return SessionSnapshot{}, nil
}

func (s *fakeReasoningSession) Restore(context.Context, SessionSnapshot) error { return nil }

func (s *fakeReasoningSession) Close() error { return nil }

func resetLlamaReasoningClientTest(t *testing.T, sess Session) {
	t.Helper()
	oldFactory := sessionFactory
	oldWarm := warm
	sessionFactory = func(string, Config) (Session, error) { return sess, nil }
	warm = modelrepo.NewWarmCache[Session]()
	t.Cleanup(func() {
		sessionFactory = oldFactory
		warm.Clear()
		warm = oldWarm
	})
}
