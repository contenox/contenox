// Command acp-stub-agent is a hermetic ACP Agent used to validate libacp's
// agent-side wire dispatch (conn.go, agent.go) against the Rust
// conformance-checking clients (acp-validator, yopo) without needing any LLM
// backend. It speaks ACP v1 over stdio exactly like the production `contenox
// acp`/`acpx` commands (runtime/contenoxcli/acp_cmd.go), but every response is
// deterministic and in-memory.
//
// The trigger text embedded in a session/prompt request selects which
// scenario runs (mirroring acp-validator's --*-trigger flags, whose defaults
// are `{"command":"run_scenario","scenario":"callbacks"}` and
// `{"command":"run_scenario","scenario":"session_updates"}`):
//
//   - trigger contains "session_updates": streams an agent message chunk, a
//     tool_call, and a tool_call_update before resolving the turn.
//   - trigger contains "callbacks": requests a permission from the client and,
//     if granted, exercises fs/read_text_file and fs/write_text_file. A
//     Cancelled permission outcome ends the turn gracefully instead of
//     hanging; a session/cancel mid-call propagates context.Canceled so
//     libacp's dispatcher resolves the turn with stopReason "cancelled".
//   - anything else: acks with a single message chunk and ends the turn. This
//     is also the path exercised by acp-validator's unknown_method liveness
//     check (trigger "ping").
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/contenox/runtime/libacp"
)

// stdio adapts the process's stdin/stdout to the io.ReadWriteCloser
// NewAgentSideConnection wants, matching contenoxcli's acpStdio.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }

func main() {
	conn := libacp.NewAgentSideConnection(stdio{}, func(c *libacp.AgentSideConnection) libacp.Agent {
		return newStubAgent(c)
	})
	if err := conn.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "acp-stub-agent: %v\n", err)
		os.Exit(1)
	}
}

const (
	stubAuthMethodID = "stub-auth"

	stubModeCode = "code"
	stubModeAsk  = "ask"
)

type stubSession struct {
	cwd                   string
	additionalDirectories []string
	modeID                string
}

type stubAgent struct {
	libacp.UnimplementedAgent

	conn *libacp.AgentSideConnection

	mu       sync.Mutex
	sessions map[libacp.SessionID]*stubSession

	nextID      atomic.Int64
	nextToolID  atomic.Int64
	authedOnce  atomic.Bool
	loggedOutOK atomic.Bool
}

func newStubAgent(c *libacp.AgentSideConnection) *stubAgent {
	return &stubAgent{
		conn:     c,
		sessions: make(map[libacp.SessionID]*stubSession),
	}
}

// negotiateProtocolVersion mirrors runtime/acpsvc's negotiateProtocolVersion:
// the spec requires echoing the requested version when supported, and
// otherwise falling back to the latest version this Agent implements —
// never echoing an unsupported version back verbatim.
func negotiateProtocolVersion(client int) int {
	if client >= 1 && client <= libacp.ProtocolVersion {
		return client
	}
	return libacp.ProtocolVersion
}

func (a *stubAgent) Initialize(_ context.Context, req libacp.InitializeRequest) (libacp.InitializeResponse, error) {
	return libacp.InitializeResponse{
		ProtocolVersion: negotiateProtocolVersion(req.ProtocolVersion),
		AgentInfo: &libacp.Implementation{
			Name:    "acp-stub-agent",
			Title:   "libacp conformance stub",
			Version: "0.0.1",
		},
		AgentCapabilities: libacp.AgentCapabilities{
			LoadSession: false,
			PromptCapabilities: libacp.PromptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: req.ClientCapabilities.FS.ReadTextFile,
			},
			McpCapabilities: libacp.McpCapabilities{
				HTTP: false,
				SSE:  false,
			},
			SessionCapabilities: libacp.SessionCapabilities{
				// Advertised (and honored in NewSession below) so the
				// validator's session_new_additional_directories check runs
				// instead of skipping.
				AdditionalDirectories: &struct{}{},
			},
			Auth: libacp.AgentAuthCapabilities{
				Logout: &libacp.LogoutCapabilities{},
			},
		},
		AuthMethods: []libacp.AuthMethod{
			{
				ID:          stubAuthMethodID,
				Name:        "Stub Auth",
				Description: "Always-succeeds auth method for conformance testing.",
			},
		},
	}, nil
}

