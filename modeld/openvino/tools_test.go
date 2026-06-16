package openvino

import (
	"encoding/json"
	"testing"

	"github.com/contenox/runtime/modeld"
	"github.com/stretchr/testify/require"
)

func TestUnit_OpenVINOToolsToJSON(t *testing.T) {
	empty, err := toolsToJSON(nil)
	require.NoError(t, err)
	require.Equal(t, "", empty)

	out, err := toolsToJSON([]modeld.Tool{
		{Type: "function", Function: &modeld.FunctionTool{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  map[string]any{"type": "object"},
		}},
	})
	require.NoError(t, err)
	require.Contains(t, out, "get_weather")

	var decoded []modeld.Tool
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	require.Len(t, decoded, 1)
	require.Equal(t, "get_weather", decoded[0].Function.Name)
}

func TestUnit_OpenVINODecodeParsedGeneration_DirectToolCall(t *testing.T) {
	parsed, err := decodeParsedGeneration(`{
		"content": "calling tool",
		"tool_calls": [
			{"name": "get_weather", "arguments": {"city": "Berlin"}}
		]
	}`)
	require.NoError(t, err)

	require.Equal(t, "calling tool", parsed.content)
	require.Len(t, parsed.calls, 1)
	require.Equal(t, "function", parsed.calls[0].Type)
	require.Equal(t, "get_weather", parsed.calls[0].Function.Name)
	require.JSONEq(t, `{"city":"Berlin"}`, parsed.calls[0].Function.Arguments)
	require.NotEmpty(t, parsed.calls[0].ID)
}

func TestUnit_OpenVINODecodeParsedGeneration_WrappedToolCall(t *testing.T) {
	parsed, err := decodeParsedGeneration(`{
		"content": "",
		"tool_calls": [
			{"type":"function","function":{"name":"lookup","parameters":{"q":"x"}}}
		]
	}`)
	require.NoError(t, err)

	require.Len(t, parsed.calls, 1)
	require.Equal(t, "lookup", parsed.calls[0].Function.Name)
	require.JSONEq(t, `{"q":"x"}`, parsed.calls[0].Function.Arguments)
}

func TestUnit_OpenVINODecodeParsedGeneration_Reasoning(t *testing.T) {
	parsed, err := decodeParsedGeneration(`{
		"content": "final",
		"reasoning_content": "think"
	}`)
	require.NoError(t, err)

	require.Equal(t, "final", parsed.content)
	require.Equal(t, "think", parsed.thinking)
	require.Empty(t, parsed.calls)
}
