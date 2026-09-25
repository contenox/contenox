package acpsvc

import (
	"testing"

	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/stretchr/testify/require"
)

// The model list is what a person picks from, and a provider that is reachable
// but contributes nothing to it is invisible: `contenox doctor` says two
// backends answered, the picker offers one, and nothing anywhere explains the
// difference. These pin the rule that prevents it.
func TestUnit_OfferableModels(t *testing.T) {
	t.Run("keeps the models a catalogue marks as chat-capable", func(t *testing.T) {
		got := offerableModels([]runtimestate.ModelPullStatus{
			{Model: "a", CanChat: true},
			{Model: "b", CanPrompt: true},
		})
		require.Len(t, got, 2)
	})

	t.Run("drops the ones it positively marks as something else", func(t *testing.T) {
		// The catalogue answered the question: these are embeddings, not chat.
		got := offerableModels([]runtimestate.ModelPullStatus{
			{Model: "chat", CanChat: true},
			{Model: "embed", CanEmbed: true},
		})
		require.Len(t, got, 1)
		require.Equal(t, "chat", got[0].Model)
	})

	t.Run("offers the undescribed rather than hiding the backend", func(t *testing.T) {
		// The regression: no capability flags anywhere in the list used to mean
		// an empty contribution, so a whole provider vanished from the picker.
		got := offerableModels([]runtimestate.ModelPullStatus{
			{Model: "mystery-1"},
			{Model: "mystery-2"},
		})
		require.Len(t, got, 2)
		require.Equal(t, "mystery-1", got[0].Model)
	})

	t.Run("an undescribed model cannot outrank a described one", func(t *testing.T) {
		// The fallback is a fallback: where the catalogue did answer, its answer
		// stands, and the undescribed entries stay out of that list.
		got := offerableModels([]runtimestate.ModelPullStatus{
			{Model: "known", CanChat: true},
			{Model: "mystery"},
		})
		require.Len(t, got, 1)
		require.Equal(t, "known", got[0].Model)
	})

	t.Run("a backend that reports no models contributes none", func(t *testing.T) {
		require.Empty(t, offerableModels(nil))
	})

	t.Run("a positively-non-chat backend stays out", func(t *testing.T) {
		got := offerableModels([]runtimestate.ModelPullStatus{
			{Model: "embed", CanEmbed: true},
			{Model: "transcribe", CanAudio: true},
		})
		require.Empty(t, got)
	})
}
