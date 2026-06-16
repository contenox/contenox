package openvino

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/modeld"
)

// toolsToJSON serializes tool definitions into the OpenAI-style JSON array that
// chat templates (e.g. Qwen) expect for their `tools` argument. Returns "" when
// there are no tools, so the template renders without a tools section.
func toolsToJSON(tools []modeld.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("openvino: marshal tools: %w", err)
	}
	return string(b), nil
}
