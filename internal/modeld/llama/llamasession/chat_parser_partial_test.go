//go:build llamanode && llamacpp_direct

package llamasession

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/modeld/llama/llamacppshim"
)

// TestUnit_ChatOutputParser_SwallowsPartialParseError is the B2 regression guard
// for the streaming-tolerance fix. It drives the parser through the seam so a
// mid-stream (partial=true) parse failure is deterministic regardless of the
// live grammar's leniency. A partial failure must be swallowed (no error, no
// delta, state preserved); the authoritative final (partial=false) parse then
// emits the full cumulative content. A final-parse failure stays fatal.
func TestUnit_ChatOutputParser_SwallowsPartialParseError(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })

	// Simulate llama.cpp's peg parser: reject every partial fragment as a hard
	// error (the result.end==0 throw path), succeed only on the final parse.
	partialFailures := 0
	parseChatResponse = func(input string, partial bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		if partial {
			partialFailures++
			return llamacppshim.ChatParseResult{}, errors.New("llamacppshim: common chat parse: The model produced output that does not match the expected peg-native format")
		}
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "deepseek", parseToolCalls: true}

	// Two partial pushes: both must be swallowed (no error, no delta emitted).
	for i, piece := range []string{"<think>hi", "</think>the ans"} {
		text, thinking, tools, err := p.Push(piece, true)
		if err != nil {
			t.Fatalf("partial push %d returned error (B2 regression): %v", i, err)
		}
		if text != "" || thinking != "" || len(tools) != 0 {
			t.Fatalf("partial push %d emitted a delta while unparseable: text=%q thinking=%q tools=%d", i, text, thinking, len(tools))
		}
	}
	if partialFailures != 2 {
		t.Fatalf("expected 2 tolerated partial failures, got %d", partialFailures)
	}

	// Final push: the authoritative parse succeeds and the full accumulated
	// content is delivered as one cumulative delta.
	text, _, _, err := p.Push("wer", false)
	if err != nil {
		t.Fatalf("final push returned error: %v", err)
	}
	if want := "<think>hi</think>the answer"; text != want {
		t.Fatalf("final content mismatch:\n got=%q\nwant=%q", text, want)
	}
}

// TestUnit_ChatOutputParser_FinalParseErrorIsFatal confirms the complement: a
// failure on the final (partial=false) parse still aborts the turn, carrying the
// bounded diagnostics.
func TestUnit_ChatOutputParser_FinalParseErrorIsFatal(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(_ string, _ bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		return llamacppshim.ChatParseResult{}, errors.New("common chat parse: broken")
	}
	p := &chatOutputParser{reasoningFormat: "deepseek"}
	if _, _, _, err := p.Push("garbage", false); err == nil {
		t.Fatal("expected final-parse failure to be fatal, got nil error")
	}
}

func TestUnit_ChatOutputParser_FinalParseErrorFallsBackToQwenToolCallTags(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, parseTools bool) (llamacppshim.ChatParseResult, error) {
		if parseTools {
			return llamacppshim.ChatParseResult{}, errors.New("llamacppshim: common chat parse: The model produced output that does not match the expected peg-native format")
		}
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "deepseek", parseToolCalls: true}
	raw := "<tool_call>\n{\"name\":\"echo\",\"arguments\":{\"input\":\"TOOL_STRICT_OK\"}}\n</tool_call>"
	text, thinking, tools, err := p.Push(raw, false)
	if err != nil {
		t.Fatalf("final qwen tool-call fallback returned error: %v", err)
	}
	if text != "" || thinking != "" {
		t.Fatalf("fallback emitted content: text=%q thinking=%q", text, thinking)
	}
	if len(tools) != 1 {
		t.Fatalf("tool calls = %+v, want one", tools)
	}
	call := tools[0]
	if call.ID != "call_1" || call.Type != "function" || call.Function.Name != "echo" {
		t.Fatalf("normalized tool call = %+v", call)
	}
	if call.Function.Arguments != `{"input":"TOOL_STRICT_OK"}` {
		t.Fatalf("arguments = %q", call.Function.Arguments)
	}
}

// TestUnit_ChatOutputParser_RescuesAnUnknownNameInTheAttributeDialect covers the
// failure the grammar cannot recover from on its own: MiniCPM5 writes a call in a
// dialect the template-derived parser reads, but the tool rule is built from the
// rendered roster, so a name that is not in it makes the whole parse throw. The
// dialect reader has to read that block anyway, or the turn dies and the model
// repeats the same call — which is what a live session did, six times, on
// "tool local_fs.list_dir not found".
func TestUnit_ChatOutputParser_RescuesAnUnknownNameInTheAttributeDialect(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, parseTools bool) (llamacppshim.ChatParseResult, error) {
		if parseTools {
			return llamacppshim.ChatParseResult{}, errors.New("llamacppshim: common chat parse: The model produced output that does not match the expected peg-native format")
		}
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	raw := "I should list the directory.\n</think>\n\n" +
		"<function name=\"local_fs.list_dir\"><param name=\"path\">.</param></function>\n" +
		"<function name=\"local_shell\"><param name=\"command\"><![CDATA[git log --oneline -10]]></param></function>"

	_, _, tools, err := p.Push(raw, false)
	if err != nil {
		t.Fatalf("an unreadable call must be rescued, not fatal: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tool calls = %+v, want both rescued", tools)
	}
	if tools[0].Function.Name != "local_fs.list_dir" || tools[1].Function.Name != "local_shell" {
		t.Fatalf("names = %q, %q", tools[0].Function.Name, tools[1].Function.Name)
	}
	if want := `{"command":"git log --oneline -10"}`; tools[1].Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", tools[1].Function.Arguments, want)
	}
}

