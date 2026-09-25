package openvino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/modeld/openvino/ovsession"
	"github.com/contenox/contenox/internal/transport"
)

func TestProfileToolConfig(t *testing.T) {
	cfg := transport.DecodeConfig{ParserProtocols: []string{"openvino:json_schema_tool_calls", "openvino:reasoning_parser"}}
	for _, tools := range []string{"", "[]", "null"} {
		got, err := resolveToolConfig(cfg, tools)
		if err != nil || got.StructuredOutput.Protocol != "" || len(got.ParserProtocols) != 1 || got.ParserProtocols[0] != "openvino:reasoning_parser" {
			t.Fatalf("no tools: %+v, %v", got, err)
		}
	}
	for _, tools := range []string{`broken`, `[{"type":"function","function":{}}]`} {
		if _, err := resolveToolConfig(cfg, tools); err == nil {
			t.Fatalf("accepted invalid tools %s", tools)
		}
	}
	tools := `[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}}}]`
	got, err := resolveToolConfig(cfg, tools)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Type       string `json:"type"`
		AtLeastOne bool   `json:"at_least_one"`
		Tags       []struct {
			Begin   string `json:"begin"`
			End     string `json:"end"`
			Content struct {
				Schema struct {
					AnyOf []struct {
						Properties struct {
							Name struct {
								Const string `json:"const"`
							} `json:"name"`
							Arguments struct {
								Required []string `json:"required"`
							} `json:"arguments"`
						} `json:"properties"`
					} `json:"anyOf"`
				} `json:"json_schema"`
			} `json:"content"`
		} `json:"tags"`
	}
	var wrapper struct {
		Type   string          `json:"type"`
		Format json.RawMessage `json:"format"`
	}
	if err := json.Unmarshal([]byte(got.StructuredOutput.Payload), &wrapper); err != nil {
		t.Fatal(err)
	}
	if wrapper.Type != "structural_tag" {
		t.Fatalf("wrapper: %+v", wrapper)
	}
	if err := json.Unmarshal(wrapper.Format, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "triggered_tags" || payload.AtLeastOne || len(payload.Tags) != 1 {
		t.Fatalf("payload: %+v", payload)
	}
	tag := payload.Tags[0]
	if tag.Begin != "<tool_call>" || tag.End != "</tool_call>" || len(tag.Content.Schema.AnyOf) != 1 {
		t.Fatalf("tag: %+v", tag)
	}
	props := tag.Content.Schema.AnyOf[0].Properties
	if props.Name.Const != "lookup" || len(props.Arguments.Required) != 1 || props.Arguments.Required[0] != "key" {
		t.Fatalf("properties: %+v", props)
	}
	cfg.StructuredOutput = transport.StructuredOutputConfig{Protocol: "explicit", Payload: "unchanged"}
	got, err = resolveToolConfig(cfg, tools)
	if err != nil || got.StructuredOutput != cfg.StructuredOutput {
		t.Fatalf("explicit config overwritten: %+v, %v", got, err)
	}
}

