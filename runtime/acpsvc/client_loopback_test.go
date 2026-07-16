package acpsvc

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	libacp "github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/approvalflow"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

// This file drives the REAL production acpsvc Agent (Transport) through a
// REAL libacp.ClientSideConnection over an in-memory duplex pipe — both
// Run loops live, exactly as an editor and `contenox acp` would talk to each
// other, except the transport is io.Pipe instead of stdio. It replaces the
// raw-frame wireClient assertions in wire_e2e_test.go with the production
// client stack (client.go/clientconn.go) on the other end: the point is
// proving the two finished halves of libacp interoperate, not re-testing
// either one in isolation (that's what clientconn_test.go and this package's
// existing unit tests already do).
//
// Deps are mocked the same way the rest of this package's tests mock them:
// sessionEntry.Agent is swapped for a scripted agentservice.Agent double
// after a real session/new call (mirroring prompt_test.go's fakeAgent), and
// the event bus is libbus.NewInMem() (mirroring taskengine/mcpworker tests).
// There is no real LLM backend and no real chain execution engine anywhere
// in this file.

// loopbackClient is a minimal libacp.Client that answers the agent's reverse
// calls (session/request_permission, fs/*) deterministically instead of
// prompting a human, and buffers every session/update notification in wire
// order so tests can assert on the stream a real editor would render.
type loopbackClient struct {
	libacp.UnimplementedClient

	updates chan libacp.SessionNotification

	mu    sync.Mutex
	files map[string]string

	permMu   sync.Mutex
	permReqs []libacp.RequestPermissionRequest
	permResp libacp.RequestPermissionResponse
}

func newLoopbackClient() *loopbackClient {
	return &loopbackClient{
		updates: make(chan libacp.SessionNotification, 256),
		files:   make(map[string]string),
		permResp: libacp.RequestPermissionResponse{
			Outcome: libacp.RequestPermissionOutcome{
				Outcome:  libacp.PermissionOutcomeSelected,
				OptionID: approvalflow.OptionAllow,
			},
		},
	}
}

func (c *loopbackClient) SessionUpdate(_ context.Context, n libacp.SessionNotification) error {
	c.updates <- n
	return nil
}

func (c *loopbackClient) RequestPermission(_ context.Context, req libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error) {
	c.permMu.Lock()
	c.permReqs = append(c.permReqs, req)
	resp := c.permResp
	c.permMu.Unlock()
	return resp, nil
}

func (c *loopbackClient) setPermissionResponse(resp libacp.RequestPermissionResponse) {
	c.permMu.Lock()
	c.permResp = resp
	c.permMu.Unlock()
}

func (c *loopbackClient) lastPermissionRequest() (libacp.RequestPermissionRequest, bool) {
	c.permMu.Lock()
	defer c.permMu.Unlock()
	if len(c.permReqs) == 0 {
		return libacp.RequestPermissionRequest{}, false
	}
	return c.permReqs[len(c.permReqs)-1], true
}

func (c *loopbackClient) ReadTextFile(_ context.Context, req libacp.ReadTextFileRequest) (libacp.ReadTextFileResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	content, ok := c.files[req.Path]
	if !ok {
		return libacp.ReadTextFileResponse{}, &libacp.Error{Code: libacp.ErrResourceNotFound, Message: "no such file: " + req.Path}
	}
	return libacp.ReadTextFileResponse{Content: content}, nil
}

func (c *loopbackClient) WriteTextFile(_ context.Context, req libacp.WriteTextFileRequest) (libacp.WriteTextFileResponse, error) {
	c.mu.Lock()
	c.files[req.Path] = req.Content
	c.mu.Unlock()
	return libacp.WriteTextFileResponse{}, nil
}

// drain reads exactly n session/update notifications, in wire order, failing
// the test if they don't arrive within the deadline.
func (c *loopbackClient) drain(t *testing.T, n int) []libacp.SessionNotification {
	t.Helper()
	got := make([]libacp.SessionNotification, 0, n)
	deadline := time.After(5 * time.Second)
	for len(got) < n {
		select {
		case note := <-c.updates:
			got = append(got, note)
		case <-deadline:
			t.Fatalf("timed out waiting for %d session/update notifications (got %d: %+v)", n, len(got), got)
		}
	}
	return got
}

