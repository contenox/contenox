package openvino

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const (
	protocolLlama3PythonicToolParser = "openvino:llama3_pythonic_tool_parser"
	protocolLlama3JSONToolParser     = "openvino:llama3_json_tool_parser"
	protocolVLLMParserWrapper        = "openvino:vllm_parser_wrapper"

	protocolReasoningParser       = "openvino:reasoning_parser"
	protocolDeepSeekR1Reasoning   = "openvino:deepseek_r1_reasoning_parser"
	protocolPhi4Reasoning         = "openvino:phi4_reasoning_parser"
	protocolReasoningIncremental  = "openvino:reasoning_incremental_parser"
	protocolDeepSeekR1Incremental = "openvino:deepseek_r1_reasoning_incremental_parser"
	protocolPhi4Incremental       = "openvino:phi4_reasoning_incremental_parser"

	protocolJSONSchema        = "openvino:json_schema"
	protocolRegex             = "openvino:regex"
	protocolEBNF              = "openvino:ebnf"
	protocolQwenXMLParameters = "openvino:qwen_xml_parameters"
	protocolStructuralTag     = "openvino:structural_tag"
	protocolTriggeredTags     = "openvino:triggered_tags"
	protocolTagsWithSeparator = "openvino:tags_with_separator"
	protocolConcat            = "openvino:concat"
	protocolUnion             = "openvino:union"
	protocolConstString       = "openvino:const_string"
	protocolAnyText           = "openvino:any_text"
)

var (
	toolParserProtocols = map[string]struct{}{
		protocolLlama3PythonicToolParser: {},
		protocolLlama3JSONToolParser:     {},
		// VLLMParserWrapper is exposed by the OpenVINO Python binding. The Go/C++
		// shim recognizes the protocol so profiles can be explicit, but the native
		// bridge returns an error until a Python parser object can be supplied.
		protocolVLLMParserWrapper: {},
	}

	reasoningParserProtocols = map[string]struct{}{
		protocolReasoningParser:       {},
		protocolDeepSeekR1Reasoning:   {},
		protocolPhi4Reasoning:         {},
		protocolReasoningIncremental:  {},
		protocolDeepSeekR1Incremental: {},
		protocolPhi4Incremental:       {},
		protocolVLLMParserWrapper:     {},
	}

	structuredOutputProtocols = map[string]struct{}{
		protocolJSONSchema:        {},
		protocolRegex:             {},
		protocolEBNF:              {},
		protocolQwenXMLParameters: {},
		protocolStructuralTag:     {},
		protocolTriggeredTags:     {},
		protocolTagsWithSeparator: {},
		protocolConcat:            {},
		protocolUnion:             {},
		protocolConstString:       {},
		protocolAnyText:           {},
	}
)

func validateToolProtocol(protocol string) error {
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		return nil
	}
	if _, ok := toolParserProtocols[protocol]; ok {
		return nil
	}
	return fmt.Errorf("unsupported protocol %q", protocol)
}

func validateReasoningProtocol(protocol string) error {
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		return nil
	}
	if _, ok := reasoningParserProtocols[protocol]; ok {
		return nil
	}
	return fmt.Errorf("unsupported protocol %q", protocol)
}

func isStructuredOutputProtocol(protocol string) bool {
	_, ok := structuredOutputProtocols[strings.TrimSpace(protocol)]
	return ok
}

type parsedGeneration struct {
	content  string
	thinking string
	calls    []modelrepo.ToolCall
}

func decodeParsedGeneration(raw string) (parsedGeneration, error) {
	if strings.TrimSpace(raw) == "" {
		return parsedGeneration{}, nil
	}
	var msg struct {
		Content          string            `json:"content"`
		ReasoningContent string            `json:"reasoning_content"`
		Reasoning        string            `json:"reasoning"`
		ToolCalls        []json.RawMessage `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return parsedGeneration{}, fmt.Errorf("openvino: decode parser output: %w", err)
	}

	out := parsedGeneration{
		content:  msg.Content,
		thinking: msg.ReasoningContent,
	}
	if out.thinking == "" {
		out.thinking = msg.Reasoning
	}

	for i, rawCall := range msg.ToolCalls {
		tc, err := decodeOpenVINOParsedToolCall(rawCall, i+1)
		if err != nil {
			return parsedGeneration{}, err
		}
		out.calls = append(out.calls, tc)
	}
	return out, nil
}

func decodeOpenVINOParsedToolCall(raw json.RawMessage, seq int) (modelrepo.ToolCall, error) {
	var direct struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Name != "" {
		return newOpenVINOToolCall(seq, direct.Name, direct.Arguments), nil
	}

	var wrapped struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Arguments  json.RawMessage `json:"arguments"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Function.Name != "" {
		args := wrapped.Function.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = wrapped.Function.Parameters
		}
		return newOpenVINOToolCall(seq, wrapped.Function.Name, args), nil
	}

	return modelrepo.ToolCall{}, errors.New("openvino: parser returned unsupported tool_call shape")
}

func newOpenVINOToolCall(seq int, name string, args json.RawMessage) modelrepo.ToolCall {
	argText := strings.TrimSpace(string(args))
	if argText == "" || argText == "null" {
		argText = "{}"
	}
	tc := modelrepo.ToolCall{
		ID:   fmt.Sprintf("call_%d", seq),
		Type: "function",
	}
	tc.Function.Name = name
	tc.Function.Arguments = argText
	return tc
}
