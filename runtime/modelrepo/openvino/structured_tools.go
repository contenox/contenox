package openvino

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func toolCallsJSONSchema(tools []modelrepo.Tool) (string, error) {
	var tags []any
	for _, tool := range tools {
		if tool.Function == nil || tool.Function.Name == "" {
			continue
		}
		tags = append(tags, map[string]any{
			"type":  "tag",
			"begin": "<tool_call>\n",
			"content": map[string]any{
				"type":        "json_schema",
				"json_schema": qwenToolCallSchema(tool.Function.Name, parametersSchema(tool.Function.Parameters)),
			},
			"end": "\n</tool_call>",
		})
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("openvino: tool-call schema requires at least one function tool")
	}

	// Qwen OpenVINO templates ask the model to emit tool calls as:
	// <tool_call>
	// {"name": "...", "arguments": {...}}
	// </tool_call>
	//
	// Triggered tags constrain only that tagged payload. Plain assistant text stays
	// unconstrained, so no-tool answers are not boxed into a full-response JSON
	// envelope that can be truncated by small max-token limits.
	triggered := map[string]any{
		"type":             "triggered_tags",
		"triggers":         []string{"<tool_call>"},
		"tags":             tags,
		"at_least_one":     false,
		"stop_after_first": false,
	}
	schema := map[string]any{
		"type":   "structural_tag",
		"format": triggered,
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("openvino: marshal tool-call JSON schema: %w", err)
	}
	return string(body), nil
}

func qwenToolCallSchema(name string, argsSchema any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
				"enum": []string{name},
			},
			"arguments": argsSchema,
		},
		"required":             []string{"name", "arguments"},
		"additionalProperties": false,
	}
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