// loopbackAgent is an agentservice.Agent double whose Prompt behavior each
// test script directly — streaming events onto the bus, calling back into
// the Transport's client-facing seams (AskApproval, ACPFileIO), or blocking
// on ctx cancellation. Every other method is a no-op: session lifecycle
// itself is exercised through the real agentservice.Agent that NewSession
// already wired up (agentservice.New, DB-backed); only Prompt is swapped
// in afterward, mirroring prompt_test.go's fakeAgent one level up the stack.
type loopbackAgent struct {
	promptFunc func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error)
}

func (a *loopbackAgent) Capabilities(context.Context) (*agentservice.AgentCapabilities, error) {
	return nil, nil
}
func (a *loopbackAgent) SessionNew(context.Context, string) (string, error) { return "", nil }
func (a *loopbackAgent) SessionList(context.Context) ([]*agentservice.SessionInfo, error) {
	return nil, nil
}
func (a *loopbackAgent) SessionLoad(context.Context, string) (string, []taskengine.Message, error) {
	return "", nil, nil
}
func (a *loopbackAgent) SessionResume(context.Context, string) (string, error) { return "", nil }
func (a *loopbackAgent) SessionDelete(context.Context, string) error           { return nil }
func (a *loopbackAgent) SessionEnsureDefault(context.Context) (string, error)  { return "", nil }
func (a *loopbackAgent) Prompt(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	return a.promptFunc(ctx, req)
}

var _ agentservice.Agent = (*loopbackAgent)(nil)

// loopbackHarness wires the real production Transport (acpsvc's Agent
// implementation, New(Deps)) to a real libacp.ClientSideConnection over an
// in-memory duplex pipe, both Run loops live — the agent-side half of this
// is exactly wire_e2e_test.go's setup; the client-side half is what this
// slice adds.
type loopbackHarness struct {
	t      *testing.T
	tr     *Transport
	client *libacp.ClientSideConnection
	lc     *loopbackClient
	bus    *libbus.InMem
}

func newLoopbackHarness(t *testing.T) *loopbackHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "loopback.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)

	agentR, clientW := io.Pipe()
	clientR, agentW := io.Pipe()
	agentSide := &wirePipe{r: agentR, w: agentW}
	clientSide := &wirePipe{r: clientR, w: clientW}

	bus := libbus.NewInMem()
	factory := New(Deps{
		Engine:        &enginesvc.Engine{Bus: bus},
		DB:            db,
		ChainRegistry: &ChainRegistry{defaultChain: &taskengine.TaskChainDefinition{}},
		WorkspaceID:   "loopback-ws",
	})

	var tr *Transport
	agentConn := libacp.NewAgentSideConnection(agentSide, func(c *libacp.AgentSideConnection) libacp.Agent {
		a := factory(c)
		tr = a.(*Transport)
		return a
	})

	lc := newLoopbackClient()
	clientConn := libacp.NewClientSideConnection(clientSide, func(*libacp.ClientSideConnection) libacp.Client {
		return lc
	})

	agentDone := make(chan error, 1)
	clientDone := make(chan error, 1)
	go func() { agentDone <- agentConn.Run(ctx) }()
	go func() { clientDone <- clientConn.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		select {
		case <-agentDone:
		case <-time.After(2 * time.Second):
			t.Error("agent connection did not shut down")
		}
		select {
		case <-clientDone:
		case <-time.After(2 * time.Second):
			t.Error("client connection did not shut down")
		}
		require.NoError(t, db.Close())
	})

	return &loopbackHarness{t: t, tr: tr, client: clientConn, lc: lc, bus: bus}
}

// swapAgent installs a into sid's live sessionEntry, replacing the real
// agentservice.Agent that NewSession created for the duration of the test's
// Prompt calls — the same white-box seam prompt_test.go uses one layer down.
func (h *loopbackHarness) swapAgent(sid libacp.SessionID, a agentservice.Agent) {
	h.tr.sessionMu.Lock()
	h.tr.sessions[sid].Agent = a
	h.tr.sessionMu.Unlock()
}

