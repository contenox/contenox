package runtimetypes_test

import (
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo/ollama"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

// A declaration outside the vocabulary that readers parse would be stored,
// reported, and then silently ignored, so the store's names must stay the
// Ollama /api/show names verbatim.
func TestUnit_ModelCapabilities_MatchTheOllamaWireVocabulary(t *testing.T) {
	require.ElementsMatch(t, []string{
		string(ollama.CapabilityCompletion),
		string(ollama.CapabilityTools),
		string(ollama.CapabilityVision),
		string(ollama.CapabilityThinking),
		string(ollama.CapabilityEmbedding),
		string(ollama.CapabilityAudio),
	}, runtimetypes.ModelCapabilities())

	for _, name := range runtimetypes.ModelCapabilities() {
		require.True(t, runtimetypes.ValidModelCapability(name))
	}
	require.False(t, runtimetypes.ValidModelCapability("telepathy"))
}