// TestUnit_ChatOutputParser_RealQwenStreamReconstructs streams a real captured
// qwen3 completion (see TestSystem_LlamaChatParser_QwenThinkingStreamTolerated)
// rune-by-rune through the actual CGo parser and asserts the turn never aborts
// and the streamed content/thinking equals a single parse of the whole output.
// This guards the live streaming path against regressions on real model output.
func TestUnit_ChatOutputParser_RealQwenStreamReconstructs(t *testing.T) {
	sb, err := os.ReadFile(filepath.Join("testdata", "qwen3_chat_syntax.json"))
	if err != nil {
		t.Fatalf("read syntax fixture: %v", err)
	}
	var c struct {
		Format           int    `json:"format"`
		Parser           string `json:"parser"`
		GenerationPrompt string `json:"generation_prompt"`
		ReasoningFormat  string `json:"reasoning_format"`
		ParseToolCalls   bool   `json:"parse_tool_calls"`
	}
	if err := json.Unmarshal(sb, &c); err != nil {
		t.Fatalf("decode syntax fixture: %v", err)
	}
	rawBytes, err := os.ReadFile(filepath.Join("testdata", "qwen3_raw_completion.txt"))
	if err != nil {
		t.Fatalf("read raw fixture: %v", err)
	}
	raw := string(rawBytes)
	syntax := llamacppshim.ChatSyntax{Format: c.Format, Parser: c.Parser, GenerationPrompt: c.GenerationPrompt}

	want, err := llamacppshim.ParseChatResponse(raw, false, syntax, c.ReasoningFormat, c.ParseToolCalls)
	if err != nil {
		t.Fatalf("reference parse of complete output failed: %v", err)
	}

	p := &chatOutputParser{syntax: syntax, reasoningFormat: c.ReasoningFormat, parseToolCalls: c.ParseToolCalls}
	var gotText, gotThinking string
	runes := []rune(raw)
	for i, r := range runes {
		final := i == len(runes)-1
		text, thinking, _, perr := p.Push(string(r), !final)
		if perr != nil {
			t.Fatalf("push at rune %d (final=%v) returned error (B2 regression): %v", i, final, perr)
		}
		gotText += text
		gotThinking += thinking
	}
	if gotText != want.Content {
		t.Fatalf("streamed content mismatch:\n got=%q\nwant=%q", gotText, want.Content)
	}
	if gotThinking != want.Thinking {
		t.Fatalf("streamed thinking mismatch:\n got=%q\nwant=%q", gotThinking, want.Thinking)
	}
}

// TestUnit_ChatOutputParser_FinalParseErrorFallsBackToQwenXMLToolCalls pins the
// dialect the Qwen 3.x templates document. llama.cpp's grammar for this template is a
// strict sequence over a tool's declared parameters — required first, then optional —
// so a model that writes them in any other order (the production model wrote the
// optional parameter first) is rejected outright, and the turn would be lost without
// this fallback. The rescued text is the real shape: reasoning, content, then the call.
func TestUnit_ChatOutputParser_FinalParseErrorFallsBackToQwenXMLToolCalls(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, parseTools bool) (llamacppshim.ChatParseResult, error) {
		if parseTools {
			return llamacppshim.ChatParseResult{}, errors.New("llamacppshim: common chat parse: The model produced output that does not match the expected peg-native format")
		}
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	raw := "The user is asking about speed with their codebase.\n</think>\n\n" +
		"I'll help you analyze and optimize your codebase for speed.\n\n" +
		"<tool_call>\n<function=local_fs.find_files>\n" +
		"<parameter=path>\n.\n</parameter>\n" +
		"<parameter=pattern>\n*.go\n</parameter>\n" +
		"</function>\n</tool_call>"

	_, _, tools, err := p.Push(raw, false)
	if err != nil {
		t.Fatalf("final qwen XML tool-call fallback returned error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool calls = %+v, want one", tools)
	}
	if tools[0].Function.Name != "local_fs.find_files" {
		t.Fatalf("name = %q", tools[0].Function.Name)
	}
	if want := `{"path":".","pattern":"*.go"}`; tools[0].Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", tools[0].Function.Arguments, want)
	}
}

