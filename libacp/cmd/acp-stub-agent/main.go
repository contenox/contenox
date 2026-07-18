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
//
// Opt-in behavior, off by default so the conformance suites see byte-identical
// output: when the environment variable ACP_STUB_ADVERTISE_COMMANDS=1 is set,
// NewSession advertises a deterministic available_commands_update right after
// its session/new result (scheduled via libacp.AfterResponse, the way
// claude-code-acp advertises its slash-command menu). This lets acpsvc's
// external-agent bridge be exercised end-to-end for the downstream slash-menu
// relay. With the variable unset the stub sends no such update.
//
// Two further opt-in flags, same precedent (default off, byte-identical), exercise
// acpsvc's downstream config-option pass-through:
//
//   - ACP_STUB_ADVERTISE_CONFIG_OPTIONS=1: NewSession carries a deterministic
//     configOptions list (a "stub-verbosity" select) IN its session/new response —
//     the way a real agent advertises its own pickers synchronously — and
//     SetSessionConfigOption updates the stored value, returns the updated set, and
//     emits a confirming config_option_update. This drives the seed pass-through and
//     the upstream→downstream set_config_option round-trip.
//   - ACP_STUB_CONFIG_OPTIONS_AFTER_NEW=1: NewSession instead emits the configOptions
//     as a deferred config_option_update AFTER its session/new result (no options in
//     the response), so acpsvc's pre-bind caching (config_option_update arriving
//     before the upstream session/new response is on the wire) is observable at the
//     wire, mirroring the command-menu ordering guarantee.
//
// A fifth opt-in flag, same precedent (default off, byte-identical), exercises
// acpsvc's downstream session-mode pass-through:
//
//   - ACP_STUB_ADVERTISE_MODES=1: NewSession carries a deterministic
//     SessionModeState (a Code/Ask pair, Code current) in its session/new response —
//     the way claude-code-acp advertises its session modes. This drives acpsvc's
//     mapping of the downstream modes onto its synthetic "contenox.agent-mode" config
//     option, and (with the always-registered SetSessionMode handler below emitting a
//     current_mode_update) the upstream set/relay round-trip. With the flag unset the
//     session/new response carries no modes, so an external session's toolbar stays
//     empty — the same byte-identical default the other flags observe.
//
// A sixth opt-in flag, same precedent (default off, byte-identical), exercises
// acpsvc's downstream terminal/* pass-through:
//
//   - ACP_STUB_USE_TERMINAL=1: every plain prompt runs a full terminal/* round trip
//     against the client — CreateTerminal (a shell echo), WaitForTerminalExit,
//     TerminalOutput, ReleaseTerminal — and reports the outcome as an
//     agent_message_chunk ("terminal-scenario termcap=… exit=… truncated=… output=…").
//     This drives acpsvc's externalBridge terminal implementation, which maps the
//     calls onto the runtime's shell-session machinery and streams the output into
//     beam's terminal panel. When the client withholds the terminal capability (no
//     shell manager), the scenario reports termcap=false and skips the round trip.
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

	stubConfigVerbosityID = "stub-verbosity"
	stubVerbosityLow      = "low"
	stubVerbosityHigh     = "high"
)

type stubSession struct {
	cwd                   string
	additionalDirectories []string
	modeID                string
	// verbosity is the current value of the stub's deterministic config option,
	// mutated by SetSessionConfigOption. Meaningful only when the stub advertises
	// config options.
	verbosity string
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

	// advertiseCommands, set from ACP_STUB_ADVERTISE_COMMANDS=1 at startup, opts
	// the stub into emitting an available_commands_update after session/new. Off
	// by default so the conformance suites keep seeing byte-identical output.
	advertiseCommands bool

	// advertiseConfigOptions (ACP_STUB_ADVERTISE_CONFIG_OPTIONS=1) opts the stub
	// into carrying a deterministic configOptions set in its session/new response
	// and honoring session/set_config_option. advertiseConfigOptionsAfterNew
	// (ACP_STUB_CONFIG_OPTIONS_AFTER_NEW=1) instead emits that set as a deferred
	// config_option_update after session/new, to exercise acpsvc's pre-bind caching.
	// Both default off (byte-identical output).
	advertiseConfigOptions         bool
	advertiseConfigOptionsAfterNew bool

	// advertiseModes (ACP_STUB_ADVERTISE_MODES=1) opts the stub into carrying a
	// deterministic SessionModeState in its session/new response, so acpsvc's
	// downstream-mode → synthetic-config-option mapping is exercised. Default off:
	// the session/new response then carries no modes (byte-identical output), which
	// is also why session/new's set_mode conformance check skips rather than runs
	// unless the flag is set. The SetSessionMode handler stays registered regardless.
	advertiseModes bool

	// useTerminal (ACP_STUB_USE_TERMINAL=1) opts the stub into running a full
	// terminal/* round trip on every plain prompt: create a terminal, wait for it
	// to exit, read its output, release it, then report the result as an
	// agent_message_chunk. It stands in for a downstream agent (claude-code-acp)
	// that runs shell commands through the client's terminals, exercising acpsvc's
	// externalBridge terminal implementation end to end. Default off (byte-identical).
	useTerminal bool
	// clientTerminal records whether the client advertised the terminal capability
	// at initialize, so the terminal scenario can report it (and gracefully skip the
	// round trip when the client withheld it, e.g. a server with no shell manager).
	clientTerminal atomic.Bool
}