func (a *stubAgent) Authenticate(_ context.Context, _ libacp.AuthenticateRequest) (libacp.AuthenticateResponse, error) {
	a.authedOnce.Store(true)
	return libacp.AuthenticateResponse{}, nil
}

func (a *stubAgent) Logout(_ context.Context, _ libacp.LogoutRequest) (libacp.LogoutResponse, error) {
	a.loggedOutOK.Store(true)
	return libacp.LogoutResponse{}, nil
}

func (a *stubAgent) NewSession(_ context.Context, req libacp.NewSessionRequest) (libacp.NewSessionResponse, error) {
	id := libacp.SessionID(fmt.Sprintf("stub-session-%d", a.nextID.Add(1)))

	a.mu.Lock()
	a.sessions[id] = &stubSession{
		cwd:                   req.Cwd,
		additionalDirectories: req.AdditionalDirectories,
		modeID:                stubModeCode,
	}
	a.mu.Unlock()

	return libacp.NewSessionResponse{
		SessionID: id,
		Modes: &libacp.SessionModeState{
			CurrentModeID: stubModeCode,
			AvailableModes: []libacp.SessionMode{
				{ID: stubModeCode, Name: "Code", Description: "Full tool access"},
				{ID: stubModeAsk, Name: "Ask", Description: "Read-only, asks before acting"},
			},
		},
	}, nil
}

func (a *stubAgent) SetSessionMode(ctx context.Context, req libacp.SetSessionModeRequest) (libacp.SetSessionModeResponse, error) {
	a.mu.Lock()
	sess, ok := a.sessions[req.SessionID]
	if ok {
		sess.modeID = req.ModeID
	}
	a.mu.Unlock()
	if !ok {
		return libacp.SetSessionModeResponse{}, libacp.InvalidParams("unknown sessionId: " + string(req.SessionID))
	}

	// Deferred so the confirming notification always reaches the wire after
	// this response, matching the AfterResponse convention documented in
	// conn.go for state-changing requests.
	libacp.AfterResponse(ctx, func() {
		_ = a.conn.SessionUpdate(libacp.SessionNotification{
			SessionID: req.SessionID,
			Update: libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateCurrentMode,
				CurrentModeID: req.ModeID,
			},
		})
	})

	return libacp.SetSessionModeResponse{}, nil
}

func (a *stubAgent) sessionCwd(id libacp.SessionID) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if sess, ok := a.sessions[id]; ok && sess.cwd != "" {
		return sess.cwd
	}
	return os.TempDir()
}

