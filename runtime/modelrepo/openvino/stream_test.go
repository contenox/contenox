package openvino

import (
	"context"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

type fakeStreamSession struct {
	prefix transport.PrefixInput
	suffix transport.SuffixInput
	decode transport.DecodeConfig
	chunks []transport.StreamChunk
	closed bool
}

func (s *fakeStreamSession) EnsurePrefix(_ context.Context, prefix transport.PrefixInput) (transport.PrefixStatus, error) {
	s.prefix = prefix
	return transport.PrefixStatus{}, nil
}

func (s *fakeStreamSession) PrefillSuffix(_ context.Context, suffix transport.SuffixInput) (transport.SuffixStatus, error) {
	s.suffix = suffix
	return transport.SuffixStatus{}, nil
}

func (s *fakeStreamSession) Decode(_ context.Context, cfg transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	s.decode = cfg
	out := make(chan transport.StreamChunk, len(s.chunks))
	for _, chunk := range s.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

func (s *fakeStreamSession) ExplainContext() transport.ContextReport {
	return transport.ContextReport{}
}
func (s *fakeStreamSession) Snapshot(context.Context) (transport.SessionSnapshot, error) {
	return transport.SessionSnapshot{}, nil
}
func (s *fakeStreamSession) Restore(context.Context, transport.SessionSnapshot) error { return nil }
func (s *fakeStreamSession) Close() error {
	s.closed = true
	return nil
}

func resetOpenVINOStreamTest(t *testing.T) {
	t.Helper()
	oldFactory := sessionFactory
	oldWarm := warm
	t.Cleanup(func() {
		sessionFactory = oldFactory
		warm.Clear()
		warm = oldWarm
	})
	warm = modelrepo.NewWarmCache[Session]()
}

func TestUnit_OpenVINOStream_ToolsComeFromModeldChunks(t *testing.T) {
	resetOpenVINOStreamTest(t)

	var call transport.ToolCall
	call.ID = "call_1"
	call.Type = "function"
	call.Function.Name = "lookup_weather"
	call.Function.Arguments = `{"city":"Berlin"}`
	fake := &fakeStreamSession{
		chunks: []transport.StreamChunk{
			{Text: "preface", ToolCalls: []transport.ToolCall{call}},
		},
	}
	sessionFactory = func(ref modeldconn.ModelRef, cfg Config) (Session, error) {
		if ref.Type != "openvino" || ref.Name != "qwen-ov" {
			t.Fatalf("session ref = %+v", ref)
		}
		return fake, nil
	}

	client := &client{
		modelName:      "qwen-ov",
		modelPath:      "/models/qwen-ov",
		modelDigest:    "digest",
		backendVersion: "OpenVINO GenAI@test",
		cfg:            Config{NumCtx: 4096, PromptTemplateDigest: "tmpl"},
		toolProtocol:   "openvino:llama3_json_tool_parser",
	}
	stream, err := client.Stream(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "weather?"},
	}, modelrepo.WithTool(modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name:        "lookup_weather",
			Description: "lookup weather",
		},
	}))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var parcels []*modelrepo.StreamParcel
	for parcel := range stream {
		if parcel.Error != nil {
			t.Fatalf("stream error: %v", parcel.Error)
		}
		parcels = append(parcels, parcel)
	}
	if len(parcels) != 1 {
		t.Fatalf("parcels = %d, want final parsed parcel", len(parcels))
	}
	if parcels[0].Data != "preface" {
		t.Fatalf("visible content = %q, want parsed content", parcels[0].Data)
	}
	if len(parcels[0].ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v, want 1", parcels[0].ToolCalls)
	}
	gotCall := parcels[0].ToolCalls[0]
	if gotCall.Function.Name != "lookup_weather" || gotCall.Function.Arguments != `{"city":"Berlin"}` {
		t.Fatalf("tool call = %+v", gotCall)
	}
	if fake.prefix.Tools == "" || !strings.Contains(fake.prefix.Tools, "lookup_weather") {
		t.Fatalf("tool definitions were not sent on the stable prefix: %s", fake.prefix.Tools)
	}
	if fake.suffix.Manifest.PromptTemplateDigest != "tmpl" || fake.suffix.Manifest.BackendVersion != "OpenVINO GenAI@test" {
		t.Fatalf("suffix manifest identity incomplete: %+v", fake.suffix.Manifest)
	}
	if got := fake.decode.ParserProtocols; len(got) != 1 || got[0] != "openvino:llama3_json_tool_parser" {
		t.Fatalf("parser protocols = %+v", got)
	}
}