func newStubAgent(c *libacp.AgentSideConnection) *stubAgent {
	return &stubAgent{
		conn:                           c,
		sessions:                       make(map[libacp.SessionID]*stubSession),
		advertiseCommands:              os.Getenv("ACP_STUB_ADVERTISE_COMMANDS") == "1",
		advertiseConfigOptions:         os.Getenv("ACP_STUB_ADVERTISE_CONFIG_OPTIONS") == "1",
		advertiseConfigOptionsAfterNew: os.Getenv("ACP_STUB_CONFIG_OPTIONS_AFTER_NEW") == "1",
		advertiseModes:                 os.Getenv("ACP_STUB_ADVERTISE_MODES") == "1",
		useTerminal:                    os.Getenv("ACP_STUB_USE_TERMINAL") == "1",
	}
}

// stubTerminalMarker is the deterministic string the terminal scenario's command
// prints, computed by the shell (echo stub-terminal-$((6*7))) so the literal
// "stub-terminal-42" appears only in the command's OUTPUT, never in the echoed
// command text — proof, when a test sees it, that the runtime shell actually ran
// the command rather than merely echoing it.
const stubTerminalMarker = "stub-terminal-42"

// stubModeState is the deterministic SessionModeState the stub advertises when
// ACP_STUB_ADVERTISE_MODES=1 — a Code/Ask pair (Code current) standing in for a
// real downstream agent's (e.g. claude-code-acp's) session modes.
func stubModeState() *libacp.SessionModeState {
	return &libacp.SessionModeState{
		CurrentModeID: stubModeCode,
		AvailableModes: []libacp.SessionMode{
			{ID: stubModeCode, Name: "Code", Description: "Full tool access"},
			{ID: stubModeAsk, Name: "Ask", Description: "Read-only, asks before acting"},
		},
	}
}

// stubConfigOptions is the deterministic config-option set the stub advertises
// when opted in — a single "verbosity" select standing in for a real downstream
// agent's own pickers. current is folded in as the option's CurrentValue.
func stubConfigOptions(current string) []libacp.SessionConfigOption {
	if current == "" {
		current = stubVerbosityLow
	}
	return []libacp.SessionConfigOption{{
		ID:           stubConfigVerbosityID,
		Name:         "Verbosity",
		Description:  "Stub: how chatty the stub agent is.",
		Category:     "stub",
		Type:         libacp.SessionConfigOptionTypeSelect,
		CurrentValue: current,
		Options: libacp.NewSessionConfigValues([]libacp.SessionConfigValue{
			{Value: stubVerbosityLow, Name: "Low"},
			{Value: stubVerbosityHigh, Name: "High"},
		}),
	}}
}

