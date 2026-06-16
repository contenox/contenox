package openvino

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

func TestUnit_OpenVINOProvider_CapabilitiesUseConfig(t *testing.T) {
	p := &openvinoProvider{
		name: "qwen-coder",
		caps: modelrepo.CapabilityConfig{
			ContextLength:   65536,
			MaxOutputTokens: 4096,
			CanThink:        true,
		},
	}

	require.Equal(t, "openvino", p.GetType())
	require.Equal(t, "qwen-coder", p.ModelName())
	require.Equal(t, "openvino:qwen-coder", p.GetID())
	require.Equal(t, []string{"openvino"}, p.GetBackendIDs())
	require.Equal(t, 65536, p.GetContextLength())
	require.Equal(t, 4096, p.GetMaxOutputTokens())
	require.Equal(t, ovsession.GenAIAvailable, p.CanChat())
	require.Equal(t, ovsession.GenAIAvailable, p.CanPrompt())
	require.Equal(t, ovsession.GenAIAvailable, p.CanStream())
	require.False(t, p.CanEmbed())
	require.True(t, p.CanThink())
}

func TestUnit_OpenVINOProvider_ConnectionsReturnNotWired(t *testing.T) {
	p := &openvinoProvider{name: "qwen-coder"}

	if !p.CanChat() {
		_, err := p.GetChatConnection(context.Background(), "openvino")
		require.ErrorContains(t, err, "not wired")
	}

	if !p.CanPrompt() {
		_, err := p.GetPromptConnection(context.Background(), "openvino")
		require.ErrorContains(t, err, "not wired")
	}

	if !p.CanStream() {
		_, err := p.GetStreamConnection(context.Background(), "openvino")
		require.ErrorContains(t, err, "not wired")
	}

	_, err := p.GetEmbedConnection(context.Background(), "openvino")
	require.ErrorContains(t, err, "not wired")
}