// TestLoopback_InitializeAdvertisesSpecCapabilities proves the real client
// stack can complete "initialize" against the real Transport and pins the
// capability-honesty contract: session lifecycle capabilities the Transport
// actually implements are advertised, and additionalDirectories — which
// NewSession/LoadSession/ResumeSession never read — is not.
func TestLoopback_InitializeAdvertisesSpecCapabilities(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	resp, err := h.client.Initialize(ctx, libacp.InitializeRequest{
		ProtocolVersion: libacp.ProtocolVersion,
		ClientInfo:      &libacp.Implementation{Name: "loopback-test", Version: "0"},
	})
	require.NoError(t, err)
	require.Equal(t, libacp.ProtocolVersion, resp.ProtocolVersion)
	require.NotNil(t, resp.AgentCapabilities.SessionCapabilities.List)
	require.NotNil(t, resp.AgentCapabilities.SessionCapabilities.Resume)
	require.NotNil(t, resp.AgentCapabilities.SessionCapabilities.Close)
	require.NotNil(t, resp.AgentCapabilities.SessionCapabilities.Delete)
	require.Nil(t, resp.AgentCapabilities.SessionCapabilities.AdditionalDirectories,
		"acpsvc never reads NewSessionRequest.AdditionalDirectories (session.go), so advertising support would be dishonest")
}

// TestUnit_Initialize_DoesNotAdvertiseAdditionalDirectories is the fast,
// wire-free companion to the loopback check above: it pins the same
// capability-honesty verdict directly against Transport.Initialize.
func TestUnit_Initialize_DoesNotAdvertiseAdditionalDirectories(t *testing.T) {
	tr := &Transport{deps: Deps{Engine: &enginesvc.Engine{}}}
	resp, err := tr.Initialize(context.Background(), libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	require.Nil(t, resp.AgentCapabilities.SessionCapabilities.AdditionalDirectories)
}

// TestLoopback_Prompt_StreamsUpdatesThroughRealClient drives initialize ->
// session/new -> session/prompt end to end and proves a streamed turn — an
// assistant chunk, a tool call's pending/completed pair, and a token usage
// update — arrives at the real Client.SessionUpdate handler, alongside the
// session_info_update session/prompt always appends.
//
// Note on ordering: session_info_update is NOT last on the wire, even though
// prompt.go schedules it via libacp.AfterResponse "so it runs after the
// turn". For session/prompt specifically, the cancelable per-turn context
// conn.go's callMethod substitutes (promptCtx = pc.ctx, registered by
// registerPromptCancel before handleRequest ever attaches its
// after-response sink) does not carry that sink, so AfterResponse falls
// back to its synchronous "no sink in ctx" branch (conn.go's AfterResponse
// doc comment) and runs immediately — before the turn's own streamed
// events, which are flushed later, when Prompt's deferred bus-drain runs.
// That is existing, already-shipped behavior this test simply documents
// rather than assumes away; it is orthogonal to this slice's scope (libacp
// connection internals are out of bounds here) and asserted by kind, not
// position, below.
func TestLoopback_Prompt_StreamsUpdatesThroughRealClient(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)

	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: "/tmp/loopback-project", McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	require.NotEmpty(t, newResp.SessionID)
	h.lc.drain(t, 1) // deferred available_commands_update

	fake := &loopbackAgent{}
	fake.promptFunc = func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		reqID, _ := ctx.Value(libtracker.ContextKeyRequestID).(string)
		require.NotEmpty(t, reqID, "acpsvc stamps a request id onto the prompt ctx before calling the agent (prompt.go)")
		subject := taskengine.TaskEventRequestSubject(reqID)
		publish := func(ev taskengine.TaskEvent) {
			raw, mErr := json.Marshal(ev)
			require.NoError(t, mErr)
			require.NoError(t, h.bus.Publish(ctx, subject, raw))
		}
		publish(taskengine.TaskEvent{Kind: taskengine.TaskEventStepChunk, TaskHandler: string(taskengine.HandleChatCompletion), Content: "Hello from the real client stack."})
		publish(taskengine.TaskEvent{Kind: taskengine.TaskEventToolCallPending, ToolName: "local_fs.read_file", ApprovalID: "call-1", ApprovalArgs: map[string]any{"path": "/tmp/x.txt"}})
		publish(taskengine.TaskEvent{Kind: taskengine.TaskEventToolCall, ToolName: "local_fs.read_file", ApprovalID: "call-1", Content: `"file contents"`})
		publish(taskengine.TaskEvent{Kind: taskengine.TaskEventTokenUsage, TokenUsed: 12, TokenSize: 4096})
		return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
	}
	h.swapAgent(newResp.SessionID, fake)

	promptResp, err := h.client.Prompt(ctx, libacp.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent("hi")},
	})
	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, promptResp.StopReason)

	updates := h.lc.drain(t, 5)
	byKind := make(map[libacp.SessionUpdateKind]libacp.SessionUpdate, len(updates))
	for _, u := range updates {
		byKind[u.Update.SessionUpdate] = u.Update
	}

	require.Contains(t, byKind, libacp.SessionUpdateSessionInfo, "session/prompt always appends a session_info_update")

	chunk, ok := byKind[libacp.SessionUpdateAgentMessageChunk]
	require.True(t, ok)
	require.NotNil(t, chunk.Content)
	require.Equal(t, "Hello from the real client stack.", chunk.Content.Text)

	pending, ok := byKind[libacp.SessionUpdateToolCall]
	require.True(t, ok, "the first notification for a tool call must be create-shaped, not update-shaped")
	require.Equal(t, "call-1", pending.ToolCallID)
	require.Equal(t, libacp.ToolCallStatusPending, pending.Status)

	completed, ok := byKind[libacp.SessionUpdateToolCallUpdate]
	require.True(t, ok)
	require.Equal(t, "call-1", completed.ToolCallID)
	require.Equal(t, libacp.ToolCallStatusCompleted, completed.Status)

	usage, ok := byKind[libacp.SessionUpdateUsageUpdate]
	require.True(t, ok)
	require.Equal(t, 12, usage.Used)
	require.Equal(t, 4096, usage.Size)
}

