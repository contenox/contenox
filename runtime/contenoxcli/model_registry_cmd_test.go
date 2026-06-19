package contenoxcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_PrintModelRegistryListIncludesBackendColumn(t *testing.T) {
	var out bytes.Buffer
	entries := []modelregistry.ModelDescriptor{
		{Name: "qwen3-8b-ov", Backend: "openvino", SourceURL: "https://example.com/ov", SizeBytes: 2 * 1024 * 1024, Curated: true},
		{Name: "qwen3-8b", SourceURL: "https://example.com/gguf", SizeBytes: 1024 * 1024, Curated: true},
	}

	require.NoError(t, printModelRegistryList(&out, entries))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "BACKEND")
	assert.Contains(t, lines[1], "qwen3-8b")
	assert.Contains(t, lines[1], "llama")
	assert.Contains(t, lines[2], "qwen3-8b-ov")
	assert.Contains(t, lines[2], "openvino")
}
