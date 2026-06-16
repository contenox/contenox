//go:build openvino && openvino_genai

package openvino

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/stretchr/testify/require"
)

// TestSystem_OpenVINOSegments_AssemblerDrivesPrefixCache proves the workspace-context reuse
// end-to-end: that AssembleContext's deterministic stable prefix actually
// controls the OpenVINO prefix cache. Same stable segments across turns ->
// byte-identical prefix -> warm cache hit (fast). Edit a stable segment ->
// changed prefix -> re-prefill (slow again). The stable-prefix hash predicts
// which happens.
func TestSystem_OpenVINOSegments_AssemblerDrivesPrefixCache(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	s, err := ovsession.NewGenAI(modelDir, ovsession.GenAIConfig{Device: device})
	require.NoError(t, err)
	defer s.Close()

	// A large stable repo-map segment — the hot workspace context a coding node
	// keeps warm. Big enough that prefill dominates fixed per-request overhead.
	var repoMap strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&repoMap, "pkg/mod%04d.go: func Handler%04d(ctx context.Context) error { return nil }\n", i, i)
	}
	stable := []Segment{
		{Kind: KindSystem, Content: "You are a local coding agent."},
		{Kind: KindRepoMap, Content: repoMap.String()},
	}

	gen := func(segs []Segment) (time.Duration, string) {
		prompt, hash := AssembleContext(segs)
		start := time.Now()
		_, err := s.Generate(context.Background(), prompt, ovsession.GenerateOptions{MaxNewTokens: 1})
		require.NoError(t, err)
		return time.Since(start), hash
	}

	// Warm the runtime so one-time init is not charged to turn 1.
	_, _ = s.Generate(context.Background(), "hello", ovsession.GenerateOptions{MaxNewTokens: 1})

	withTurn := func(base []Segment, q string) []Segment {
		return append(append([]Segment{}, base...), Segment{Kind: KindUserTurn, Content: q})
	}

	t1, h1 := gen(withTurn(stable, "question A"))
	t2, h2 := gen(withTurn(stable, "a different question B"))

	editedStable := []Segment{
		{Kind: KindSystem, Content: "You are a local coding agent. EDITED."},
		{Kind: KindRepoMap, Content: repoMap.String()},
	}
	t3, h3 := gen(withTurn(editedStable, "question C"))

	t.Logf("turn1 cold (new stable prefix)       = %v  hash=%s", t1, h1[:12])
	t.Logf("turn2 warm (same stable prefix)      = %v  hash=%s", t2, h2[:12])
	t.Logf("turn3 stable edited (prefix changed) = %v  hash=%s", t3, h3[:12])

	require.Equal(t, h1, h2, "unchanged stable segments must yield the same stable-prefix hash")
	require.NotEqual(t, h1, h3, "editing a stable segment must change the stable-prefix hash")
	require.Less(t, float64(t2), 0.5*float64(t1), "warm turn (same stable prefix) must be much faster than cold")
	require.Greater(t, float64(t3), 2*float64(t2), "editing a stable segment must re-prefill and be slow again")
}