// TestLoopback_CancelPrompt_ResolvesStopReasonCancelled cancels a prompt
// turn mid-flight through the real client's CancelPrompt and proves the
// production agent side resolves it with stopReason "cancelled" and no
// JSON-RPC error, per the spec's cancellation contract.
func TestLoopback_CancelPrompt_ResolvesStopReasonCancelled(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: "/tmp/loopback-cancel", McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	h.lc.drain(t, 1)

	started := make(chan struct{})
	var startOnce sync.Once
	fake := &loopbackAgent{promptFunc: func(ctx context.Context, _ agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			// Mirrors the real agentservice.Agent's own cancellation behavior
			// (see TestUnit_Prompt_CancelledStopReasonReturnsNilError).
			return &agentservice.PromptResponse{StopReason: agentservice.StopCancelled}, ctx.Err()
		case <-time.After(5 * time.Second):
			return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
		}
	}}
	h.swapAgent(newResp.SessionID, fake)

	type result struct {
		resp libacp.PromptResponse
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		resp, err := h.client.Prompt(ctx, libacp.PromptRequest{
			SessionID: newResp.SessionID,
			Prompt:    []libacp.ContentBlock{libacp.NewTextContent("please cancel me")},
		})
		resultCh <- result{resp, err}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not reach the fake agent")
	}

	require.NoError(t, h.client.CancelPrompt(newResp.SessionID))

	select {
	case r := <-resultCh:
		require.NoError(t, r.err, "ACP spec: cancellation must not surface as a JSON-RPC error")
		require.Equal(t, libacp.StopReasonCancelled, r.resp.StopReason)
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not resolve after CancelPrompt")
	}
}

// TestLoopback_Prompt_PermissionRoundTripThroughRealClient exercises the
// permission client-callback flow reachable with mocked deps: the fake
// agent calls Transport.AskApproval directly — standing in for the real
// engine's HITL wrapper (localtools.NewHITLWrapper, wired in
// runtime/enginesvc/engine.go, which calls exactly this method when a gated
// tool call is hit mid-chain execution) — to prove the session/
// request_permission round trip works end to end against a real
// ClientSideConnection, for both the allow and the deny outcome.
func TestLoopback_Prompt_PermissionRoundTripThroughRealClient(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: "/tmp/loopback-perm", McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	h.lc.drain(t, 1)

	var approvalErr error
	var allowed bool
	fake := &loopbackAgent{promptFunc: func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		approveCtx := context.WithValue(ctx, runtimetypes.SessionIDContextKey, req.SessionID)
		allowed, approvalErr = h.tr.AskApproval(approveCtx, hitlservice.ApprovalRequest{
			ToolCallID: "call-perm-1",
			ToolName:   "local_shell.exec",
			Args:       map[string]any{"command": "rm -rf /tmp/x"},
		})
		return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
	}}
	h.swapAgent(newResp.SessionID, fake)

	// The client answers "allow".
	h.lc.setPermissionResponse(libacp.RequestPermissionResponse{
		Outcome: libacp.RequestPermissionOutcome{Outcome: libacp.PermissionOutcomeSelected, OptionID: approvalflow.OptionAllow},
	})
	promptResp, err := h.client.Prompt(ctx, libacp.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent("do the thing")},
	})
	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, promptResp.StopReason)
	require.NoError(t, approvalErr)
	require.True(t, allowed, "client answered allow_once; AskApproval must resolve true")

	req, ok := h.lc.lastPermissionRequest()
	require.True(t, ok, "the real client must have received session/request_permission")
	require.Equal(t, newResp.SessionID, req.SessionID)
	require.Equal(t, "call-perm-1", req.ToolCall.ToolCallID)

	// The client rejects the next request.
	h.lc.setPermissionResponse(libacp.RequestPermissionResponse{
		Outcome: libacp.RequestPermissionOutcome{Outcome: libacp.PermissionOutcomeSelected, OptionID: approvalflow.OptionDeny},
	})
	_, err = h.client.Prompt(ctx, libacp.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent("do it again")},
	})
	require.NoError(t, err)
	require.NoError(t, approvalErr)
	require.False(t, allowed, "client answered reject; AskApproval must resolve false")
}

