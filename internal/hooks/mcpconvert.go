package hooks

import (
	"github.com/contenox/contenox/runtimetypes"
	"github.com/contenox/contenox/taskengine"
)

// mcpToolToTaskTool converts a runtimetypes.MCPTool (received from mcpworker via NATS)
// to a taskengine.Tool. InputSchema is passed as-is; the LLM provider handles any
// schema sanitisation it needs (e.g. Gemini strips additionalProperties).
func mcpToolToTaskTool(hookName string, t runtimetypes.MCPTool) taskengine.Tool {
	_ = hookName // available for future namespacing
	var params any
	if len(t.InputSchema) > 0 {
		params = t.InputSchema
	}
	return taskengine.Tool{
		Type: "function",
		Function: taskengine.FunctionTool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		},
	}
}
