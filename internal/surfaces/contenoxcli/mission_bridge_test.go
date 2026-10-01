package contenoxcli

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/services/hitlservice"
	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionMCPAttentionRoundTrip(t *testing.T) {
	db := systemTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := runtimetypes.New(db.WithoutTransaction())
	hitl := hitlservice.NewWithDefaultPolicy(nil, runtimetypes.LocalTenantID, store, nil, "")
	pub := &recordingPublisher{}
	missions := missionservice.New(db, missionservice.WithEventPublisher(pub))
	m := &missionservice.Mission{Intent: "ask for direction", AgentName: "external", HITLPolicyName: "default", ParentSessionID: "parent"}
	require.NoError(t, missions.Create(ctx, m))
	repo := missiontools.New(missions, missiontools.WithAttentionAsker(missionAttentionAsker{hitl: hitl, missions: missions, bus: pub}))
	desc, close, err := missiontools.OpenMCP(ctx, repo, m.ID, t.TempDir(), []string{"contenox"})
	require.NoError(t, err)
	defer close()
	conn, err := net.Dial("unix", desc.Args[0])
	require.NoError(t, err)
	_, err = io.WriteString(conn, desc.Env[0].Value)
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	require.NoError(t, err)
	defer session.Close()
	type reply struct {
		result *mcp.CallToolResult
		err    error
	}
	answered := make(chan reply, 1)
	go func() {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "mission_ask_attention", Arguments: map[string]any{"summary": "Which diff?"}})
		answered <- reply{result, err}
	}()
	var ask *runtimetypes.HITLApproval
	require.Eventually(t, func() bool {
		pending, err := hitl.ListPending(ctx, 10)
		if err != nil || len(pending) != 1 {
			return false
		}
		ask = pending[0]
		return true
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, m.ID, *ask.MissionID)
	require.NoError(t, hitl.Answer(ctx, ask.ID, "Use the supplied patch"))
	select {
	case reply := <-answered:
		require.NoError(t, reply.err)
		require.False(t, reply.result.IsError)
		require.Equal(t, "Use the supplied patch", reply.result.Content[0].(*mcp.TextContent).Text)
	case <-ctx.Done():
		t.Fatal("MCP question did not receive the answer")
	}
	require.True(t, pub.seen(missionservice.AttentionAskedSubject))
}