// TestLoopback_Prompt_FSReadWriteThroughRealClient exercises the other
// mocked-deps-reachable client-callback flow: fs/read_text_file and
// fs/write_text_file through acpsvc's ACPFileIO (fileio.go), which routes
// through Transport.conn exactly like AskApproval routes through it for
// permissions.
func TestLoopback_Prompt_FSReadWriteThroughRealClient(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{
		ProtocolVersion: libacp.ProtocolVersion,
		ClientCapabilities: libacp.ClientCapabilities{
			FS: libacp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
		},
	})
	require.NoError(t, err)
	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: "/tmp/loopback-fs", McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	h.lc.drain(t, 1)

	h.lc.mu.Lock()
	h.lc.files["/tmp/loopback-fs/note.txt"] = "hello from the client"
	h.lc.mu.Unlock()

	fio := NewACPFileIO(func() *Transport { return h.tr })
	var readBack []byte
	fake := &loopbackAgent{promptFunc: func(ctx context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
		approveCtx := context.WithValue(ctx, runtimetypes.SessionIDContextKey, req.SessionID)
		var readErr, writeErr error
		readBack, readErr = fio.ReadFile(approveCtx, "/tmp/loopback-fs/note.txt")
		require.NoError(t, readErr)
		writeErr = fio.WriteFile(approveCtx, "/tmp/loopback-fs/written.txt", []byte("hello from the agent"))
		require.NoError(t, writeErr)
		return &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}, nil
	}}
	h.swapAgent(newResp.SessionID, fake)

	_, err = h.client.Prompt(ctx, libacp.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent("read and write")},
	})
	require.NoError(t, err)
	require.Equal(t, "hello from the client", string(readBack))

	h.lc.mu.Lock()
	written := h.lc.files["/tmp/loopback-fs/written.txt"]
	h.lc.mu.Unlock()
	require.Equal(t, "hello from the agent", written)
}

// TestLoopback_SetSessionConfigOption_RoundTripThroughRealClient drives
// session/set_config_option through the real client and proves the change
// (here, the "think" level) is both reflected in the response and durably
// applied to the session's live state.
func TestLoopback_SetSessionConfigOption_RoundTripThroughRealClient(t *testing.T) {
	h := newLoopbackHarness(t)
	ctx := context.Background()

	_, err := h.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	newResp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: "/tmp/loopback-config", McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	h.lc.drain(t, 1)

	thinkOption := func(options []libacp.SessionConfigOption) libacp.SessionConfigOption {
		t.Helper()
		for _, o := range options {
			if o.ID == configIDThink {
				return o
			}
		}
		t.Fatalf("think config option missing from %#v", options)
		return libacp.SessionConfigOption{}
	}
	require.Equal(t, "high", thinkOption(newResp.ConfigOptions).CurrentValue, "session/new's default think level")

	setResp, err := h.client.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: newResp.SessionID,
		ConfigID:  configIDThink,
		Value:     libacp.StringConfigValue("xhigh"),
	})
	require.NoError(t, err)
	require.Equal(t, "xhigh", thinkOption(setResp.ConfigOptions).CurrentValue)

	h.tr.sessionMu.Lock()
	sess := h.tr.sessions[newResp.SessionID]
	h.tr.sessionMu.Unlock()
	require.Equal(t, "xhigh", sess.think(), "the change must durably apply to the session's live state")
}
