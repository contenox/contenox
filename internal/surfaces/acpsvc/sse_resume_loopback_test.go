package acpsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/agentservice"
	libacp "github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

// Each notification the production Transport sends over a real client
// connection must carry its session's monotonic cursor in `_meta`, so a
// client can name its last seen sequence when it reconnects. This pins the
// wire contract the resume(sinceSeq) handler answers.
func TestLoopback_SessionUpdate_CarriesMonotonicSeqCursor(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)

	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: t.TempDir(), McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	h.lc.drain(t, 2) // command menu and initial context gauge

	var calls int
	fake := &loopbackAgent{}
	fake.promptFunc = func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		calls++
		reqID, _ := ctx.Value(libtracker.ContextKeyRequestID).(string)
		subject := taskengine.TaskEventRequestSubject(reqID)
		raw, mErr := json.Marshal(taskengine.TaskEvent{
			Kind:        taskengine.TaskEventStepChunk,
			TaskHandler: string(taskengine.HandleChatCompletion),
			Content:     fmt.Sprintf("turn %d chunk", calls),
		})
		require.NoError(t, mErr)
		require.NoError(t, h.bus.Publish(ctx, subject, raw))
		return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
	}
	h.swapAgent(newResp.SessionID, fake)

	prompt := func() uint64 {
		_, err := h.client.Prompt(ctx, libacp.PromptRequest{
			SessionID: newResp.SessionID,
			Prompt:    []libacp.ContentBlock{libacp.NewTextContent("hi")},
		})
		require.NoError(t, err)
		updates := h.lc.drain(t, 2) // chunk + session_info (no usage update in this harness)
		for _, n := range updates {
			if n.Update.SessionUpdate != libacp.SessionUpdateAgentMessageChunk {
				continue
			}
			var meta map[string]any
			require.NoError(t, json.Unmarshal(n.Meta, &meta))
			seq, ok := meta[libacp.EventSeqMetaKey].(float64)
			require.True(t, ok, "chunk notification must carry contenox.seq in _meta")
			return uint64(seq)
		}
		t.Fatalf("no agent message chunk delivered")
		return 0
	}

	first := prompt()
	second := prompt()
	require.Greater(t, second, first, "cursors are monotonic across turns on the same session")
}
