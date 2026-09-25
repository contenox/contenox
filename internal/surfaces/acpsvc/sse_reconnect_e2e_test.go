package acpsvc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/agentservice"
	libacp "github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

// The reconnect gap end to end: one client streams events live, disconnects
// (no connection attached), more events are journaled into the gap, a SECOND
// client reconnects naming its last seen cursor and receives exactly the
// missed cursor-carrying events — once, and not again on a second resume.
func TestLoopback_ResumeAcrossReconnect_ReplaysMissedGapExactlyOnce(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	cwd := t.TempDir()
	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: cwd, McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	sid := newResp.SessionID
	require.NotEmpty(t, sid)
	h.lc.drain(t, 2) // command menu and initial context gauge

	// One live turn, so the client holds a real cursor.
	fake := &loopbackAgent{}
	fake.promptFunc = func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		reqID, _ := ctx.Value(libtracker.ContextKeyRequestID).(string)
		raw, mErr := json.Marshal(taskengine.TaskEvent{
			Kind:        taskengine.TaskEventStepChunk,
			TaskHandler: string(taskengine.HandleChatCompletion),
			Content:     "live chunk",
		})
		require.NoError(t, mErr)
		require.NoError(t, h.bus.Publish(ctx, taskengine.TaskEventRequestSubject(reqID), raw))
		return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
	}
	h.swapAgent(sid, fake)
	_, err = h.client.Prompt(ctx, libacp.PromptRequest{SessionID: sid, Prompt: []libacp.ContentBlock{libacp.NewTextContent("hi")}})
	require.NoError(t, err)
	live := h.lc.drain(t, 2)
	var cursor uint64
	for _, n := range live {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(n.Meta, &meta))
		seq, ok := meta[libacp.EventSeqMetaKey].(float64)
		require.True(t, ok)
		if uint64(seq) > cursor {
			cursor = uint64(seq)
		}
	}
	require.Greater(t, cursor, uint64(0), "live turn must advance the cursor")

	// The client detaches. The session keeps producing: two more events are
	// journaled into the gap (the origin journaling path), with no connection.
	csid, ok := h.tr.contenoxSessionForACPID(sid)
	require.True(t, ok, "session must map to a contenox session id to journal")
	gapA := libacp.SessionNotification{SessionID: sid, Update: libacp.SessionUpdate{SessionUpdate: libacp.SessionUpdateAgentMessageChunk}}
	gapA.Update.Content = func() *libacp.ContentBlock { c := libacp.NewTextContent("gap a"); return &c }()
	gapB := libacp.SessionNotification{SessionID: sid, Update: libacp.SessionUpdate{SessionUpdate: libacp.SessionUpdateAgentMessageChunk}}
	gapB.Update.Content = func() *libacp.ContentBlock { c := libacp.NewTextContent("gap b"); return &c }()
	h.router.journalAppend(csid, gapA)
	h.router.journalAppend(csid, gapB)

	// A second client reconnects, naming its last seen cursor.
	rig := h.addSecondTransport(t)
	_, err = rig.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	_, err = rig.client.ResumeSession(ctx, libacp.ResumeSessionRequest{SessionID: sid, Cwd: cwd, McpServers: []libacp.McpServer{}, SinceSeq: cursor})
	require.NoError(t, err)

	got := collectChunks(t, rig, 2, 3*time.Second)
	require.Len(t, got, 2, "reconnect must deliver exactly the two missed events")
	require.Equal(t, "gap a", got[0].text)
	require.Equal(t, "gap b", got[1].text)
	require.Equal(t, cursor+1, got[0].seq)
	require.Equal(t, cursor+2, got[1].seq)

	// The client advances to cursor+2 and resumes again: nothing may repeat.
	_, err = rig.client.ResumeSession(ctx, libacp.ResumeSessionRequest{SessionID: sid, Cwd: cwd, McpServers: []libacp.McpServer{}, SinceSeq: cursor + 2})
	require.NoError(t, err)
	if again := collectChunks(t, rig, 1, 500*time.Millisecond); len(again) != 0 {
		t.Fatalf("second resume replayed %d chunk events (want none)", len(again))
	}
}

type chunkSeen struct {
	text string
	seq  uint64
}

func collectChunks(t *testing.T, rig *secondRig, want int, within time.Duration) []chunkSeen {
	t.Helper()
	var out []chunkSeen
	deadline := time.After(within)
	for len(out) < want {
		select {
		case n := <-rig.lc.updates:
			if n.Update.SessionUpdate != libacp.SessionUpdateAgentMessageChunk {
				continue
			}
			var meta map[string]any
			if err := json.Unmarshal(n.Meta, &meta); err != nil {
				continue
			}
			seq, _ := meta[libacp.EventSeqMetaKey].(float64)
			out = append(out, chunkSeen{text: n.Update.Content.Text, seq: uint64(seq)})
		case <-deadline:
			return out
		}
	}
	return out
}
