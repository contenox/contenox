package openvino

const (
	toolParserProtocolLlama3Pythonic = "openvino:llama3_pythonic_tool_parser"
	toolParserProtocolLlama3JSON     = "openvino:llama3_json_tool_parser"
	toolProtocolJSONSchemaToolCalls  = "openvino:json_schema_tool_calls"
)

func toolCallProtocolKnown(protocol string) bool {
	switch protocol {
	case toolParserProtocolLlama3Pythonic,
		toolParserProtocolLlama3JSON,
		toolProtocolJSONSchemaToolCalls:
		return true
	default:
		return false
	}
}

func toolCallProtocolUsesParser(protocol string) bool {
	switch protocol {
	case toolParserProtocolLlama3Pythonic,
		toolParserProtocolLlama3JSON:
		return true
	default:
		return false
	}
}