// stubAdvertisedCommands is the deterministic slash-command menu the stub
// advertises when ACP_STUB_ADVERTISE_COMMANDS=1, standing in for a real
// downstream agent's (e.g. claude-code-acp's) menu.
func stubAdvertisedCommands() []libacp.AvailableCommand {
	return []libacp.AvailableCommand{
		{Name: "review", Description: "Stub: review the current changes."},
		{Name: "explain", Description: "Stub: explain a file.", Input: &libacp.AvailableCommandInput{Hint: "[path]"}},
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
	// Remember whether the client offered the terminal capability, so the terminal
	// scenario can both report it and decline the round trip when it is absent.
	a.clientTerminal.Store(req.ClientCapabilities.Terminal)
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

func (a *stubAgent) NewSession(ctx context.Context, req libacp.NewSessionRequest) (libacp.NewSessionResponse, error) {
	id := libacp.SessionID(fmt.Sprintf("stub-session-%d", a.nextID.Add(1)))

	a.mu.Lock()
	a.sessions[id] = &stubSession{
		cwd:                   req.Cwd,
		additionalDirectories: req.AdditionalDirectories,
		modeID:                stubModeCode,
		verbosity:             stubVerbosityLow,
	}
	a.mu.Unlock()

	// Opt-in only. Deferred via AfterResponse so the update reaches the client
	// strictly after the session/new result — otherwise a conformant client drops
	// it as referencing a session id it has not yet learned. Default (unset) sends
	// nothing, so the conformance suites' expectations are unchanged.
	if a.advertiseCommands {
		libacp.AfterResponse(ctx, func() {
			_ = a.conn.SessionUpdate(libacp.SessionNotification{
				SessionID: id,
				Update: libacp.SessionUpdate{
					SessionUpdate:     libacp.SessionUpdateAvailableCommands,
					AvailableCommands: stubAdvertisedCommands(),
				},
			})
		})
	}

	// Config options advertised as a DEFERRED config_option_update after the
	// session/new result (no options in the response) — the pre-bind path that
	// exercises acpsvc's caching of an update the upstream client cannot resolve
	// yet.
	if a.advertiseConfigOptionsAfterNew {
		libacp.AfterResponse(ctx, func() {
			_ = a.conn.SessionUpdate(libacp.SessionNotification{
				SessionID: id,
				Update: libacp.SessionUpdate{
					SessionUpdate: libacp.SessionUpdateConfigOption,
					ConfigOptions: stubConfigOptions(stubVerbosityLow),
				},
			})
		})
	}

	resp := libacp.NewSessionResponse{SessionID: id}
	// Session modes carried IN the session/new response — opt-in (default off, so the
	// response is byte-identical to a bare agent), the way claude-code-acp advertises
	// its modes; drives acpsvc's synthetic mode-config-option mapping.
	if a.advertiseModes {
		resp.Modes = stubModeState()
	}
	// Config options carried IN the session/new response — the synchronous path a
	// real agent (were it to advertise pickers) uses, seeding acpsvc's cache with no
	// timing gap.
	if a.advertiseConfigOptions {
		resp.ConfigOptions = stubConfigOptions(stubVerbosityLow)
	}
	return resp, nil
}

// SetSessionConfigOption honors the stub's deterministic "verbosity" option when
// opted in: it validates the id/value, updates the session's stored value, returns
// the updated set, and emits a confirming config_option_update. With config options
// not advertised it reports MethodNotFound, matching the default (unimplemented)
// behavior so the conformance suites are unchanged.
func (a *stubAgent) SetSessionConfigOption(ctx context.Context, req libacp.SetSessionConfigOptionRequest) (libacp.SetSessionConfigOptionResponse, error) {
	if !a.advertiseConfigOptions {
		return libacp.SetSessionConfigOptionResponse{}, libacp.MethodNotFound(libacp.MethodSessionSetConfigOption)
	}
	if req.ConfigID != stubConfigVerbosityID {
		return libacp.SetSessionConfigOptionResponse{}, libacp.InvalidParams("unknown configId: " + req.ConfigID)
	}
	value := req.Value.AsString()
	if value != stubVerbosityLow && value != stubVerbosityHigh {
		return libacp.SetSessionConfigOptionResponse{}, libacp.InvalidParams("unknown value: " + value)
	}

	a.mu.Lock()
	sess, ok := a.sessions[req.SessionID]
	if ok {
		sess.verbosity = value
	}
	a.mu.Unlock()
	if !ok {
		return libacp.SetSessionConfigOptionResponse{}, libacp.InvalidParams("unknown sessionId: " + string(req.SessionID))
	}

	// Deferred so the confirming notification reaches the wire after this response,
	// matching the AfterResponse convention used above for set_mode.
	libacp.AfterResponse(ctx, func() {
		_ = a.conn.SessionUpdate(libacp.SessionNotification{
			SessionID: req.SessionID,
			Update: libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateConfigOption,
				ConfigOptions: stubConfigOptions(value),
			},
		})
	})

	return libacp.SetSessionConfigOptionResponse{ConfigOptions: stubConfigOptions(value)}, nil
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
	case a.useTerminal:
		return a.promptTerminal(ctx, req)
	default:
		return a.promptPlain(ctx, req)
	}
}

