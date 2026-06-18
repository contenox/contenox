package llama

import (
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/toolcalls"
)

// Model-native tool-call machinery is shared with the OpenVINO backend in
// runtime/modelrepo/toolcalls; these thin aliases keep the llama call sites and
// the model-declared protocol stance (no guessing) unchanged.

type toolCallParser = toolcalls.Parser

func serializeToolDefs(tools []modelrepo.Tool) (string, error) {
	return toolcalls.SerializeToolDefs(tools)
}

func toolCallProtocolKnown(protocol string) bool { return toolcalls.ProtocolKnown(protocol) }

func toolCallParserFor(protocol string) (toolCallParser, error) { return toolcalls.ParserFor(protocol) }
