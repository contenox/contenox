package llama

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// toolCallsJSONSchema builds the JSON-schema payload for the
// llama:json_schema_tool_calls protocol: modeld converts it to a GBNF grammar
// (llama.cpp common's json_schema_to_grammar) that constrains decoding to the
// envelope sessionkit.StructuredToolCallChunk parses:
//
//	{"tool_calls":[{"name":"<tool>","arguments":{…}}, …]}
//
// Unlike the OpenVINO structural-tags payload, GBNF constrains the entire
// generation, so this protocol suits forced-tool-call turns; free-text
// answers use the model-native parser protocol instead.
func toolCallsJSONSchema(tools []modelrepo.Tool) (string, error) {
	var calls []any
	for _, tool := range tools {
		if tool.Function == nil || tool.Function.Name == "" {
			continue
		}
		calls = append(calls, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
					"enum": []string{tool.Function.Name},
				},
				"arguments": parametersSchema(tool.Function.Parameters),
			},
			"required":             []string{"name", "arguments"},
			"additionalProperties": false,
		})
	}
	if len(calls) == 0 {
		return "", fmt.Errorf("llama: tool-call schema requires at least one function tool")
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool_calls": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]any{"anyOf": calls},
			},
		},
		"required":             []string{"tool_calls"},
		"additionalProperties": false,
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("llama: marshal tool-call JSON schema: %w", err)
	}
	return string(body), nil
}

func parametersSchema(in any) any {
	if in == nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	return in
}