func TestUnit_OpenVINOChat_ToolsPopulateTopLevelResult(t *testing.T) {
	resetOpenVINOStreamTest(t)

	var call transport.ToolCall
	call.ID = "call_1"
	call.Type = "function"
	call.Function.Name = "echo.echo"
	call.Function.Arguments = `{"input":"hello"}`
	fake := &fakeStreamSession{
		chunks: []transport.StreamChunk{{ToolCalls: []transport.ToolCall{call}}},
	}
	sessionFactory = func(ref modeldconn.ModelRef, cfg Config) (Session, error) {
		return fake, nil
	}

	client := &client{
		modelName:      "qwen-ov",
		modelPath:      "/models/qwen-ov",
		modelDigest:    "digest",
		backendVersion: "OpenVINO GenAI@test",
		cfg:            Config{NumCtx: 4096, PromptTemplateDigest: "tmpl"},
		toolProtocol:   "openvino:json_schema_tool_calls",
	}
	res, err := client.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "echo"}}, modelrepo.WithTool(modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name:        "echo.echo",
			Description: "echo",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
				"required": []string{"input"},
			},
		},
	}))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ID != "call_1" || res.ToolCalls[0].Function.Name != "echo.echo" {
		t.Fatalf("top-level tool calls = %+v", res.ToolCalls)
	}
	if len(res.Message.ToolCalls) != 1 || res.Message.ToolCalls[0].ID != "call_1" || res.Message.ToolCalls[0].Function.Name != "echo.echo" {
		t.Fatalf("message tool calls = %+v", res.Message.ToolCalls)
	}
}

func TestUnit_OpenVINOStream_JSONSchemaToolProtocolUsesStructuredOutput(t *testing.T) {
	resetOpenVINOStreamTest(t)

	fake := &fakeStreamSession{
		chunks: []transport.StreamChunk{
			{ToolCalls: []transport.ToolCall{func() transport.ToolCall {
				var call transport.ToolCall
				call.ID = "call_1"
				call.Type = "function"
				call.Function.Name = "echo.echo"
				call.Function.Arguments = `{"input":"hello"}`
				return call
			}()}},
		},
	}
	sessionFactory = func(ref modeldconn.ModelRef, cfg Config) (Session, error) {
		return fake, nil
	}

	client := &client{
		modelName:      "qwen-ov",
		modelPath:      "/models/qwen-ov",
		modelDigest:    "digest",
		backendVersion: "OpenVINO GenAI@test",
		cfg:            Config{NumCtx: 4096, PromptTemplateDigest: "tmpl"},
		toolProtocol:   "openvino:json_schema_tool_calls",
	}
	stream, err := client.Stream(context.Background(), []modelrepo.Message{{Role: "user", Content: "echo"}}, modelrepo.WithTool(modelrepo.Tool{
		Type: "function",
		Function: &modelrepo.FunctionTool{
			Name:        "echo.echo",
			Description: "echo",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
				"required": []string{"input"},
			},
		},
	}))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for parcel := range stream {
		if parcel.Error != nil {
			t.Fatalf("stream error: %v", parcel.Error)
		}
	}
	if fake.decode.StructuredOutput.Protocol != "openvino:json_schema_tool_calls" {
		t.Fatalf("structured protocol = %q", fake.decode.StructuredOutput.Protocol)
	}
	if !strings.Contains(fake.decode.StructuredOutput.Payload, `"echo.echo"`) || !strings.Contains(fake.decode.StructuredOutput.Payload, `"tool_calls"`) {
		t.Fatalf("structured payload missing tool schema: %s", fake.decode.StructuredOutput.Payload)
	}
	if len(fake.decode.ParserProtocols) != 0 {
		t.Fatalf("parser protocols = %+v, want none for structured tool protocol", fake.decode.ParserProtocols)
	}
}