func promptText(req libacp.PromptRequest) string {
	var sb strings.Builder
	for _, block := range req.Prompt {
		if block.Type == string(libacp.ContentKindText) {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

func (a *stubAgent) Prompt(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	text := promptText(req)
	switch {
	case strings.Contains(text, "session_updates"):
		return a.promptStreaming(ctx, req)
	case strings.Contains(text, "callbacks"):
		return a.promptCallbacks(ctx, req)
	default:
		return a.promptPlain(ctx, req)
	}
}

func (a *stubAgent) promptPlain(_ context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	if err := a.conn.SessionUpdate(libacp.SessionNotification{
		SessionID: req.SessionID,
		Update:    libacp.NewAgentMessageChunk("ack"),
	}); err != nil {
		return libacp.PromptResponse{}, err
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}

// promptStreaming exercises the message-chunk / tool_call / tool_call_update
// session/update sequence the prompt_streaming and update_ordering checks
// look for — all emitted synchronously before the handler returns, so they
// reach the wire strictly before the session/prompt response.
func (a *stubAgent) promptStreaming(_ context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	toolCallID := fmt.Sprintf("stub-tool-%d", a.nextToolID.Add(1))

	updates := []libacp.SessionUpdate{
		libacp.NewAgentMessageChunk("running scenario..."),
		{
			SessionUpdate: libacp.SessionUpdateToolCall,
			ToolCallID:    toolCallID,
			Title:         "stub tool call",
			Kind:          libacp.ToolKindExecute,
			Status:        libacp.ToolCallStatusInProgress,
		},
		{
			SessionUpdate: libacp.SessionUpdateToolCallUpdate,
			ToolCallID:    toolCallID,
			Status:        libacp.ToolCallStatusCompleted,
		},
		libacp.NewAgentMessageChunk("done"),
	}
	for _, u := range updates {
		if err := a.conn.SessionUpdate(libacp.SessionNotification{SessionID: req.SessionID, Update: u}); err != nil {
			return libacp.PromptResponse{}, err
		}
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}

// promptCallbacks requests a client permission and, once granted, drives an
// fs/read_text_file + fs/write_text_file round trip — covering
// permission_roundtrip, fs_callbacks, and (via session/cancel arriving while
// the RequestPermission call is in flight) the cancel check, all with a
// single trigger scenario.
func (a *stubAgent) promptCallbacks(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	if err := a.conn.SessionUpdate(libacp.SessionNotification{
		SessionID: req.SessionID,
		Update:    libacp.NewAgentMessageChunk("requesting permission..."),
	}); err != nil {
		return libacp.PromptResponse{}, err
	}

	toolCallID := fmt.Sprintf("stub-tool-%d", a.nextToolID.Add(1))
	permResp, err := a.conn.RequestPermission(ctx, libacp.RequestPermissionRequest{
		SessionID: req.SessionID,
		ToolCall: libacp.PermissionToolCall{
			ToolCallID: toolCallID,
			Title:      "write scratch file",
			Kind:       libacp.ToolKindEdit,
			Status:     libacp.ToolCallStatusPending,
		},
		Options: []libacp.PermissionOption{
			{OptionID: "allow-once", Name: "Allow once", Kind: libacp.PermissionAllowOnce},
			{OptionID: "reject-once", Name: "Reject once", Kind: libacp.PermissionRejectOnce},
		},
	})
	if err != nil {
		// A session/cancel mid-call surfaces here as ctx's own cancellation;
		// propagate it so conn.go's dispatch resolves the turn with
		// stopReason "cancelled" instead of a JSON-RPC error.
		if ctx.Err() != nil {
			return libacp.PromptResponse{}, ctx.Err()
		}
		return libacp.PromptResponse{}, err
	}

	if permResp.Outcome.Outcome == libacp.PermissionOutcomeCancelled {
		return libacp.PromptResponse{StopReason: libacp.StopReasonRefusal}, nil
	}

	path := a.sessionCwd(req.SessionID) + "/acp-stub-scratch.txt"
	if _, err := a.conn.WriteTextFile(ctx, libacp.WriteTextFileRequest{
		SessionID: req.SessionID,
		Path:      path,
		Content:   "acp-stub-agent scratch content\n",
	}); err != nil {
		if ctx.Err() != nil {
			return libacp.PromptResponse{}, ctx.Err()
		}
		return libacp.PromptResponse{}, err
	}

	if _, err := a.conn.ReadTextFile(ctx, libacp.ReadTextFileRequest{
		SessionID: req.SessionID,
		Path:      path,
	}); err != nil {
		if ctx.Err() != nil {
			return libacp.PromptResponse{}, ctx.Err()
		}
		return libacp.PromptResponse{}, err
	}

	if err := a.conn.SessionUpdate(libacp.SessionNotification{
		SessionID: req.SessionID,
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateToolCallUpdate,
			ToolCallID:    toolCallID,
			Status:        libacp.ToolCallStatusCompleted,
		},
	}); err != nil {
		return libacp.PromptResponse{}, err
	}

	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}
