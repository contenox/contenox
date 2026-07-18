package agenthost_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libacp/acpexec"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agenthost"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

// registerAgent walks the real registration leg: it stands up the agent
// registry service on a fresh SQLite DB, creates an agents row pointing at
// command, and resolves it back by name — the same
// registry-row → resolve path the composed e2e is meant to prove, not a
// hand-built struct handed straight to the host. Shared by every server this
// package's e2e drives (stub, testy, the contenox loopback, claude).
func registerAgent(t *testing.T, name, command string, args ...string) (context.Context, *runtimetypes.Agent) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "agenthost-e2e.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	svc := agentregistryservice.New(db)
	agent := &runtimetypes.Agent{Name: name, Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   command,
		Args:      args,
	}))
	require.NoError(t, svc.Create(ctx, agent))

	resolved, err := svc.GetByName(ctx, name)
	require.NoError(t, err)
	return ctx, resolved
}

// TestHost_DriveTurn_RegistryToStubRoundTrip is the composed e2e this slice
// exists for: an agents row created and resolved through the real registry
// service, handed to DriveTurn, spawns the hermetic stub agent and drives a
// full initialize → session/new → prompt turn to a terminal stopReason, with
// the streamed reply landing on the recording harness — registry → host →
// live ACP session → answer, no piece mocked.
func TestHost_DriveTurn_RegistryToStubRoundTrip(t *testing.T) {
	stubBin := buildStubAgent(t)
	ctx, agent := registerAgent(t, "stub-roundtrip", stubBin)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var stderr acpexec.LockedBuffer
	harness := &agenthost.RecordingHarness{}
	res, err := agenthost.DriveTurn(ctx, agent, harness, agenthost.TurnRequest{
		Cwd:    t.TempDir(),
		Prompt: []libacp.ContentBlock{libacp.NewTextContent("hello from the composed host")},
		Stderr: &stderr,
	})
	require.NoError(t, err, "stub stderr:\n%s", stderr.String())

	require.Equal(t, libacp.ProtocolVersion, res.Initialize.ProtocolVersion)
	require.NotNil(t, res.Initialize.AgentInfo)
	require.Equal(t, "acp-stub-agent", res.Initialize.AgentInfo.Name)
	require.NotEmpty(t, res.SessionID)
	require.Equal(t, libacp.StopReasonEndTurn, res.StopReason)

	// The plain-prompt path acks with exactly one message chunk; the turn
	// must have produced displayable output, not just a stopReason.
	require.Equal(t, "ack", harness.MessageText())
	tracker := &libacp.TurnTracker{}
	for _, n := range harness.Updates() {
		tracker.Observe(n)
	}
	require.NoError(t, tracker.Err(res.StopReason))
}

// TestHost_DriveTurn_StreamingUpdatesReachHarness drives the stub's
// session_updates scenario and asserts the whole notification stream —
// message chunks and tool_call/tool_call_update — passes through DriveTurn to
// the caller's harness in wire order, pinning that the harness seam really is
// pass-through for non-message updates too.
func TestHost_DriveTurn_StreamingUpdatesReachHarness(t *testing.T) {
	stubBin := buildStubAgent(t)
	ctx, agent := registerAgent(t, "stub-streaming", stubBin)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	harness := &agenthost.RecordingHarness{}
	res, err := agenthost.DriveTurn(ctx, agent, harness, agenthost.TurnRequest{
		Cwd:    t.TempDir(),
		Prompt: []libacp.ContentBlock{libacp.NewTextContent(`{"command":"run_scenario","scenario":"session_updates"}`)},
	})
	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, res.StopReason)

	require.Equal(t, "running scenario...done", harness.MessageText())

	var kinds []libacp.SessionUpdateKind
	for _, n := range harness.Updates() {
		require.Equal(t, res.SessionID, n.SessionID)
		kinds = append(kinds, n.Update.SessionUpdate)
	}
	require.Equal(t, []libacp.SessionUpdateKind{
		libacp.SessionUpdateAgentMessageChunk,
		libacp.SessionUpdateToolCall,
		libacp.SessionUpdateToolCallUpdate,
		libacp.SessionUpdateAgentMessageChunk,
	}, kinds)
}

// TestHost_DriveTurn_RejectsNonExternalKind locks in that DriveTurn refuses a
// row whose kind it cannot drive (the reserved "chain" kind) with a clear
// resolve error instead of attempting to spawn anything.
func TestHost_DriveTurn_RejectsNonExternalKind(t *testing.T) {
	agent := &runtimetypes.Agent{Name: "future-chain", Kind: runtimetypes.AgentKindChain}

	_, err := agenthost.DriveTurn(context.Background(), agent, &agenthost.RecordingHarness{}, agenthost.TurnRequest{
		Cwd:    t.TempDir(),
		Prompt: []libacp.ContentBlock{libacp.NewTextContent("hi")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain")
}

// TestHost_DriveTurn_RequiresCwdAndPrompt locks in the early, clear errors
// for the two required TurnRequest fields, so a misassembled call fails
// before any subprocess is spawned.
func TestHost_DriveTurn_RequiresCwdAndPrompt(t *testing.T) {
	agent := &runtimetypes.Agent{Name: "unused"}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "irrelevant-not-actually-spawned",
	}))

	_, err := agenthost.DriveTurn(context.Background(), agent, &agenthost.RecordingHarness{}, agenthost.TurnRequest{
		Prompt: []libacp.ContentBlock{libacp.NewTextContent("hi")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cwd")

	_, err = agenthost.DriveTurn(context.Background(), agent, &agenthost.RecordingHarness{}, agenthost.TurnRequest{
		Cwd: t.TempDir(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Prompt")
}