func TestProfileToolDecode(t *testing.T) {
	fake := &fakeGenAIBackend{generateResult: ovsession.GenAIResult{Text: `<tool_call>{"name":"lookup","arguments":{"key":"x"}}</tool_call>`}}
	fake.generateResult.Metrics.UsageKnown = true
	fake.generateResult.Metrics.PromptTokens = 25
	fake.generateResult.Metrics.CompletionTokens = 12
	s := newGenaiSession(fake, 4096)
	s.parserProtocols = []string{"openvino:json_schema_tool_calls"}
	_, err := s.EnsurePrefix(context.Background(), transport.PrefixInput{Text: "USER", Manifest: ovManifest("hash", "r1"), Tools: `[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := s.Decode(context.Background(), transport.DecodeConfig{MaxTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for chunk := range stream {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		calls += len(chunk.ToolCalls)
		if chunk.Usage == nil || chunk.Usage.PromptTokens != 25 || chunk.Usage.CompletionTokens != 12 {
			t.Fatalf("lost usage: %+v", chunk)
		}
	}
	if calls != 1 || len(fake.generateOptions) != 1 {
		t.Fatalf("calls=%d, options=%+v", calls, fake.generateOptions)
	}
	opts := fake.generateOptions[0]
	if len(opts.ParserProtocols) != 0 || opts.StructuredOutput.Protocol != "openvino:triggered_tags" {
		t.Fatalf("wrong native routing: %+v", opts)
	}
}

func TestStructuredToolsAfterReasoningParser(t *testing.T) {
	result := ovsession.GenAIResult{ParsedJSON: `{"content":"<tool_call>{\"name\":\"lookup\",\"arguments\":{}}</tool_call>","reasoning_content":"Need a lookup"}`}
	result.Metrics.UsageKnown = true
	result.Metrics.PromptTokens = 20
	chunk, err := chunkFromGenAIResult(result, transport.StructuredOutputConfig{Protocol: "openvino:json_schema_tool_calls"})
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Text != "" || chunk.Thinking != "Need a lookup" || len(chunk.ToolCalls) != 1 || chunk.Usage == nil || chunk.Usage.PromptTokens != 20 {
		t.Fatalf("chunk: %+v", chunk)
	}
}

// productionXMLCall is the shape a Qwen 3.x template documents and the model emits:
// reasoning, content, then the XML tool call.
const productionXMLCall = "The user is asking about speed with their codebase.\n</think>\n\n" +
	"I'll help you analyze it.\n\n" +
	"<tool_call>\n<function=local_fs.find_files>\n" +
	"<parameter=path>\n.\n</parameter>\n<parameter=pattern>\n*.go\n</parameter>\n" +
	"</function>\n</tool_call>"

// TestUnit_ChunkFromGenAIResult_UnstructuredTextRescuesToolCall pins the rule the
// llama adapter already follows: when a decode asked for tool output and GenAI hands
// back text carrying a tool-call block, the block is read — not returned as prose that
// nobody executed and nobody reported (the pre-fix behaviour, observed live).
func TestUnit_ChunkFromGenAIResult_UnstructuredTextRescuesToolCall(t *testing.T) {
	chunk, err := chunkFromGenAIResult(ovsession.GenAIResult{Text: productionXMLCall}, transport.StructuredOutputConfig{})
	if err != nil {
		t.Fatalf("chunkFromGenAIResult: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want the block rescued", chunk.ToolCalls)
	}
	if got := chunk.ToolCalls[0].Function.Name; got != "local_fs.find_files" {
		t.Fatalf("name = %q", got)
	}
	if strings.Contains(chunk.Text, "<tool_call>") {
		t.Fatalf("the block leaked into the text: %q", chunk.Text)
	}
}

// TestUnit_ChunkFromGenAIResult_ProseWithoutBlockPassesThrough keeps the guard off
// ordinary answers.
func TestUnit_ChunkFromGenAIResult_ProseWithoutBlockPassesThrough(t *testing.T) {
	chunk, err := chunkFromGenAIResult(ovsession.GenAIResult{Text: "No tool call here."}, transport.StructuredOutputConfig{})
	if err != nil {
		t.Fatalf("chunkFromGenAIResult: %v", err)
	}
	if chunk.Text != "No tool call here." || len(chunk.ToolCalls) != 0 {
		t.Fatalf("chunk = %+v", chunk)
	}
}

// TestUnit_ChunkFromGenAIResult_UnreadableBlockFails pins the other half: a block no
// dialect reader can read fails the turn with a nameable error.
func TestUnit_ChunkFromGenAIResult_UnreadableBlockFails(t *testing.T) {
	_, err := chunkFromGenAIResult(ovsession.GenAIResult{Text: "example: <tool_call>not a call</tool_call>"}, transport.StructuredOutputConfig{})
	if !errors.Is(err, errUnreadableToolCall) {
		t.Fatalf("error = %v, want errUnreadableToolCall", err)
	}
}

// TestUnit_ChunkFromGenAIResult_StructuredXMLDialectRescued is the other half of the
// probe: once tools are requested through the structured protocol, the dialect reader
// handles the XML form the template documents, so the call is extracted.
func TestUnit_ChunkFromGenAIResult_StructuredXMLDialectRescued(t *testing.T) {
	chunk, err := chunkFromGenAIResult(ovsession.GenAIResult{Text: productionXMLCall}, transport.StructuredOutputConfig{Protocol: "openvino:json_schema_tool_calls"})
	if err != nil {
		t.Fatalf("structured path: %v", err)
	}
	t.Logf("structured protocol: tool_calls=%d text_has_block=%t", len(chunk.ToolCalls), strings.Contains(chunk.Text, "<tool_call>"))
	for _, c := range chunk.ToolCalls {
		t.Logf("  call %s args=%s", c.Function.Name, c.Function.Arguments)
	}
}

// TestUnit_ChunkFromGenAIResult_TruncatedBlockFails pins the same rule as the llama
// adapter for a block that never closed.
func TestUnit_ChunkFromGenAIResult_TruncatedBlockFails(t *testing.T) {
	_, err := chunkFromGenAIResult(ovsession.GenAIResult{Text: "I'll stage them.\n\n<tool_call>\n<function=native-git.git_add>\n<parameter=paths>"}, transport.StructuredOutputConfig{})
	if !errors.Is(err, errUnreadableToolCall) {
		t.Fatalf("error = %v, want errUnreadableToolCall", err)
	}
}