// promptTerminal drives a full terminal/* round trip against the client — the way
// a downstream agent (claude-code-acp) runs a shell command — and reports the
// result as a single agent_message_chunk so a test observing the upstream stream
// can assert on it. When the client withheld the terminal capability (no shell
// manager on the server) it reports that and skips the round trip, so the
// declined-capability path is observable too.
func (a *stubAgent) promptTerminal(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	if !a.clientTerminal.Load() {
		return a.terminalReport(req, "termcap=false")
	}
	if strings.Contains(promptText(req), "terminal_kill") {
		return a.promptTerminalKill(ctx, req)
	}

	createResp, err := a.conn.CreateTerminal(ctx, libacp.CreateTerminalRequest{
		SessionID: req.SessionID,
		Command:   "sh",
		// Compute the marker in the shell so the literal is in the OUTPUT only.
		Args: []string{"-c", "echo stub-terminal-$((6*7))"},
	})
	if err != nil {
		return a.terminalReport(req, "termcap=true create-error="+err.Error())
	}
	termID := createResp.TerminalID

	exit, err := a.conn.WaitForTerminalExit(ctx, libacp.WaitForTerminalExitRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	})
	if err != nil {
		return a.terminalReport(req, "termcap=true wait-error="+err.Error())
	}

	out, err := a.conn.TerminalOutput(ctx, libacp.TerminalOutputRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	})
	if err != nil {
		return a.terminalReport(req, "termcap=true output-error="+err.Error())
	}

	if _, err := a.conn.ReleaseTerminal(ctx, libacp.ReleaseTerminalRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	}); err != nil {
		return a.terminalReport(req, "termcap=true release-error="+err.Error())
	}

	exitStr := "nil"
	if exit.ExitCode != nil {
		exitStr = fmt.Sprintf("%d", *exit.ExitCode)
	} else if exit.Signal != nil {
		exitStr = "signal:" + *exit.Signal
	}
	report := fmt.Sprintf("termcap=true exit=%s truncated=%v output=%q", exitStr, out.Truncated, out.Output)
	return a.terminalReport(req, report)
}

// promptTerminalKill exercises the kill lifecycle: it starts a long-running
// command, kills it, then waits for (and reports) the resolved exit — proving the
// bridge interrupts the command and resolves WaitForTerminalExit promptly rather
// than blocking for the command's natural duration.
func (a *stubAgent) promptTerminalKill(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	createResp, err := a.conn.CreateTerminal(ctx, libacp.CreateTerminalRequest{
		SessionID: req.SessionID,
		Command:   "sh",
		Args:      []string{"-c", "sleep 30; echo should-not-appear"},
	})
	if err != nil {
		return a.terminalReport(req, "termcap=true kill create-error="+err.Error())
	}
	termID := createResp.TerminalID

	if _, err := a.conn.KillTerminal(ctx, libacp.KillTerminalRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	}); err != nil {
		return a.terminalReport(req, "termcap=true kill kill-error="+err.Error())
	}

	exit, err := a.conn.WaitForTerminalExit(ctx, libacp.WaitForTerminalExitRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	})
	if err != nil {
		return a.terminalReport(req, "termcap=true kill wait-error="+err.Error())
	}

	if _, err := a.conn.ReleaseTerminal(ctx, libacp.ReleaseTerminalRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	}); err != nil {
		return a.terminalReport(req, "termcap=true kill release-error="+err.Error())
	}

	exitStr := "nil"
	if exit.ExitCode != nil {
		exitStr = fmt.Sprintf("%d", *exit.ExitCode)
	} else if exit.Signal != nil {
		exitStr = "signal:" + *exit.Signal
	}
	return a.terminalReport(req, "kill exit="+exitStr)
}

// terminalReport emits the terminal scenario's outcome as one agent_message_chunk
// and ends the turn.
func (a *stubAgent) terminalReport(req libacp.PromptRequest, msg string) (libacp.PromptResponse, error) {
	if err := a.conn.SessionUpdate(libacp.SessionNotification{
		SessionID: req.SessionID,
		Update:    libacp.NewAgentMessageChunk("terminal-scenario " + msg),
	}); err != nil {
		return libacp.PromptResponse{}, err
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
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
