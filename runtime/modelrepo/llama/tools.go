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
	b, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("llama: serialize tool definitions: %w", err)
	}
	return string(b), nil
}

func toolCallProtocolKnown(protocol string) bool {
	switch protocol {
	case toolParserProtocolCommonChat:
		return true
	default:
		return false
	}
}
