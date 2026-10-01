package openai

import (
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/stretchr/testify/require"
)

func TestUnit_CodexContinuationPreservesInterleavedOutput(t *testing.T) {
	output := `[
		{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-1","summary":[]},
		{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","status":"completed","content":[{"type":"output_text","text":"Inspecting source details","annotations":[]}]},
		{"type":"reasoning","id":"rs_2","encrypted_content":"opaque-2","summary":[]},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"local_fs_read","arguments":"{}","status":"completed"},
		{"type":"function_call","id":"fc_2","call_id":"call_2","name":"local_fs_read","arguments":"{}","status":"completed"}
	]`
	var response openAIResponse
	require.NoError(t, json.Unmarshal([]byte(`{"output":`+output+`}`), &response))
	c := &openAIClient{codex: &codexCatalog{}, modelName: "codex-test"}
	message := modelrepo.Message{Role: "assistant", Content: "Inspecting source details", Continuation: c.responseContinuation(&response)}
	message.ToolCalls = []modelrepo.ToolCall{{ID: "call_1"}, {ID: "call_2"}}
	saved, err := json.Marshal(message)
	require.NoError(t, err)
	var resumed modelrepo.Message
	require.NoError(t, json.Unmarshal(saved, &resumed))
	req, _, err := c.buildResponsesRequest([]modelrepo.Message{
		{Role: "user", Content: "read the sources"},
		resumed,
		{Role: "tool", ToolCallID: "call_1", Content: "first result"},
		{Role: "tool", ToolCallID: "call_2", Content: "second result"},
	}, nil)
	require.NoError(t, err)
	require.Len(t, req.Input, 8)
	replayed, err := json.Marshal(req.Input[1:6])
	require.NoError(t, err)
	require.JSONEq(t, output, string(replayed))
	require.Equal(t, "call_1", req.Input[6].CallID)
	require.Equal(t, "call_2", req.Input[7].CallID)
}

func TestUnit_CodexLegacyReasoningContinuation(t *testing.T) {
	c := &openAIClient{codex: &codexCatalog{}, modelName: "codex-test"}
	req, _, err := c.buildResponsesRequest([]modelrepo.Message{{
		Role: "assistant", Content: "old session",
		Continuation: &modelrepo.Continuation{Provider: modelauth.ProviderType, Model: "codex-test", Items: json.RawMessage(`[{"type":"reasoning","id":"rs_old","encrypted_content":"opaque","summary":[]}]`)},
	}}, nil)
	require.NoError(t, err)
	require.Len(t, req.Input, 2)
	require.Equal(t, "reasoning", req.Input[0].Type)
	require.Equal(t, "message", req.Input[1].Type)
}
