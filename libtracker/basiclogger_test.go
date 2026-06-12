package libtracker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnit_BoundedLogValue_CapsLargePayloads(t *testing.T) {
	large := strings.Repeat("x", logChangeDataMaxJSONBytes+1024)

	got := boundedLogValue(large)

	summary, ok := got.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, summary["truncated"])
	require.Equal(t, "payload_exceeds_limit", summary["reason"])
	require.Equal(t, "string", summary["original_type"])
	require.Greater(t, summary["original_json_bytes"], logChangeDataMaxJSONBytes)
	require.NotEmpty(t, summary["sha256"])
	require.LessOrEqual(t, summary["preview_bytes"], logChangeDataPreviewBytes)
}

func TestUnit_BoundedLogValue_KeepsSmallPayloads(t *testing.T) {
	got := boundedLogValue(map[string]any{"status": "ok"})

	require.Equal(t, map[string]any{"status": "ok"}, got)
}
