package openvino

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func toolCallsJSONSchema(tools []modelrepo.Tool) (string, error) {
	var variants []any
	for _, tool := range tools {
		if tool.Function == nil || tool.Function.Name == "" {
			continue
		}
		variants = append(variants, toolCallVariantSchema(tool.Function.Name, parametersSchema(tool.Function.Parameters)))
	}
	if len(variants) == 0 {
		return "", fmt.Errorf("openvino: tool-call schema requires at least one function tool")
	}
	item := variants[0]
	if len(variants) > 1 {
		item = map[string]any{"oneOf": variants}
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool_calls": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    item,
			},
		},
		"required":             []string{"tool_calls"},
		"additionalProperties": false,
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("openvino: marshal tool-call JSON schema: %w", err)
	}
	return string(body), nil
}

func toolCallVariantSchema(name string, argsSchema any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
			"type": map[string]any{
				"type": "string",
				"enum": []string{"function"},
			},
			"function": map[string]any{
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
			},
		},
		"required":             []string{"function"},
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
