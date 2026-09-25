//go:build llamanode && llamacpp_direct

package llamasession

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/llama"
	"github.com/contenox/contenox/internal/modeld/llama/llamacppshim"
)

// acpToolSetShape mirrors the shape that decides llama.cpp's generated grammar for
// this template: a tool declaring required parameters, plus a sibling with non-string
// ones. The grammar is a strict sequence — required parameters first, then optional —
// so a call that emits an optional parameter first is rejected outright, which is
// what the model did in production: find_files declares pattern required, and the
// model wrote path first.
const acpToolSetShape = `[` +
	`{"type":"function","function":{"name":"local_fs.find_files","description":"Find files","parameters":{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}}},` +
	`{"type":"function","function":{"name":"local_fs.list_dir","description":"List a directory","parameters":{"type":"object","properties":{"path":{"type":"string"},"recursive":{"type":"boolean"},"max_depth":{"type":"integer"}}}}}` +
	`]`

// productionXMLCall is verbatim the output the model returned in the beam session
// that failed: reasoning, the template's closing thinking tag, content, then the XML
// tool call — with its parameters in the model's own order, not the schema's.
const productionXMLCall = "The user is asking about speed with their codebase. I need to first understand what they're referring to. Let me check what files/commands might be available in the project.\n</think>\n\nI'll help you analyze and optimize your codebase for speed. Let me first see what we're working with.\n\n<tool_call>\n<function=local_fs.find_files>\n<parameter=path>\n.\n</parameter>\n<parameter=pattern>\n*.go\n</parameter>\n</function>\n</tool_call>"

// TestSystem_LlamaSession_TypedToolSchemaXMLCallRecovered drives the real grammar and
// the real model output: render with the production tool shape, then stream the
// verbatim call through the output parser. Whichever path resolves it, the turn must
// end with the tool call extracted and the reasoning split off — before the XML
// dialect was supported this ended the turn with a peg-native parse error.
func TestSystem_LlamaSession_TypedToolSchemaXMLCallRecovered(t *testing.T) {
	modelPath := qwen35ModelPath(t)

	sess, err := New(modelPath, llama.Config{NumCtx: 2048, NumBatch: 64, NumThreads: 4, DisableBOS: true, ReasoningFormat: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	stable := "system\nYou are a helpful assistant.\n"
	turn := "user\nWhich Go files are in this repository?\n"
	m := tinyManifest(stable, turn)
	if _, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m, Tools: acpToolSetShape}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: turn, Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}

	s, ok := sess.(*session)
	if !ok {
		t.Fatal("session is not the llama session implementation")
	}
	if s.chatSyntax.Parser == "" {
		t.Fatal("no parser syntax captured for the typed tool set")
	}
	primaryAccepted := true
	if _, err := llamacppshim.ParseChatResponse(productionXMLCall, false, s.chatSyntax, "auto", true); err != nil {
		primaryAccepted = false
	}

	p := &chatOutputParser{syntax: s.chatSyntax, reasoningFormat: "auto", parseToolCalls: true}
	text, thinking, calls, err := p.Push(productionXMLCall, false)
	if err != nil {
		t.Fatalf("push (primary peg parse accepted=%v): %v", primaryAccepted, err)
	}
	if len(calls) != 1 {
		t.Fatalf("tool calls = %+v, want one", calls)
	}
	if calls[0].Function.Name != "local_fs.find_files" {
		t.Fatalf("name = %q", calls[0].Function.Name)
	}
	if want := `{"path":".","pattern":"*.go"}`; calls[0].Function.Arguments != want {
		t.Fatalf("arguments = %q, want %q", calls[0].Function.Arguments, want)
	}
	if !strings.Contains(text, "I'll help you analyze") {
		t.Fatalf("content = %q, want the prose before the call", text)
	}
	if !strings.Contains(thinking, "The user is asking about speed") {
		t.Fatalf("thinking = %q, want the reasoning split off the content", thinking)
	}
	t.Logf("recovered (primary peg parse accepted=%v): content=%q thinking_bytes=%d args=%s",
		primaryAccepted, text, len(thinking), calls[0].Function.Arguments)
}

// TestSystem_LlamaSession_ToolLessGrammarStillRecovers covers the other route into
// the same silence: a grammar rendered without tools (a session whose previous task
// ran tool-less) accepts the whole turn and files the call under content. The guard
// must still hand the call to the tool loop.
func TestSystem_LlamaSession_ToolLessGrammarStillRecovers(t *testing.T) {
	modelPath := qwen35ModelPath(t)

	sess, err := New(modelPath, llama.Config{NumCtx: 2048, NumBatch: 64, NumThreads: 4, DisableBOS: true, ReasoningFormat: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	stable := "system\nYou are a helpful assistant.\n"
	turn := "user\nWhich Go files are in this repository?\n"
	m := tinyManifest(stable, turn)
	if _, err := sess.EnsurePrefix(ctx, llama.PrefixInput{Text: stable, Manifest: m}); err != nil {
		t.Fatalf("EnsurePrefix: %v", err)
	}
	if _, err := sess.PrefillSuffix(ctx, llama.SuffixInput{Text: turn, Manifest: m}); err != nil {
		t.Fatalf("PrefillSuffix: %v", err)
	}

	s, ok := sess.(*session)
	if !ok {
		t.Fatal("session is not the llama session implementation")
	}
	p := &chatOutputParser{syntax: s.chatSyntax, reasoningFormat: "auto", parseToolCalls: true}
	text, _, calls, err := p.Push(productionXMLCall, false)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("tool calls = %+v, want the call rescued from the content", calls)
	}
	if strings.Contains(text, "<tool_call>") {
		t.Fatalf("the block leaked into the answer text: %q", text)
	}
	t.Logf("recovered from a tool-less grammar: content=%q args=%s", text, calls[0].Function.Arguments)
}
