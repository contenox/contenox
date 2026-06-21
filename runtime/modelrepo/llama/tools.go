package llama

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const (
	toolParserProtocolCommonChat = "llama:common_chat_tool_parser"
)

func serializeToolDefs(tools []modelrepo.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	wire := make([]llamaToolDef, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" || tool.Function == nil {
			continue
		}
		wire = append(wire, llamaToolDef{
			Type: "function",
			Function: llamaFunctionToolDef{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  normalizeLlamaToolParameters(tool.Function.Parameters),
			},
		})
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("llama: serialize tool definitions: %w", err)
	}
	return string(b), nil
}

type llamaToolDef struct {
	Type     string               `json:"type"`
	Function llamaFunctionToolDef `json:"function"`
}

type llamaFunctionToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

func normalizeLlamaToolParameters(params any) any {
	if params == nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	return normalizeLlamaSchema(params, true)
}

func normalizeLlamaSchema(in any, root bool) any {
	switch v := in.(type) {
	case nil:
		return map[string]any{"type": "value"}
	case bool:
		if root {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			}
		}
		return map[string]any{"type": "value"}
	case map[string]any:
		return normalizeLlamaSchemaMap(v, root)
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = normalizeLlamaSchema(val, false)
		}
		return out
	default:
		raw, err := json.Marshal(in)
		if err != nil {
			return map[string]any{"type": "value"}
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return map[string]any{"type": "value"}
		}
		switch decoded.(type) {
		case map[string]any, []any, bool:
			return normalizeLlamaSchema(decoded, root)
		default:
			return map[string]any{"type": "value"}
		}
	}
}

func normalizeLlamaSchemaMap(schema map[string]any, root bool) map[string]any {
	out := make(map[string]any, len(schema)+1)
	for k, v := range schema {
		out[k] = normalizeLlamaSchemaField(k, v)
	}

	if nullable, ok := out["nullable"].(bool); ok && nullable {
		if typeValue, ok := out["type"]; ok {
			out["type"] = addNullToLlamaType(typeValue)
		}
	}

	if _, ok := out["type"]; !ok && needsExplicitLlamaType(out, root) {
		out["type"] = inferLlamaSchemaType(out, root)
	}
	if typeValue, ok := out["type"]; ok {
		out["type"] = normalizeLlamaType(typeValue)
	}
	return out
}

func normalizeLlamaSchemaField(key string, value any) any {
	switch key {
	case "properties", "$defs", "definitions", "dependentSchemas":
		if props, ok := value.(map[string]any); ok {
			out := make(map[string]any, len(props))
			for name, schema := range props {
				out[name] = normalizeLlamaSchema(schema, false)
			}
			return out
		}
		return value
	case "additionalProperties", "items", "contains", "propertyNames", "not", "if", "then", "else", "unevaluatedProperties":
		if _, ok := value.(bool); ok {
			return value
		}
		return normalizeLlamaSchema(value, false)
	case "prefixItems", "oneOf", "anyOf", "allOf":
		if values, ok := value.([]any); ok {
			out := make([]any, len(values))
			for i, schema := range values {
				out[i] = normalizeLlamaSchema(schema, false)
			}
			return out
		}
		return value
	default:
		return value
	}
}

func needsExplicitLlamaType(schema map[string]any, root bool) bool {
	if root {
		return true
	}
	for _, key := range []string{"$ref", "oneOf", "anyOf", "allOf", "const", "enum"} {
		if _, ok := schema[key]; ok {
			return false
		}
	}
	return true
}

func inferLlamaSchemaType(schema map[string]any, root bool) any {
	if root {
		return "object"
	}
	if _, ok := schema["properties"]; ok {
		return "object"
	}
	if _, ok := schema["additionalProperties"]; ok {
		return "object"
	}
	if _, ok := schema["items"]; ok {
		return "array"
	}
	if _, ok := schema["prefixItems"]; ok {
		return "array"
	}
	for _, key := range []string{"pattern", "format", "minLength", "maxLength"} {
		if _, ok := schema[key]; ok {
			return "string"
		}
	}
	for _, key := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf"} {
		if _, ok := schema[key]; ok {
			return "number"
		}
	}
	return "value"
}

func normalizeLlamaType(value any) any {
	switch v := value.(type) {
	case string:
		if isLlamaPrimitiveType(v) {
			return v
		}
		return "value"
	case []any:
		return normalizeLlamaTypeList(v)
	default:
		return "value"
	}
}

func normalizeLlamaTypeList(values []any) any {
	seen := map[string]struct{}{}
	out := make([]any, 0, len(values))
	for _, item := range values {
		s, ok := item.(string)
		if !ok || !isLlamaPrimitiveType(s) {
			s = "value"
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return "value"
	}
	if len(out) == 1 {
		return out[0]
	}
	return out
}

func addNullToLlamaType(value any) any {
	switch v := value.(type) {
	case string:
		if v == "value" || v == "null" {
			return v
		}
		return []any{v, "null"}
	case []any:
		for _, item := range v {
			if item == "null" || item == "value" {
				return v
			}
		}
		return append(v, "null")
	default:
		return value
	}
}

func isLlamaPrimitiveType(s string) bool {
	switch s {
	case "boolean", "integer", "number", "string", "array", "object", "null", "value":
		return true
	default:
		return false
	}
}

func toolCallProtocolKnown(protocol string) bool {
	switch protocol {
	case toolParserProtocolCommonChat:
		return true
	default:
		return false
	}
}
