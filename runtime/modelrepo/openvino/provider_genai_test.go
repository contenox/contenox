//go:build openvino && openvino_genai

package openvino

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/modelrepo"
)

func TestSystem_OpenVINOProvider_GenAIChatAndPrompt(t *testing.T) {
	require.NoError(t, closeGenAISessionPoolForTest())
	t.Cleanup(func() {
		require.NoError(t, closeGenAISessionPoolForTest())
	})

	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL to an OpenVINO IR model directory")
	}

	p := &openvinoProvider{
		name:     filepath.Base(modelDir),
		modelDir: filepath.Dir(modelDir),
		caps: modelrepo.CapabilityConfig{
			MaxOutputTokens: 16,
		},
	}
	require.True(t, p.CanChat())
	require.True(t, p.CanPrompt())
	require.True(t, p.CanStream())
	require.False(t, p.CanEmbed())

	chat, err := p.GetChatConnection(context.Background(), "openvino")
	require.NoError(t, err)

	chatResult, err := chat.Chat(
		context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "Continue this Python code:\ndef add(a, b):\n    return"}},
		modelrepo.WithMaxTokens(16),
	)
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(chatResult.Message.Content))
	require.Equal(t, "assistant", chatResult.Message.Role)

	prompt, err := p.GetPromptConnection(context.Background(), "openvino")
	require.NoError(t, err)
	entries, refs := genAISessionPoolStatsForTest()
	require.Equal(t, 1, entries)
	require.Equal(t, 2, refs)

	promptResult, err := prompt.Prompt(
		context.Background(),
		"You complete tiny Python snippets.",
		0,
		"Continue this code:\ndef sub(a, b):\n    return",
	)
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(promptResult))

	stream, err := p.GetStreamConnection(context.Background(), "openvino")
	require.NoError(t, err)
	entries, refs = genAISessionPoolStatsForTest()
	require.Equal(t, 1, entries)
	require.Equal(t, 3, refs)

	parcels, err := stream.Stream(
		context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "Continue this Python code:\ndef mul(a, b):\n    return"}},
		modelrepo.WithMaxTokens(16),
	)
	require.NoError(t, err)
	var streamed strings.Builder
	for parcel := range parcels {
		require.NoError(t, parcel.Error)
		streamed.WriteString(parcel.Data)
	}
	require.NotEmpty(t, strings.TrimSpace(streamed.String()))

	require.NoError(t, chat.(interface{ Close() error }).Close())
	require.NoError(t, prompt.(interface{ Close() error }).Close())
	require.NoError(t, stream.(interface{ Close() error }).Close())
	entries, refs = genAISessionPoolStatsForTest()
	require.Equal(t, 1, entries)
	require.Equal(t, 0, refs)
}
