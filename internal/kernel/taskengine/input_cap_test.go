package taskengine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/stretchr/testify/require"
)

func TestUnit_CappedContentCannotReplayUncappedContinuation(t *testing.T) {
	history := ChatHistory{Messages: []Message{{
		Role: "assistant", Content: strings.Repeat("content ", 100),
		Continuation: &modelrepo.Continuation{Provider: "openai-codex", Model: "test", Items: json.RawMessage(`{"output":[]}`)},
	}}}
	out := capTaskChatHistory(history, 100)
	require.Len(t, out.Messages, 1)
	require.NotEqual(t, history.Messages[0].Content, out.Messages[0].Content)
	require.Nil(t, out.Messages[0].Continuation)
	require.NotNil(t, history.Messages[0].Continuation)
}
