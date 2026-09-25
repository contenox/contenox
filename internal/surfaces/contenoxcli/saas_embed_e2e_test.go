package contenoxcli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelrepo/scriptedtest"
	"github.com/contenox/contenox/internal/services/chatservice"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/acpsvc"
	"github.com/contenox/contenox/internal/surfaces/gateway"
	libacp "github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

// This file is the embedding shape a hosted deployment needs, and nothing else:
// a caller in this process drives a session with a direct method call —
// NewSession, then Prompt — with no ACP client, no socket, no bridge and no beam.
// What a deployment replaces is the other end of the connection: instead of a
// terminal draining notifications, an HTTP or SSE sink does. The turn, the tool
// calls, the approval gate and the stored transcript are the real ones.

// inProcessRuntime is one embedded runtime: a real store, the real engine on the
// scripted backend, the real HITL service and the real ACP transport, with only
// the model replaced.
type inProcessRuntime struct {
	t             *testing.T
	db            libdb.DBManager
	transport     *acpsvc.Transport
	contenoxDir   string
	workspace     string
	workspaceID   string
	notifications *notificationSink
}

// notificationSink drains the agent-side stream, standing in for whatever a
// hosted deployment reads it with. It exists because the streaming half of a
// turn is not in the method's return value: a direct caller gets a stop reason,
// and everything the user sees arrives on the connection.
type notificationSink struct {
	mu      sync.Mutex
	updates []libacp.SessionNotification
}

func (s *notificationSink) add(notif libacp.SessionNotification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, notif)
}

func (s *notificationSink) snapshot() []libacp.SessionNotification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]libacp.SessionNotification(nil), s.updates...)
}

// text is every assistant message chunk the turn produced, concatenated — what a
// UI renders as the reply.
func (s *notificationSink) text() string {
	var out string
	for _, n := range s.snapshot() {
		if n.Update.SessionUpdate == libacp.SessionUpdateAgentMessageChunk && n.Update.Content != nil {
			out += n.Update.Content.Text
		}
	}
	return out
}

// kinds is the ordered set of update kinds, so a test can assert a tool call
// reached the client and not only the transcript.
func (s *notificationSink) kinds() []string {
	var out []string
	for _, n := range s.snapshot() {
		out = append(out, string(n.Update.SessionUpdate))
	}
	return out
}

