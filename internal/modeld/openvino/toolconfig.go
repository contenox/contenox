package openvino

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/contenox/internal/transport"
)

func resolveToolConfig(cfg transport.DecodeConfig, tools string) (transport.DecodeConfig, error) {
	protocols := make([]string, 0, len(cfg.ParserProtocols))
	structured := false
	for _, protocol := range cfg.ParserProtocols {
		if protocol == "openvino:json_schema_tool_calls" {
			structured = true
		} else {
			protocols = append(protocols, protocol)
		}
	}
	cfg.ParserProtocols = protocols
	if !structured || cfg.StructuredOutput.Protocol != "" || strings.TrimSpace(tools) == "" {
		return cfg, nil
	}
	var definitions []struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(tools), &definitions); err != nil {
		return cfg, fmt.Errorf("openvino: decode tool definitions: %w", err)
	}
	if len(definitions) == 0 {
		return cfg, nil
	}
	choices := make([]any, 0, len(definitions))
	for _, tool := range definitions {
		if tool.Type != "function" || tool.Function.Name == "" {
			return cfg, fmt.Errorf("openvino: tool definition requires type function and a name")
		}
		parameters := tool.Function.Parameters
		if len(parameters) == 0 || string(parameters) == "null" {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		choices = append(choices, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"const": tool.Function.Name},
				"arguments": parameters,
			},
			"required":             []string{"name", "arguments"},
			"additionalProperties": false,
		})
	}
	format := map[string]any{
		"type":     "triggered_tags",
		"triggers": []string{"<tool_call>"},
		"tags": []any{map[string]any{
			"type": "tag", "begin": "<tool_call>", "end": "</tool_call>",
			"content": map[string]any{"type": "json_schema", "json_schema": map[string]any{"anyOf": choices}},
		}},
		"at_least_one": false, "stop_after_first": false,
	}
	payload, err := json.Marshal(map[string]any{"type": "structural_tag", "format": format})
	if err != nil {
		return cfg, fmt.Errorf("openvino: encode tool constraints: %w", err)
	}
	cfg.StructuredOutput = transport.StructuredOutputConfig{Protocol: "openvino:json_schema_tool_calls", Payload: string(payload)}
	return cfg, nil
}
