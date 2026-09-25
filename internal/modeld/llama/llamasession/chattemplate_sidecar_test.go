//go:build llamanode && llamacpp_direct

package llamasession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/modeld/llama/llamacppshim"
)

// The templates under testdata are the ones the model publishers ship, kept
// here because the GGUF quants of those models carry no tokenizer.chat_template
// at all: nanbeige4-3b-thinking.jinja from Nanbeige/Nanbeige4-3B-Thinking-2511
// (tokenizer_config.json, Hermes JSON inside <tool_call>) and minicpm5-2b.jinja
// from openbmb/MiniCPM5-2B (chat_template.jinja, attribute-style
// <function name="…"><param name="…">). Both are apache-2.0.
func templateFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read template fixture %s: %v", name, err)
	}
	return string(data)
}

func requireTinyModel(t *testing.T) string {
	t.Helper()
	modelPath := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	requireTinyGGUF(t, modelPath)
	return modelPath
}

const sidecarTools = `[{"type":"function","function":{"name":"git_status","description":"Report the working tree status","parameters":{"type":"object","properties":{"short":{"type":"boolean"}},"required":["short"]}}}]`

// TestSystem_ChatTemplateSidecar_DerivesAToolParser pins what the sidecar is
// for: llama.cpp builds the tool-call parser from the template it is given, so a
// GGUF that lost tokenizer.chat_template still gets a parser that recognises the
// dialect its publisher trained it on — instead of the built-in ChatML fallback,
// which declares no call markers and lets every call through as prose.
func TestSystem_ChatTemplateSidecar_DerivesAToolParser(t *testing.T) {
	modelPath := requireTinyModel(t)
	model, err := llamacppshim.LoadModel(modelPath, llamacppshim.ModelConfig{})
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	defer model.Close()

	messages := `[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"What is the working tree status?"}]`

	for _, tc := range []struct {
		name     string
		template string
		call     string
	}{
		{
			name:     "nanbeige (Hermes JSON in <tool_call>)",
			template: templateFixture(t, "nanbeige4-3b-thinking.jinja"),
			call:     "<tool_call>\n{\"name\": \"git_status\", \"arguments\": {\"short\": true}}\n</tool_call>",
		},
		{
			name:     "minicpm5 (attribute-style <function name=…>)",
			template: templateFixture(t, "minicpm5-2b.jinja"),
			call:     "<function name=\"git_status\"><param name=\"short\">true</param></function>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := model.ApplyChatTemplateCommonWithOptions(messages, sidecarTools, llamacppshim.ChatTemplateOptions{
				AddAssistant: true,
				ChatTemplate: tc.template,
			})
			if err != nil {
				t.Fatalf("apply template: %v", err)
			}
			if !strings.Contains(rendered.Prompt, "git_status") {
				t.Fatalf("the tool definition never reached the prompt:\n%s", rendered.Prompt)
			}
			if strings.TrimSpace(rendered.Syntax.Parser) == "" {
				t.Fatal("no parser was derived from the template, so no call can be read back")
			}

			parsed, err := llamacppshim.ParseChatResponse(tc.call, false, rendered.Syntax, "", true)
			if err != nil {
				t.Fatalf("parse a call in this dialect: %v", err)
			}
			if !strings.Contains(parsed.ToolCallsJSON, "git_status") {
				t.Fatalf("tool calls = %q, want the call read back out of %q", parsed.ToolCallsJSON, tc.call)
			}
			if strings.Contains(parsed.Content, "git_status") {
				t.Fatalf("the call was also delivered as content: %q", parsed.Content)
			}
		})
	}
}

const parallelCallTools = `[` +
	`{"type":"function","function":{"name":"git_status","description":"Report the working tree status","parameters":{"type":"object","properties":{"short":{"type":"boolean"}},"required":["short"]}}},` +
	`{"type":"function","function":{"name":"local_shell","description":"Run a command","parameters":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}}` +
	`]`