// drainStream decodes the NDJSON the transport writes. A line that does not
// decode is skipped rather than fatal: the point is to observe what a caller
// would receive, not to re-implement the codec.
func drainStream(r io.Reader, sink *notificationSink, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			Method string                     `json:"method"`
			Params libacp.SessionNotification `json:"params"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}
		if envelope.Method != "session/update" {
			continue
		}
		sink.add(envelope.Params)
	}
}

// newInProcessRuntime composes what the CLI composes for a host profile, with no
// surface attached to it.
func newInProcessRuntime(t *testing.T, dialog, policy string) *inProcessRuntime {
	t.Helper()
	ctx := context.Background()
	t.Setenv("HOME", t.TempDir())

	workspace := workspaceDir(t, "saas-workspace")
	contenoxDir := filepath.Join(workspace, ".contenox")
	require.NoError(t, os.MkdirAll(contenoxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contenoxDir, "hitl-policy-default.json"), []byte(policy), 0o644))
	scriptPath := filepath.Join(contenoxDir, "dialog.json")
	require.NoError(t, os.WriteFile(scriptPath, []byte(dialog), 0o644))

	db := systemTestDB(t)
	require.NoError(t, runtimetypes.New(db.WithoutTransaction()).CreateBackend(ctx, &runtimetypes.Backend{
		ID:      "scripted-backend",
		Name:    "scripted",
		Type:    modelrepo.ScriptedTestBackendType,
		BaseURL: scriptPath,
	}))

	var transport *acpsvc.Transport
	transportFn := func() *acpsvc.Transport { return transport }
	hitl := newHITLService(ctx, contenoxDir, runtimetypes.New(db.WithoutTransaction()), libtracker.NoopTracker{}, "")
	router := acpsvc.NewSessionRouter()

	engine, err := BuildEngine(ctx, db, chatOpts{
		EffectiveDefaultModel:    scriptedtest.DefaultModelName,
		EffectiveDefaultProvider: modelrepo.ScriptedTestBackendType,
		ContenoxDir:              contenoxDir,
		EffectiveHITL:            true,
		EffectiveHITLService:     hitl,
		EffectiveAskApproval:     routedAskApproval(router, transportFn),
		EffectiveExtraTools: map[string]taskengine.ToolsRepo{
			localtools.GitToolsName: localtools.NewGitToolsWith("", localtools.GitToolsName,
				acpsvc.NewACPCwdResolver(func(context.Context) *acpsvc.Transport { return transport })),
		},
	})
	require.NoError(t, err)
	t.Cleanup(engine.Stop)

	t.Setenv("CONTENOX_ACP_CHAIN_PATH", generatedACPChain(t))
	chains, err := acpsvc.LoadChainRegistryFrom("chain-agent-acp.json", "CONTENOX_ACP_CHAIN_PATH")
	require.NoError(t, err)

	factory, err := buildWorkspaceFactory(workspace)
	require.NoError(t, err)
	workspaceID := ResolveWorkspaceID(contenoxDir)

	// The agent-side stream the transport writes to: a pipe this test drains.
	streamR, streamW := io.Pipe()
	sink := &notificationSink{}
	drained := make(chan struct{})
	go drainStream(streamR, sink, drained)

	// The inbound side is never written to in this test — a direct caller never
	// replies — so it is a pipe whose read half is closed and whose write half is
	// dropped with the connection.
	inboundR, inboundW := io.Pipe()
	_ = inboundR.Close()

	build := acpsvc.New(acpsvc.Deps{
		Engine:          engine,
		DB:              db,
		ChainRegistry:   chains,
		DefaultModel:    scriptedtest.DefaultModelName,
		DefaultProvider: modelrepo.ScriptedTestBackendType,
		WorkspaceID:     workspaceID,
		ContenoxDir:     contenoxDir,
		WorkspaceRoots:  factory,
		SessionRouter:   router,
		Asks:            hitl,
	})
	conn := libacp.NewAgentSideConnection(&duplexPipe{r: inboundR, w: streamW}, func(c *libacp.AgentSideConnection) libacp.Agent {
		agent := build(c)
		transport = agent.(*acpsvc.Transport)
		return agent
	})
	require.NotNil(t, conn)
	t.Cleanup(func() {
		_ = inboundW.Close()
		_ = streamW.Close()
		<-drained
	})

	return &inProcessRuntime{
		t:             t,
		db:            db,
		transport:     transport,
		contenoxDir:   contenoxDir,
		workspace:     workspace,
		workspaceID:   workspaceID,
		notifications: sink,
	}
}

// newSession opens a session by direct call.
func (r *inProcessRuntime) newSession(t *testing.T, cwd string) libacp.SessionID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := r.transport.NewSession(ctx, libacp.NewSessionRequest{Cwd: cwd})
	require.NoError(t, err)
	require.NotEmpty(t, resp.SessionID)
	return resp.SessionID
}

// prompt calls the transport directly. No connection on this side, no client, no
// JSON-RPC: this is the call a hosted handler makes.
func (r *inProcessRuntime) prompt(t *testing.T, sessionID libacp.SessionID, text string) libacp.PromptResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resp, err := r.transport.Prompt(ctx, libacp.PromptRequest{
		SessionID: sessionID,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent(text)},
	})
	require.NoError(t, err)
	return resp
}

// storedMessages reads the durable transcript for a session's internal id.
func (r *inProcessRuntime) storedMessages(t *testing.T, internalID string) []taskengine.Message {
	t.Helper()
	msgs, err := chatservice.NewManager(r.workspaceID).ListMessages(context.Background(), r.db.WithoutTransaction(), internalID)
	require.NoError(t, err)
	return msgs
}

// TestSystem_EmbeddedTurn_DirectCall is the whole claim in one test: a caller in
// this process opens a session and runs a turn with direct method calls, the
// reply streams to the connection, and the transcript lands in the store — with
// no ACP client, no socket and no bridge anywhere in the path.
func TestSystem_EmbeddedTurn_DirectCall(t *testing.T) {
	r := newInProcessRuntime(t, plainAnswerDialog, askEverything)

	sessionID := r.newSession(t, r.workspace)

	resp := r.prompt(t, sessionID, "summarize the greeting change")
	require.Equal(t, libacp.StopReasonEndTurn, resp.StopReason)

	// The reply streamed to the connection, which is the only place a direct
	// caller can observe it.
	require.Contains(t, r.notifications.text(), scriptedFinalAnswer,
		"the assistant's answer must reach the sink")

	// And the turn is durable: the transcript is in the store, which is what a
	// stateless deployment re-hydrates from.
	msgs := r.storedMessages(t, r.internalID(t, sessionID))
	require.NotEmpty(t, msgs, "the turn must leave a stored transcript")

	var sawUser, sawAssistant bool
	for _, m := range msgs {
		switch m.Role {
		case "user":
			sawUser = true
		case "assistant":
			sawAssistant = true
		}
	}
	require.True(t, sawUser, "the operator's prompt is part of the record")
	require.True(t, sawAssistant, "the assistant's answer is part of the record")
}

// plainAnswerDialog is one ungated turn: the embedded claim under test is the
// call path — direct Prompt, no client, no socket — not the approval loop, which
// has its own test.
const plainAnswerDialog = `{
  "model": "scripted-test",
  "turns": [
    {"text": "general"},
    {"text": "Suggested commit message: tighten the greeting copy."}
  ]
}`

// internalID maps the ACP session to the durable id the transcript is stored
// under. A hosted deployment persists this mapping alongside its own session
// row, which is what lets any replica rehydrate the session later.
func (r *inProcessRuntime) internalID(t *testing.T, sessionID libacp.SessionID) string {
	t.Helper()
	internal, ok := r.transport.InternalSessionID(sessionID)
	require.True(t, ok, "the ACP session must map to a durable session id")
	return internal
}

func TestSystem_EmbeddedTurn_MetersEveryModelCall(t *testing.T) {
	dialog := strings.ReplaceAll(plainAnswerDialog, `{"text":`, `{"usage":{"prompt_tokens":100,"completion_tokens":10},"text":`)
	r := newInProcessRuntime(t, dialog, askEverything)
	sessionID := r.newSession(t, r.workspace)
	response := r.prompt(t, sessionID, "summarize the greeting change")
	require.Equal(t, libacp.StopReasonEndTurn, response.StopReason)
	usage := runtimetypes.NewUsageStore(r.db)
	totals, err := usage.UsageByScope(context.Background(), runtimetypes.UsageScopeClient, gateway.LocalClientID)
	require.NoError(t, err)
	require.Len(t, totals, 1)
	require.EqualValues(t, 220, totals[0].Snapshot.TotalTokens)
	session, err := usage.UsageByScope(context.Background(), runtimetypes.UsageScopeSession,
		runtimetypes.SessionUsageScopeID(gateway.LocalClientID, r.internalID(t, sessionID)))
	require.NoError(t, err)
	require.Equal(t, totals, session)
}
