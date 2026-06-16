//go:build openvino && openvino_genai

package openvino

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/openvino/ovsession"
	"github.com/stretchr/testify/require"
)

// TestSystem_OpenVINOChatContext_ClassifierWarmsChatPath proves that the
// provider chat path, not only the raw segment harness, preserves a stable
// prefix that OpenVINO's prefix cache can reuse.
func TestSystem_OpenVINOChatContext_ClassifierWarmsChatPath(t *testing.T) {
	require.NoError(t, closeGenAISessionPoolForTest())
	t.Cleanup(func() {
		require.NoError(t, closeGenAISessionPoolForTest())
	})

	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}
	device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE")
	if device == "" {
		device = "CPU"
	}

	var repoMap strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&repoMap, "pkg/mod%04d.go: func Handler%04d(ctx context.Context) error { return nil }\n", i, i)
	}
	stableSystem := "You are a local coding agent. Use this repository map as context:\n" + repoMap.String()
	withQuestion := func(question string) []modeld.Message {
		return []modeld.Message{
			{Role: "system", Content: stableSystem},
			{Role: "user", Content: question},
		}
	}

	modelDigest, err := modelDirDigest(modelDir)
	require.NoError(t, err)
	cfg := ovsession.GenAIConfig{Device: device}
	modelName := filepath.Base(modelDir)
	warm, err := newGenAIClient(context.Background(), modelName, modelDir, modelDigest, cfg, 8, "", "", libtracker.NoopTracker{})
	require.NoError(t, err)
	defer warm.Close()

	cold, err := newGenAIClient(context.Background(), modelName, modelDir, modelDigest+"-cold-control", cfg, 8, "", "", libtracker.NoopTracker{})
	require.NoError(t, err)
	defer cold.Close()

	turnA := withQuestion("Reply with exactly one short word about functions.")
	turnB := withQuestion("Reply with exactly one short word about handlers.")
	planA := classifyChatContext(turnA, "")
	planB := classifyChatContext(turnB, "")
	require.Equal(t, planA.StablePrefixHash, planB.StablePrefixHash)

	_, err = warm.Chat(context.Background(), []modeld.Message{{Role: "user", Content: "hello"}}, modeld.WithMaxTokens(1), modeld.WithTemperature(0))
	require.NoError(t, err)
	_, err = cold.Chat(context.Background(), []modeld.Message{{Role: "user", Content: "hello"}}, modeld.WithMaxTokens(1), modeld.WithTemperature(0))
	require.NoError(t, err)

	start := time.Now()
	_, err = warm.Chat(context.Background(), turnA, modeld.WithMaxTokens(8), modeld.WithTemperature(0), modeld.WithSeed(1))
	require.NoError(t, err)
	coldPrefix := time.Since(start)

	start = time.Now()
	warmB, err := warm.Chat(context.Background(), turnB, modeld.WithMaxTokens(8), modeld.WithTemperature(0), modeld.WithSeed(1))
	require.NoError(t, err)
	warmSuffix := time.Since(start)

	start = time.Now()
	coldB, err := cold.Chat(context.Background(), turnB, modeld.WithMaxTokens(8), modeld.WithTemperature(0), modeld.WithSeed(1))
	require.NoError(t, err)
	coldFull := time.Since(start)

	t.Logf("chat turn A cold prefix = %v", coldPrefix)
	t.Logf("chat turn B warm suffix = %v  hash=%s", warmSuffix, planB.StablePrefixHash[:12])
	t.Logf("chat turn B cold full   = %v", coldFull)

	require.Equal(t, strings.TrimSpace(coldB.Message.Content), strings.TrimSpace(warmB.Message.Content))
	require.Less(t, warmSuffix, coldFull, "classified chat turn should reuse the warm stable prefix")
}