// TestSystem_ChatTemplateSidecar_ParsesEveryCallInOneReply pins the multi-call
// contract: both publisher templates tell the model it may return zero or more
// calls in one reply, and MiniCPM5 returns exactly that on its first turn — two
// calls, in one message. llama.cpp types the tool-call rule as exactly one call
// unless the render asks for the list form, and the single-call rule does not
// merely drop the second call: the whole parse fails with "does not match the
// expected peg-native format" and the turn is lost.
func TestSystem_ChatTemplateSidecar_ParsesEveryCallInOneReply(t *testing.T) {
	modelPath := requireTinyModel(t)
	model, err := llamacppshim.LoadModel(modelPath, llamacppshim.ModelConfig{})
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	defer model.Close()

	messages := `[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"What changed here?"}]`

	for _, tc := range []struct {
		name     string
		template string
		reply    string
	}{
		{
			name:     "nanbeige (Hermes JSON in <tool_call>)",
			template: templateFixture(t, "nanbeige4-3b-thinking.jinja"),
			reply: "<tool_call>\n{\"name\": \"git_status\", \"arguments\": {\"short\": true}}\n</tool_call>\n" +
				"<tool_call>\n{\"name\": \"local_shell\", \"arguments\": {\"command\": \"git log --oneline -10\"}}\n</tool_call>",
		},
		{
			name:     "minicpm5 (attribute-style <function name=…>)",
			template: templateFixture(t, "minicpm5-2b.jinja"),
			reply: "I should look at the working tree and the recent history.\n</think>\n\n" +
				"<function name=\"git_status\"><param name=\"short\">true</param></function>\n" +
				"<function name=\"local_shell\"><param name=\"command\"><![CDATA[git log --oneline -10]]></param></function>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := model.ApplyChatTemplateCommonWithOptions(messages, parallelCallTools, llamacppshim.ChatTemplateOptions{
				AddAssistant:    true,
				ChatTemplate:    tc.template,
				EnableThinking:  true,
				ReasoningFormat: "auto",
			})
			if err != nil {
				t.Fatalf("apply template: %v", err)
			}

			parsed, err := llamacppshim.ParseChatResponse(tc.reply, false, rendered.Syntax, "auto", true)
			if err != nil {
				t.Fatalf("parse a two-call reply in this dialect: %v", err)
			}
			for _, want := range []string{"git_status", "local_shell"} {
				if !strings.Contains(parsed.ToolCallsJSON, want) {
					t.Fatalf("tool calls = %q, want %s among them", parsed.ToolCallsJSON, want)
				}
			}
			if strings.Contains(parsed.Content, "git_status") || strings.Contains(parsed.Content, "local_shell") {
				t.Fatalf("a call was also delivered as content: %q", parsed.Content)
			}
		})
	}
}

// TestSystem_ChatTemplateSidecar_SplitsReasoningFromContent pins the other half
// of the same render: with a reasoning format set, a thinking model's chain of
// thought leaves the render as reasoning and never reaches content — which is
// what keeps it out of the assistant turn the model reads back. An unset format
// bakes a parser with no reasoning rule, and the same reply then carries the
// chain of thought as its answer.
func TestSystem_ChatTemplateSidecar_SplitsReasoningFromContent(t *testing.T) {
	modelPath := requireTinyModel(t)
	model, err := llamacppshim.LoadModel(modelPath, llamacppshim.ModelConfig{})
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	defer model.Close()

	messages := `[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"What is the working tree status?"}]`
	reply := "I should check the working tree.\n</think>\n\n<function name=\"git_status\"><param name=\"short\">true</param></function>"

	for _, tc := range []struct {
		name        string
		format      string
		wantThink   bool
		wantContent string
	}{
		{name: "extracting", format: "auto", wantThink: true, wantContent: ""},
		{name: "not extracting", format: "", wantThink: false, wantContent: "</think>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := model.ApplyChatTemplateCommonWithOptions(messages, sidecarTools, llamacppshim.ChatTemplateOptions{
				AddAssistant:    true,
				ChatTemplate:    templateFixture(t, "minicpm5-2b.jinja"),
				EnableThinking:  true,
				ReasoningFormat: tc.format,
			})
			if err != nil {
				t.Fatalf("apply template: %v", err)
			}
			parsed, err := llamacppshim.ParseChatResponse(reply, false, rendered.Syntax, tc.format, true)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := strings.Contains(parsed.Thinking, "check the working tree"); got != tc.wantThink {
				t.Fatalf("reasoning extracted = %t, want %t (thinking=%q)", got, tc.wantThink, parsed.Thinking)
			}
			if tc.wantThink && strings.Contains(parsed.Content, "check the working tree") {
				t.Fatalf("the chain of thought also reached content: %q", parsed.Content)
			}
			if tc.wantContent != "" && !strings.Contains(parsed.Content, tc.wantContent) {
				t.Fatalf("content = %q, want it to carry %q", parsed.Content, tc.wantContent)
			}
		})
	}
}
