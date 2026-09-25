package openvino

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/transport"
)

const testToolsJSON = `[{"type":"function","function":{"name":"native-git.git_add","parameters":{"type":"object","properties":{"files":{"type":"array","items":{"type":"string"}}}}}}]`

func drainDecode(t *testing.T, ch <-chan transport.StreamChunk) (string, []transport.ToolCall, error) {
	t.Helper()
	var text string
	var calls []transport.ToolCall
	var streamErr error
	for chunk := range ch {
		if chunk.Error != nil {
			streamErr = chunk.Error
			continue
		}
		text += chunk.Text
		calls = append(calls, chunk.ToolCalls...)
	}
	return text, calls, streamErr
}

func TestGenaiSessionDecodeTokenStreamHoldsToolCall(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{
		"I'll stage the changed files.",
		" <tool_",
		"call>{\"name\":\"native-git.git_add\",\"arguments\":{\"files\":[\"a.go\"]}}</tool_",
		"call>",
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m, Tools: testToolsJSON}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 32})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	text, calls, streamErr := drainDecode(t, ch)
	if streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	if len(fake.streamTokenPrompts) != 1 {
		t.Fatalf("streamed token prompts = %d, want the resident-token path", len(fake.streamTokenPrompts))
	}
	if strings.TrimSpace(text) != "I'll stage the changed files." {
		t.Errorf("decoded text = %q, want the prose before the call and nothing else", text)
	}
	if len(calls) != 1 || calls[0].Function.Name != "native-git.git_add" {
		t.Fatalf("tool calls = %+v, want one native-git.git_add call", calls)
	}
}

func TestGenaiSessionDecodePromptStreamHoldsToolCall(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{
		"Staging now.",
		"<tool_call><function=native-git.git_add><parameter=files>[\"a.go\"]</parameter></function></tool_call>",
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if err := s.Restore(ctx, transport.SessionSnapshot{
		ResidentTokens: 0,
		PrefixTokens:   0,
		StableText:     "SYSTEM",
		PrefixText:     "SYSTEM",
		Manifest:       m,
		Tools:          testToolsJSON,
	}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 32})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	text, calls, streamErr := drainDecode(t, ch)
	if streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	if len(fake.streamPrompts) != 1 {
		t.Fatalf("streamed prompts = %d, want the string-prompt path", len(fake.streamPrompts))
	}
	if text != "Staging now." {
		t.Errorf("decoded text = %q, want the prose before the call and nothing else", text)
	}
	if len(calls) != 1 || calls[0].Function.Name != "native-git.git_add" {
		t.Fatalf("tool calls = %+v, want one native-git.git_add call", calls)
	}
	if !strings.Contains(calls[0].Function.Arguments, "a.go") {
		t.Errorf("call arguments = %q, want the XML parameter recovered", calls[0].Function.Arguments)
	}
}

func TestGenaiSessionDecodeTokenStreamUnreadableToolCallFails(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{
		"I'll stage the changed files.",
		"<tool_call>{\"name\":\"native-git.git_add\",\"arguments\":{\"files\"",
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m, Tools: testToolsJSON}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 32})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	text, calls, streamErr := drainDecode(t, ch)
	if !errors.Is(streamErr, errUnreadableToolCall) {
		t.Fatalf("stream error = %v, want errUnreadableToolCall", streamErr)
	}
	if calls != nil {
		t.Errorf("tool calls = %+v, want none from a truncated block", calls)
	}
	if strings.Contains(text, "<tool_call>") {
		t.Errorf("decoded text = %q, want no call block delivered as prose", text)
	}
}

func TestGenaiSessionDecodeWithoutToolsPassesToolCallTextThrough(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{
		"The format is ",
		"<tool_call>{\"name\":\"x\"}</tool_call>",
		" in this template.",
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 32})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	text, calls, streamErr := drainDecode(t, ch)
	if streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	want := "The format is <tool_call>{\"name\":\"x\"}</tool_call> in this template."
	if text != want {
		t.Errorf("decoded text = %q, want %q", text, want)
	}
	if calls != nil {
		t.Errorf("tool calls = %+v, want none when no tools were rendered", calls)
	}
}

// TestGenaiSessionDecodeToolStreamStripsAStrayCloser pins the tail case in the
// one path where the dialect is in play: a decode that rendered tools must not
// deliver the previous call's closing tag as text.
func TestGenaiSessionDecodeToolStreamStripsAStrayCloser(t *testing.T) {
	fake := &fakeGenAIBackend{emit: []string{
		"done",
		" </tool_call> next step",
	}}
	s := newGenaiSession(fake, 4096)
	ctx := context.Background()
	m := ovManifest("hash-AAA", "r1")

	if _, err := s.EnsurePrefix(ctx, transport.PrefixInput{Text: "SYSTEM", Manifest: m, Tools: testToolsJSON}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := s.PrefillSuffix(ctx, transport.SuffixInput{Text: "USER", Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}
	ch, err := s.Decode(ctx, transport.DecodeConfig{MaxTokens: 32})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	text, calls, streamErr := drainDecode(t, ch)
	if streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	if strings.Contains(text, "</tool_call>") {
		t.Errorf("decoded text = %q, want the closer stripped", text)
	}
	if !strings.Contains(text, "done") || !strings.Contains(text, "next step") {
		t.Errorf("decoded text = %q, want the prose around the tag kept", text)
	}
	if calls != nil {
		t.Errorf("tool calls = %+v, want none", calls)
	}
}