func TestUnit_OpenVINOStream_QwenXMLParametersProtocolIsRejected(t *testing.T) {
	resetOpenVINOStreamTest(t)

	client := &client{
		modelName:    "qwen-ov",
		modelPath:    "/models/qwen-ov",
		modelDigest:  "digest",
		cfg:          Config{NumCtx: 4096},
		toolProtocol: "openvino:qwen_xml_parameters",
	}
	_, err := client.Stream(context.Background(), []modelrepo.Message{{Role: "user", Content: "echo"}}, modelrepo.WithTools(
		modelrepo.Tool{Type: "function", Function: &modelrepo.FunctionTool{Name: "echo.echo"}},
	))
	if err == nil || !strings.Contains(err.Error(), `tool protocol "openvino:qwen_xml_parameters"`) {
		t.Fatalf("error = %v, want unsupported qwen XML protocol", err)
	}
}

func TestUnit_OpenVINOStream_ReasoningParserCleansContentAndSurfacesThinkingWhenRequested(t *testing.T) {
	resetOpenVINOStreamTest(t)

	fake := &fakeStreamSession{
		chunks: []transport.StreamChunk{
			{Thinking: "hidden "},
			{Text: "visible"},
		},
	}
	sessionFactory = func(ref modeldconn.ModelRef, cfg Config) (Session, error) {
		return fake, nil
	}

	client := &client{
		modelName:       "deepseek-ov",
		modelPath:       "/models/deepseek-ov",
		modelDigest:     "digest",
		backendVersion:  "OpenVINO GenAI@test",
		cfg:             Config{NumCtx: 4096, PromptTemplateDigest: "tmpl"},
		reasoningStream: "openvino:deepseek_r1_reasoning_incremental_parser",
	}
	stream, err := client.Stream(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "why?"},
	}, modelrepo.WithThink("high"))
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
	if text != "visible" || thinking != "hidden " {
		t.Fatalf("stream text/thinking = %q/%q", text, thinking)
	}
	if got := fake.decode.ParserProtocols; len(got) != 1 || got[0] != "openvino:deepseek_r1_reasoning_incremental_parser" {
		t.Fatalf("parser protocols = %+v", got)
	}
}

func TestUnit_OpenVINOStream_ReasoningParserStillRunsWhenThinkOff(t *testing.T) {
	resetOpenVINOStreamTest(t)

	fake := &fakeStreamSession{
		chunks: []transport.StreamChunk{
			{Thinking: "hidden "},
			{Text: "visible"},
		},
	}
	sessionFactory = func(ref modeldconn.ModelRef, cfg Config) (Session, error) {
		return fake, nil
	}

	client := &client{
		modelName:       "deepseek-ov",
		modelPath:       "/models/deepseek-ov",
		modelDigest:     "digest",
		backendVersion:  "OpenVINO GenAI@test",
		cfg:             Config{NumCtx: 4096, PromptTemplateDigest: "tmpl"},
		reasoningStream: "openvino:deepseek_r1_reasoning_incremental_parser",
	}
	stream, err := client.Stream(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "why?"},
	}, modelrepo.WithThink("off"))
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
	if text != "visible" || thinking != "" {
		t.Fatalf("stream text/thinking = %q/%q, want visible/no displayed thinking", text, thinking)
	}
	if got := fake.decode.ParserProtocols; len(got) != 1 || got[0] != "openvino:deepseek_r1_reasoning_incremental_parser" {
		t.Fatalf("parser protocols = %+v", got)
	}
}