// TestUnit_ChatOutputParser_SwallowedToolCallIsRescued pins the silent failure: a
// parse can succeed while treating the call as content (the grammar stops
// recognising the dialect). The call must be extracted, never delivered as prose.
func TestUnit_ChatOutputParser_SwallowedToolCallIsRescued(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, parseTools bool) (llamacppshim.ChatParseResult, error) {
		// Tools on: the primary parser succeeds with the whole block as content.
		// Tools off (the fallback's re-parse of the remaining prose): content too.
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	raw := "I'll stage the changed files.\n\n" +
		"<tool_call>\n<function=native-git.git_add>\n" +
		"<parameter=paths>\n[\"a.go\"]\n</parameter>\n" +
		"</function>\n</tool_call>"

	text, _, tools, err := p.Push(raw, false)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if want := "I'll stage the changed files."; text != want {
		t.Fatalf("text delta = %q, want %q (measured, not lost or duplicated)", text, want)
	}
	if len(tools) != 1 {
		t.Fatalf("tool calls = %+v, want the swallowed call", tools)
	}
	if tools[0].Function.Name != "native-git.git_add" {
		t.Fatalf("name = %q", tools[0].Function.Name)
	}
	if want := `{"paths":"[\"a.go\"]"}`; tools[0].Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", tools[0].Function.Arguments, want)
	}
}

// TestUnit_ChatOutputParser_UnreadableToolCallFails pins the other half of the same
// contract: when neither parser can read the block, the turn fails with diagnostics
// instead of passing the block off as an answer.
func TestUnit_ChatOutputParser_UnreadableToolCallFails(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	_, _, _, err := p.Push("here is an example: <tool_call>not really a call</tool_call>", false)
	if err == nil {
		t.Fatal("a tool-call block no parser can read must fail the turn")
	}
	if !errors.Is(err, errUnreadableToolCall) {
		t.Fatalf("error = %v, want errUnreadableToolCall", err)
	}
}

// TestUnit_ChatOutputParser_TruncatedToolCallFails pins the truncated-generation case:
// a block whose payload never arrived is a lost call, so the turn fails with the
// same nameable error instead of printing a half-written call as the answer.
func TestUnit_ChatOutputParser_TruncatedToolCallFails(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	_, _, _, err := p.Push("I'll stage the changed files.\n\n<tool_call>\n<function=native-git.git_add>\n<parameter=paths>\n[\"a.go\"]", false)
	if !errors.Is(err, errUnreadableToolCall) {
		t.Fatalf("error = %v, want errUnreadableToolCall", err)
	}
}

// TestUnit_ChatOutputParser_StreamedToolCallNeverReachesTheClient pins the leak a
// live session showed: with a grammar that does not read the dialect, every
// partial push delivered the block as prose, so by the time the final parse
// rescued the call the markup had already reached the client. The stream is held
// from the first byte that could open a marker instead, and the rescue then
// delivers only what the client has not seen.
func TestUnit_ChatOutputParser_StreamedToolCallNeverReachesTheClient(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	pieces := []string{
		"Let me look.",
		"\n\n<tool_",
		"call>\n<function=local_shell.local_shell>\n<parameter=command>",
		"\ncd /tmp && ls -la\n</parameter>\n</function>",
		"\n</tool_call>",
	}
	var streamed strings.Builder
	for _, piece := range pieces {
		text, thinking, calls, err := p.Push(piece, true)
		if err != nil {
			t.Fatalf("partial push %q: %v", piece, err)
		}
		if len(calls) != 0 {
			t.Fatalf("partial push %q produced calls; the final parse decides", piece)
		}
		streamed.WriteString(text)
		streamed.WriteString(thinking)
	}
	for _, leak := range []string{"<tool", "<function", "<parameter", "</function", "</tool_call"} {
		if strings.Contains(streamed.String(), leak) {
			t.Fatalf("markup reached the client mid-stream (%q in %q)", leak, streamed.String())
		}
	}

	text, _, tools, err := p.Push("", false)
	if err != nil {
		t.Fatalf("final push: %v", err)
	}
	if len(tools) != 1 || tools[0].Function.Name != "local_shell.local_shell" {
		t.Fatalf("tool calls = %+v, want the streamed call", tools)
	}
	if got := strings.TrimSpace(streamed.String() + text); got != "Let me look." {
		t.Fatalf("delivered text = %q, want the prose once and no markup", got)
	}
}

// TestUnit_ChatOutputParser_StrayClosingTagIsNotProse pins the second half of the
// live leak: the turn after a rescued call opened with the previous call's closing
// tag, and it was delivered to the client as text.
func TestUnit_ChatOutputParser_StrayClosingTagIsNotProse(t *testing.T) {
	orig := parseChatResponse
	t.Cleanup(func() { parseChatResponse = orig })
	parseChatResponse = func(input string, _ bool, _ llamacppshim.ChatSyntax, _ string, _ bool) (llamacppshim.ChatParseResult, error) {
		return llamacppshim.ChatParseResult{Content: input}, nil
	}

	p := &chatOutputParser{reasoningFormat: "auto", parseToolCalls: true}
	text, _, tools, err := p.Push("</tool_call>The listing failed, so let me look directly.", true)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tool calls = %+v, want none from a closer alone", tools)
	}
	if strings.Contains(text, "</tool_call>") {
		t.Fatalf("text delta = %q, want the closing tag stripped", text)
	}
	if !strings.Contains(text, "The listing failed") {
		t.Fatalf("text delta = %q, want the prose kept", text)
	}
}
