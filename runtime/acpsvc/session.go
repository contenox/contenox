package acpsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
)

// sessionListTitleMaxLen bounds SessionInfo.Title derived from a session's
// first user message, so a humane session picker never has to render a
// multi-paragraph prompt as its label.
const sessionListTitleMaxLen = 60

// truncateSessionListTitle collapses whitespace and clips to
// sessionListTitleMaxLen runes, appending an ellipsis when it clips.
func truncateSessionListTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= sessionListTitleMaxLen {
		return s
	}
	if sessionListTitleMaxLen <= 3 {
		return string(runes[:sessionListTitleMaxLen])
	}
	return string(runes[:sessionListTitleMaxLen-3]) + "..."
}

const mcpNamePrefix = runtimetypes.ACPMCPServerNamePrefix

func mcpNameFor(connectionID string, sessionID libacp.SessionID, original string) string {
	sum := sha256.Sum256([]byte(connectionID + "\x00" + string(sessionID) + "\x00" + original))
	hash := hex.EncodeToString(sum[:])[:12]
	return mcpNamePrefix + hash + "-" + sanitizeMCPNameComponent(original)
}

func sanitizeMCPNameComponent(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '_' || r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
		if sb.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(sb.String(), "_-")
	if out == "" {
		return "mcp"
	}
	return out
}

func (t *Transport) LoadSession(ctx context.Context, req libacp.LoadSessionRequest) (libacp.LoadSessionResponse, error) {
	reportErr, reportChange, end := t.tracker().Start(ctx, "load", "acp_session", "session_id", string(req.SessionID))
	defer end()

	if req.SessionID == "" {
		err := libacp.NewError(libacp.ErrInvalidParams, "sessionId is required")
		reportErr(err)
		return libacp.LoadSessionResponse{}, err
	}
	if !filepath.IsAbs(req.Cwd) {
		err := libacp.NewErrorf(libacp.ErrInvalidParams, "cwd must be an absolute path, got %q", req.Cwd)
		reportErr(err)
		return libacp.LoadSessionResponse{}, err
	}
	if t.deps.Engine == nil {
		err := errSetupRequired()
		reportErr(err)
		return libacp.LoadSessionResponse{}, err
	}
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	sessionCwd, err := t.resolveExistingSessionCwd(ctx, store, req.SessionID, req.Cwd)
	if err != nil {
		reportErr(err)
		return libacp.LoadSessionResponse{}, err
	}
	workspaceID := t.workspaceID()
	if resolved, ok := t.resolveSessionWorkspace(ctx, string(req.SessionID)); ok {
		workspaceID = resolved
	}

	registered, err := t.registerMcpServers(ctx, store, req.SessionID, req.McpServers)
	if err != nil {
		reportErr(err)
		return libacp.LoadSessionResponse{}, err
	}

	ag := agentservice.New(agentservice.Deps{
		Engine:      t.deps.Engine,
		DB:          t.deps.DB,
		WorkspaceID: workspaceID,
		Identity:    "acp-client",
	})

	contenoxSessionID, messages, err := ag.SessionLoad(ctx, string(req.SessionID))
	if err != nil {
		t.cleanupMcpServers(ctx, store, registered)
		wrapped := libacp.NewErrorf(libacp.ErrInvalidParams, "load session %q: %v", req.SessionID, err)
		reportErr(wrapped)
		return libacp.LoadSessionResponse{}, wrapped
	}

	entry := &sessionEntry{
		WorkspaceID:       workspaceID,
		Cwd:               sessionCwd,
		InternalSessionID: contenoxSessionID,
		McpServerNames:    registered,
		driver:            &nativeDriver{t: t, agent: ag},
		Provider:          t.provider(),
		Model:             t.model(),
		Think:             t.thinkDefault(),
		HITLPolicy:        hitlPolicyDefaultValue,
	}
	t.sessionMu.Lock()
	t.sessions[req.SessionID] = entry
	t.bindContenoxSession(contenoxSessionID, req.SessionID)
	t.sessionMu.Unlock()
	t.persistSessionCwd(ctx, store, req.SessionID, sessionCwd)
	// Re-flag an external session from its persisted agent name so its config
	// options come back minimal and the next prompt routes to the downstream
	// agent (lazily respawned). The transcript is replayed below either way; the
	// downstream process is deliberately NOT resurrected during load.
	t.markExternalIfPersisted(ctx, store, req.SessionID, entry)

	t.clearToolCallState(req.SessionID)
	t.subscribeTerminal(req.SessionID, contenoxSessionID)
	t.replayMessages(ctx, req.SessionID, messages)
	// Emit the slash-command menu only after the session/load result is on the wire
	// (see sendAvailableCommands) so the client can resolve the session. A native
	// session emits its contenox menu (and banner). An external session has no native
	// menu (AvailableCommands is nil); its downstream agent's menu — dead with the
	// pre-load connection and not resurrected until the next prompt — is re-emitted
	// from the values persisted at session/new, so the reopened session shows it
	// without a first prompt.
	if _, isExternal := entry.driver.(*externalDriver); isExternal {
		t.reemitExternalCommandMenu(ctx, store, req.SessionID)
	} else if entry.driver.AvailableCommands() != nil {
		libacp.AfterResponse(ctx, func() {
			t.sendAvailableCommands(ctx, req.SessionID)
			if banner := t.takeBanner(); banner != "" {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: req.SessionID,
					Update:    libacp.NewAgentMessageChunk(banner),
				})
			}
		})
	}

	reportChange(string(req.SessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
		"message_count":       len(messages),
	})
	return libacp.LoadSessionResponse{ConfigOptions: t.reloadedConfigOptions(ctx, store, req.SessionID, entry)}, nil
}

func (t *Transport) replayMessages(ctx context.Context, sessionID libacp.SessionID, messages []taskengine.Message) {
	_, reportChange, end := t.tracker().Start(ctx, "replay", "acp_session", "session_id", string(sessionID), "message_count", len(messages))
	defer end()

	var users, assistantText, toolCalls, toolResults int
	// One messageId per historical message: the spec groups replayed chunks by
	// id, so thinking + text of one assistant turn render as one message and a
	// change of id marks the next.
	for i, m := range messages {
		messageID := fmt.Sprintf("replay-%d", i)
		switch m.Role {
		case "user":
			if m.Content == "" {
				continue
			}
			update := libacp.NewUserMessageChunk(m.Content)
			update.MessageID = messageID
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sessionID,
				Update:    update,
			})
			users++
		case "assistant":
			if m.Thinking != "" {
				update := libacp.NewAgentThoughtChunk(m.Thinking)
				update.MessageID = messageID
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sessionID,
					Update:    update,
				})
			}
			if m.Content != "" {
				update := libacp.NewAgentMessageChunk(m.Content)
				update.MessageID = messageID
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sessionID,
					Update:    update,
				})
				assistantText++
			}
			for _, tc := range m.CallTools {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sessionID,
					Update:    toolCallUpdateFromCall(tc),
				})
				toolCalls++
			}
		case "tool":
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sessionID,
				Update:    toolCallUpdateFromResult(m),
			})
			toolResults++
		}
	}
	reportChange(string(sessionID), map[string]any{
		"user":         users,
		"assistant":    assistantText,
		"tool_calls":   toolCalls,
		"tool_results": toolResults,
	})

	t.sendInitialUsageUpdate(ctx, sessionID)
}

func toolCallUpdateFromCall(tc taskengine.ToolCall) libacp.SessionUpdate {
	title := tc.Function.Name
	var argsMap map[string]any
	if tc.Function.Arguments != "" && json.Valid([]byte(tc.Function.Arguments)) {
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsMap)
	}
	if summary := summarizeToolCallArgs(tc.Function.Name, argsMap); summary != "" {
		title = tc.Function.Name + ": " + summary
	}
	update := libacp.SessionUpdate{
		SessionUpdate: libacp.SessionUpdateToolCall,
		ToolCallID:    tc.ID,
		Title:         title,
		Kind:          toolKindFor(tc.Function.Name),
		Status:        libacp.ToolCallStatusCompleted,
	}
	if tc.Function.Arguments != "" && json.Valid([]byte(tc.Function.Arguments)) {
		update.RawInput = json.RawMessage(tc.Function.Arguments)
	}
	return update
}

func toolCallUpdateFromResult(m taskengine.Message) libacp.SessionUpdate {
	update := libacp.SessionUpdate{
		SessionUpdate: libacp.SessionUpdateToolCallUpdate,
		ToolCallID:    m.ToolCallID,
		Status:        libacp.ToolCallStatusCompleted,
	}
	if m.Content != "" {
		update.RawOutput = json.RawMessage(jsonString(m.Content))
		if diff := diffContentFromResult(m.Content); diff != nil {
			update.ToolContent = []libacp.ToolCallContent{*diff}
		}
	}
	return update
}

// errSetupRequired is returned by session operations when the transport is
// running setup-only (no default-model was configured at launch, so the engine
// is nil). It gives the ACP client an actionable message instead of letting the
// nil engine panic on first use. initialize/authenticate stay available so the
// "Setup Contenox" terminal auth method (or the env_var method) can configure a
// model. The code is the spec's -32000 auth_required: a conformant client
// reacts by offering the advertised auth methods, which is exactly the setup
// flow.
func errSetupRequired() error {
	return libacp.NewError(libacp.ErrAuthRequired, "contenox is not configured yet: no default-model is set. Run the \"Setup Contenox\" auth method, set the CONTENOX_DEFAULT_* environment variables (or run `contenox acp --setup`), then reconnect.")
}

// resolveWorkspaceCwd validates a requested session cwd against the workspace
// allowlist and returns the concrete root the session will use. When no
// allowlist is configured (Deps.WorkspaceRoots nil — the stdio ACP path) the
// cwd is returned unchanged, preserving the editor-owns-the-filesystem
// behavior. When an allowlist IS configured (serve), the sentinel cwd "/" (what
// beam sends today) and an empty cwd resolve to the default root so existing
// clients keep working; any other value must be an allowlisted root, else the
// request is refused with an actionable error.
func (t *Transport) resolveWorkspaceCwd(cwd string) (string, error) {
	f := t.deps.WorkspaceRoots
	if f == nil {
		return cwd, nil
	}
	resolved, err := f.Resolve(cwd)
	if err != nil {
		return "", libacp.NewErrorf(libacp.ErrInvalidParams,
			"workspace directory %q is not permitted; choose one of the configured workspace roots", cwd)
	}
	return resolved, nil
}

// resolveExistingSessionCwd resolves the cwd for session/load and session/resume
// on an existing session. A concrete requested cwd is validated and adopted (a
// client may re-root a session it owns to another allowlisted directory). The
// sentinel "/" or empty cwd — what beam sends on every load/resume — must NOT
// clobber the session's stored workspace back to the default; it preserves the
// persisted cwd (when still allowlisted), falling back to the default only when
// the session has none. When no allowlist is configured the requested cwd is
// returned unchanged (stdio behavior).
func (t *Transport) resolveExistingSessionCwd(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID, cwd string) (string, error) {
	f := t.deps.WorkspaceRoots
	if f == nil {
		return cwd, nil
	}
	if cwd != "" && cwd != "/" {
		resolved, err := f.Resolve(cwd)
		if err != nil {
			return "", libacp.NewErrorf(libacp.ErrInvalidParams,
				"workspace directory %q is not permitted; choose one of the configured workspace roots", cwd)
		}
		return resolved, nil
	}
	if existing := t.sessionCwd(ctx, store, sid); existing != "" {
		if resolved, ok := f.Allows(existing); ok {
			return resolved, nil
		}
	}
	return f.Default(), nil
}

func (t *Transport) NewSession(ctx context.Context, req libacp.NewSessionRequest) (libacp.NewSessionResponse, error) {
	internalID := newSessionID(sessionNamespace(t))
	sessionID := libacp.SessionID(internalID)

	reportErr, reportChange, end := t.tracker().Start(ctx, "new", "acp_session", "session_id", string(sessionID), "cwd", req.Cwd, "mcp_servers", len(req.McpServers))
	defer end()

	if !filepath.IsAbs(req.Cwd) {
		err := libacp.NewErrorf(libacp.ErrInvalidParams, "cwd must be an absolute path, got %q", req.Cwd)
		reportErr(err)
		return libacp.NewSessionResponse{}, err
	}
	if t.deps.Engine == nil {
		err := errSetupRequired()
		reportErr(err)
		return libacp.NewSessionResponse{}, err
	}
	sessionCwd, err := t.resolveWorkspaceCwd(req.Cwd)
	if err != nil {
		reportErr(err)
		return libacp.NewSessionResponse{}, err
	}

	workspaceID := t.workspaceID()

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())

	// A client binds this session to a REGISTERED external ACP agent via the
	// contenox.agent `_meta` key; absent, the native chain path below runs
	// unchanged (byte-for-byte the historical behavior).
	if agentName := parseAgentMeta(req.Meta); agentName != "" {
		// Spawn and drive the downstream agent first: an unknown/disabled agent or a
		// spawn/handshake failure must fail session/new cleanly with NO session and
		// NO leaked process. The upstream client's req.McpServers are for the chain
		// engine (unused here); the downstream agent gets its own declared allowlist.
		handle, downstreamID, bridge, spawnErr := t.spawnExternal(ctx, sessionID, sessionCwd, agentName, false)
		if spawnErr != nil {
			reportErr(spawnErr)
			return libacp.NewSessionResponse{}, spawnErr
		}
		ag := agentservice.New(agentservice.Deps{
			Engine:      t.deps.Engine,
			DB:          t.deps.DB,
			WorkspaceID: workspaceID,
			Identity:    "acp-client",
		})
		contenoxSessionID, sessErr := ag.SessionNew(ctx, internalID)
		if sessErr != nil {
			_ = handle.Close()
			wrapped := fmt.Errorf("acpsvc: agent.SessionNew: %w", sessErr)
			reportErr(wrapped)
			return libacp.NewSessionResponse{}, wrapped
		}
		entry := &sessionEntry{
			WorkspaceID:       workspaceID,
			Cwd:               sessionCwd,
			InternalSessionID: contenoxSessionID,
			HITLPolicy:        hitlPolicyDefaultValue,
			driver: &externalDriver{
				t:            t,
				agentName:    agentName,
				upstreamID:   sessionID,
				handle:       handle,
				downstreamID: downstreamID,
				bridge:       bridge,
			},
		}
		t.sessionMu.Lock()
		t.sessions[sessionID] = entry
		t.bindContenoxSession(contenoxSessionID, sessionID)
		t.sessionMu.Unlock()
		t.persistSessionCwd(ctx, store, sessionID, sessionCwd)
		t.persistSessionAgent(ctx, store, sessionID, agentName)
		t.clearToolCallState(sessionID)

		// The downstream agent advertises its slash-command menu immediately after
		// its own session/new (an available_commands_update the bridge cached without
		// relaying — a menu delivered before THIS session/new response references a
		// session id the upstream client has not learned and is dropped). Re-emit the
		// cached menu once the result is on the wire, mirroring the native menu's
		// sendAvailableCommands scheduling (see externalBridge.markBound).
		libacp.AfterResponse(ctx, func() {
			bridge.markBound(ctx)
		})

		reportChange(string(sessionID), map[string]any{
			"contenox_session_id": contenoxSessionID,
			"workspace_id":        workspaceID,
			"external_agent":      agentName,
		})
		return libacp.NewSessionResponse{
			SessionID:     sessionID,
			ConfigOptions: t.sessionConfigOptions(ctx, entry),
			Meta:          agentMetaJSON(entry.driver.AgentName()),
		}, nil
	}

	registered, err := t.registerMcpServers(ctx, store, sessionID, req.McpServers)
	if err != nil {
		reportErr(err)
		return libacp.NewSessionResponse{}, err
	}

	ag := agentservice.New(agentservice.Deps{
		Engine:      t.deps.Engine,
		DB:          t.deps.DB,
		WorkspaceID: workspaceID,
		Identity:    "acp-client",
	})

	contenoxSessionID, err := ag.SessionNew(ctx, internalID)
	if err != nil {
		t.cleanupMcpServers(ctx, store, registered)
		wrapped := fmt.Errorf("acpsvc: agent.SessionNew: %w", err)
		reportErr(wrapped)
		return libacp.NewSessionResponse{}, wrapped
	}

	entry := &sessionEntry{
		WorkspaceID:       workspaceID,
		Cwd:               sessionCwd,
		InternalSessionID: contenoxSessionID,
		McpServerNames:    registered,
		driver:            &nativeDriver{t: t, agent: ag},
		Provider:          t.provider(),
		Model:             t.model(),
		Think:             t.thinkDefault(),
		HITLPolicy:        hitlPolicyDefaultValue,
	}
	t.sessionMu.Lock()
	t.sessions[sessionID] = entry
	t.bindContenoxSession(contenoxSessionID, sessionID)
	t.sessionMu.Unlock()
	t.persistSessionCwd(ctx, store, sessionID, sessionCwd)
	t.clearToolCallState(sessionID)
	t.subscribeTerminal(sessionID, contenoxSessionID)

	// A client learns this new session's id only from the session/new result;
	// emitting available_commands_update before that result makes the client drop
	// it as an unknown session (and the slash-command menu never appears). Defer
	// it until libacp has written the result.
	libacp.AfterResponse(ctx, func() {
		t.sendAvailableCommands(ctx, sessionID)
		if banner := t.takeBanner(); banner != "" {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sessionID,
				Update:    libacp.NewAgentMessageChunk(banner),
			})
		}
		t.sendInitialUsageUpdate(ctx, sessionID)
	})

	reportChange(string(sessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
		"workspace_id":        workspaceID,
	})
	return libacp.NewSessionResponse{
		SessionID:     sessionID,
		ConfigOptions: t.sessionConfigOptions(ctx, entry),
	}, nil
}

// ResumeSession is session/load without the history replay: the client kept
// its transcript and only needs the server-side session re-bound.
func (t *Transport) ResumeSession(ctx context.Context, req libacp.ResumeSessionRequest) (libacp.ResumeSessionResponse, error) {
	reportErr, reportChange, end := t.tracker().Start(ctx, "resume", "acp_session", "session_id", string(req.SessionID))
	defer end()

	if req.SessionID == "" {
		err := libacp.NewError(libacp.ErrInvalidParams, "sessionId is required")
		reportErr(err)
		return libacp.ResumeSessionResponse{}, err
	}
	if !filepath.IsAbs(req.Cwd) {
		err := libacp.NewErrorf(libacp.ErrInvalidParams, "cwd must be an absolute path, got %q", req.Cwd)
		reportErr(err)
		return libacp.ResumeSessionResponse{}, err
	}
	if t.deps.Engine == nil {
		err := errSetupRequired()
		reportErr(err)
		return libacp.ResumeSessionResponse{}, err
	}
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	sessionCwd, err := t.resolveExistingSessionCwd(ctx, store, req.SessionID, req.Cwd)
	if err != nil {
		reportErr(err)
		return libacp.ResumeSessionResponse{}, err
	}
	workspaceID := t.workspaceID()
	if resolved, ok := t.resolveSessionWorkspace(ctx, string(req.SessionID)); ok {
		workspaceID = resolved
	}

	registered, err := t.registerMcpServers(ctx, store, req.SessionID, req.McpServers)
	if err != nil {
		reportErr(err)
		return libacp.ResumeSessionResponse{}, err
	}

	ag := agentservice.New(agentservice.Deps{
		Engine:      t.deps.Engine,
		DB:          t.deps.DB,
		WorkspaceID: workspaceID,
		Identity:    "acp-client",
	})

	contenoxSessionID, err := ag.SessionResume(ctx, string(req.SessionID))
	if err != nil {
		t.cleanupMcpServers(ctx, store, registered)
		wrapped := libacp.NewErrorf(libacp.ErrInvalidParams, "resume session %q: %v", req.SessionID, err)
		reportErr(wrapped)
		return libacp.ResumeSessionResponse{}, wrapped
	}

	entry := &sessionEntry{
		WorkspaceID:       workspaceID,
		Cwd:               sessionCwd,
		InternalSessionID: contenoxSessionID,
		McpServerNames:    registered,
		driver:            &nativeDriver{t: t, agent: ag},
		Provider:          t.provider(),
		Model:             t.model(),
		Think:             t.thinkDefault(),
		HITLPolicy:        hitlPolicyDefaultValue,
	}
	t.sessionMu.Lock()
	t.sessions[req.SessionID] = entry
	t.bindContenoxSession(contenoxSessionID, req.SessionID)
	t.sessionMu.Unlock()
	t.persistSessionCwd(ctx, store, req.SessionID, sessionCwd)
	t.markExternalIfPersisted(ctx, store, req.SessionID, entry)
	t.clearToolCallState(req.SessionID)
	t.subscribeTerminal(req.SessionID, contenoxSessionID)

	// Mirror LoadSession: a native session re-advertises its contenox menu; an
	// external session re-emits its downstream agent's persisted menu (the live bridge
	// died with the pre-resume connection and is not respawned until the next prompt).
	if _, isExternal := entry.driver.(*externalDriver); isExternal {
		t.reemitExternalCommandMenu(ctx, store, req.SessionID)
	} else if entry.driver.AvailableCommands() != nil {
		libacp.AfterResponse(ctx, func() {
			t.sendAvailableCommands(ctx, req.SessionID)
		})
	}

	reportChange(string(req.SessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
	})
	return libacp.ResumeSessionResponse{ConfigOptions: t.reloadedConfigOptions(ctx, store, req.SessionID, entry)}, nil
}

// SetSessionMode is not supported: contenox does not model a Zed-style
// Ask/Code session mode toggle as a first-class session/set_mode capability —
// the equivalent controls (model, HITL policy, think level) are exposed as
// session config options instead. Initialize never returns a Modes state in
// session/new or session/load, so a conformant client will never call this.
func (t *Transport) SetSessionMode(_ context.Context, _ libacp.SetSessionModeRequest) (libacp.SetSessionModeResponse, error) {
	return libacp.SetSessionModeResponse{}, libacp.MethodNotFound(libacp.MethodSessionSetMode)
}

// CloseSession releases the connection-local resources of a session without
// touching its stored history. Closing an unknown session succeeds: the
// desired state (not open here) already holds.
func (t *Transport) CloseSession(ctx context.Context, req libacp.CloseSessionRequest) (libacp.CloseSessionResponse, error) {
	_, reportChange, end := t.tracker().Start(ctx, "close", "acp_session", "session_id", string(req.SessionID))
	defer end()

	if req.SessionID == "" {
		return libacp.CloseSessionResponse{}, libacp.NewError(libacp.ErrInvalidParams, "sessionId is required")
	}
	entry := t.dropSessionEntry(req.SessionID)
	if entry != nil && t.deps.DB != nil {
		store := runtimetypes.New(t.deps.DB.WithoutTransaction())
		t.cleanupMcpServers(ctx, store, entry.McpServerNames)
	}
	// An explicit close ends the session on this connection: tear down its driver
	// (an external driver closes its downstream agent now, rather than waiting for
	// connection teardown to reap it; the native driver is a no-op).
	if entry != nil {
		_ = entry.driver.Close()
	}
	t.clearToolCallState(req.SessionID)
	// An explicit close is a user action ending the session on this connection —
	// tear down its shell (unlike a bare connection drop, which keeps the shell
	// alive for reconnect and lets the idle reaper reclaim it).
	t.closeTerminal(req.SessionID, entry)
	reportChange(string(req.SessionID), map[string]any{"was_open": entry != nil})
	return libacp.CloseSessionResponse{}, nil
}

// DeleteSession removes the session's stored history (and any connection-local
// state). Per spec, deleting a nonexistent session succeeds silently, and the
// session disappears from session/list.
func (t *Transport) DeleteSession(ctx context.Context, req libacp.DeleteSessionRequest) (libacp.DeleteSessionResponse, error) {
	reportErr, reportChange, end := t.tracker().Start(ctx, "delete", "acp_session", "session_id", string(req.SessionID))
	defer end()

	if req.SessionID == "" {
		err := libacp.NewError(libacp.ErrInvalidParams, "sessionId is required")
		reportErr(err)
		return libacp.DeleteSessionResponse{}, err
	}
	if t.deps.Engine == nil {
		err := errSetupRequired()
		reportErr(err)
		return libacp.DeleteSessionResponse{}, err
	}

	workspaceID := t.workspaceID()
	if resolved, ok := t.resolveSessionWorkspace(ctx, string(req.SessionID)); ok {
		workspaceID = resolved
	}

	entry := t.dropSessionEntry(req.SessionID)
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	if entry != nil {
		t.cleanupMcpServers(ctx, store, entry.McpServerNames)
	}
	// The session is being deleted; its driver's downstream agent (if any) must
	// not outlive it.
	if entry != nil {
		_ = entry.driver.Close()
	}
	t.clearToolCallState(req.SessionID)
	// The session's history is being deleted; its shell must not outlive it.
	t.closeTerminal(req.SessionID, entry)

	ag := agentservice.New(agentservice.Deps{
		Engine:      t.deps.Engine,
		DB:          t.deps.DB,
		WorkspaceID: workspaceID,
		Identity:    "acp-client",
	})
	if err := ag.SessionDelete(ctx, string(req.SessionID)); err != nil {
		reportErr(err)
		return libacp.DeleteSessionResponse{}, libacp.InternalError(err.Error())
	}
	_ = store.DeleteKV(ctx, acpSessionCwdKVPrefix+string(req.SessionID))
	_ = store.DeleteKV(ctx, acpSessionAgentKVPrefix+string(req.SessionID))
	// The downstream-surface keys (command menu, config options) and the external
	// session's per-session HITL policy are meaningful only alongside the agent-name
	// key; drop them with it.
	_ = store.DeleteKV(ctx, acpSessionAgentCommandsKVPrefix+string(req.SessionID))
	_ = store.DeleteKV(ctx, acpSessionAgentConfigOptionsKVPrefix+string(req.SessionID))
	_ = store.DeleteKV(ctx, acpSessionHITLPolicyKVPrefix+string(req.SessionID))

	reportChange(string(req.SessionID), map[string]any{"was_open": entry != nil})
	return libacp.DeleteSessionResponse{}, nil
}

// dropSessionEntry removes a session from the in-memory maps and returns the
// removed entry (nil if it was not open on this connection).
func (t *Transport) dropSessionEntry(sid libacp.SessionID) *sessionEntry {
	// Abort any in-flight turn before the session's connection-local state goes
	// away: a Close/Delete that races a running prompt must stop the chain, not
	// let it keep executing against a session that no longer exists here. A clean
	// no-op when nothing is running.
	t.cancelInflightPrompt(sid)
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()
	entry, ok := t.sessions[sid]
	if !ok {
		return nil
	}
	delete(t.sessions, sid)
	t.unbindContenoxSession(entry.InternalSessionID)
	return entry
}

func (t *Transport) registerMcpServers(ctx context.Context, store runtimetypes.Store, sessionID libacp.SessionID, servers []libacp.McpServer) ([]string, error) {
	var registered []string
	for _, srv := range servers {
		if err := srv.Validate(); err != nil {
			t.cleanupMcpServers(ctx, store, registered)
			return nil, fmt.Errorf("acpsvc: invalid mcp server %q: %w", srv.Name, err)
		}
		name := mcpNameFor(t.mcpOwnerID(), sessionID, srv.Name)
		row := mcpRowFromLibacp(name, srv)
		if err := store.UpsertMCPServerByName(ctx, row); err != nil {
			t.cleanupMcpServers(ctx, store, registered)
			return nil, fmt.Errorf("acpsvc: register mcp server %q: %w", srv.Name, err)
		}
		if t.deps.Engine != nil && t.deps.Engine.MCPManager != nil {
			if err := t.deps.Engine.MCPManager.StartWorker(ctx, row); err != nil {
				registered = append(registered, name)
				t.cleanupMcpServers(ctx, store, registered)
				return nil, fmt.Errorf("acpsvc: start mcp worker %q: %w", srv.Name, err)
			}
		}
		registered = append(registered, name)
	}
	return registered, nil
}

func (t *Transport) cleanupMcpServers(ctx context.Context, store runtimetypes.Store, names []string) {
	for _, name := range names {
		if t.deps.Engine != nil && t.deps.Engine.MCPManager != nil {
			t.deps.Engine.MCPManager.StopWorker(ctx, name)
		}
		cleanupMCPSessionIDs(ctx, store, name)
		row, err := store.GetMCPServerByName(ctx, name)
		if err != nil {
			if errors.Is(err, libdb.ErrNotFound) {
				continue
			}
			continue
		}
		_ = store.DeleteMCPServer(ctx, row.ID)
	}
}

func cleanupMCPSessionIDs(ctx context.Context, store runtimetypes.Store, serverName string) {
	prefix := "mcp_session:" + serverName + ":"
	for {
		page, err := store.ListKVPrefix(ctx, prefix, nil, 100)
		if err != nil {
			return
		}
		for _, kv := range page {
			_ = store.DeleteKV(ctx, kv.Key)
		}
		if len(page) < 100 {
			return
		}
	}
}

func (t *Transport) runtimeToolsAllowlist(ctx context.Context, store runtimetypes.Store, sessionNames []string) ([]string, error) {
	allowlist := []string{"*"}
	current := make(map[string]struct{}, len(sessionNames))
	for _, name := range sessionNames {
		current[name] = struct{}{}
	}
	var cursor *time.Time
	for {
		page, err := store.ListMCPServers(ctx, cursor, 100)
		if err != nil {
			return nil, fmt.Errorf("acpsvc: list mcp servers for runtime allowlist: %w", err)
		}
		for _, srv := range page {
			if !runtimetypes.IsACPManagedMCPServerName(srv.Name) {
				continue
			}
			if _, ok := current[srv.Name]; ok {
				continue
			}
			allowlist = append(allowlist, "!"+srv.Name)
		}
		if len(page) < 100 {
			return allowlist, nil
		}
		cursor = &page[len(page)-1].CreatedAt
	}
}

// CleanupStaleACPManagedMCPServers removes client-scoped ACP MCP registrations
// left behind by a previous process. Durable MCP configuration must be created
// through the normal `contenox mcp` commands or HTTP API; session/new and
// session/load MCP servers are temporary by ACP contract.
func CleanupStaleACPManagedMCPServers(ctx context.Context, db libdb.DBManager) error {
	if db == nil {
		return nil
	}
	store := runtimetypes.New(db.WithoutTransaction())
	var stale []*runtimetypes.MCPServer
	var cursor *time.Time
	for {
		page, err := store.ListMCPServers(ctx, cursor, 100)
		if err != nil {
			return err
		}
		for _, srv := range page {
			if runtimetypes.IsACPManagedMCPServerName(srv.Name) {
				stale = append(stale, srv)
			}
		}
		if len(page) < 100 {
			break
		}
		cursor = &page[len(page)-1].CreatedAt
	}
	for _, srv := range stale {
		cleanupMCPSessionIDs(ctx, store, srv.Name)
		if err := store.DeleteMCPServer(ctx, srv.ID); err != nil && !errors.Is(err, libdb.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (t *Transport) mcpOwnerID() string {
	if t.connectionID != "" {
		return t.connectionID
	}
	return "conn-unknown"
}

func mcpRowFromLibacp(name string, srv libacp.McpServer) *runtimetypes.MCPServer {
	row := &runtimetypes.MCPServer{
		Name:                  name,
		Transport:             "stdio",
		Command:               srv.Command,
		Args:                  srv.Args,
		URL:                   srv.URL,
		ConnectTimeoutSeconds: 30,
	}
	switch srv.Kind() {
	case libacp.McpServerKindHTTP:
		row.Transport = "http"
	case libacp.McpServerKindSSE:
		row.Transport = "sse"
	default:
		row.Transport = "stdio"
	}
	if len(srv.Headers) > 0 {
		row.Headers = make(map[string]string, len(srv.Headers))
		for _, h := range srv.Headers {
			row.Headers[h.Name] = h.Value
		}
	}
	return row
}

func newSessionID(namespace string) string {
	return namespace + "-" + uuid.NewString()
}

func sessionNamespace(t *Transport) string {
	id := t.clientIdentity()
	if id == nil {
		return "acp"
	}
	var sb strings.Builder
	for _, r := range strings.ToLower(id.Name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
		if sb.Len() >= 16 {
			break
		}
	}
	if sb.Len() == 0 {
		return "acp"
	}
	return sb.String()
}

func (t *Transport) Close(ctx context.Context) error {
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())

	// A bare connection drop stops streaming but does NOT kill shells: a browser
	// reload reconnects and re-subscribes, and persistent shells are the point.
	// Idle-timeout reclaims anything genuinely abandoned.
	t.unsubscribeAllTerminals()

	t.sessionMu.Lock()
	entries := make([]*sessionEntry, 0, len(t.sessions))
	sids := make([]libacp.SessionID, 0, len(t.sessions))
	for sid, e := range t.sessions {
		entries = append(entries, e)
		sids = append(sids, sid)
		// Deregister from the shared permission router before dropping the map so
		// a shared engine stops routing approvals to this closing connection.
		t.deps.PermissionRouter.unbind(e.InternalSessionID, t)
	}
	t.sessions = make(map[libacp.SessionID]*sessionEntry)
	t.contenoxToACPID = make(map[string]libacp.SessionID)
	t.sessionMu.Unlock()

	// A connection drop must stop any in-flight turns on this transport. libacp
	// cancels the prompt contexts it substituted when its own Run loop ends, but
	// the server owns cancellation here too, so it does not depend on that.
	for _, sid := range sids {
		t.cancelInflightPrompt(sid)
	}

	for _, e := range entries {
		t.cleanupMcpServers(ctx, store, e.McpServerNames)
		// Tear down this session's driver. For an external session this closes the
		// downstream agent it spawned; idempotent with the connCtx-cancel teardown
		// the New() Closed goroutine performs. Native is a no-op.
		_ = e.driver.Close()
	}
	return nil
}

func (t *Transport) sessionFor(id libacp.SessionID) (*sessionEntry, bool) {
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()
	e, ok := t.sessions[id]
	return e, ok
}

func (t *Transport) resolveSessionWorkspace(ctx context.Context, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	row := t.deps.DB.WithoutTransaction().QueryRowContext(ctx, `
		SELECT mi.workspace_id
		FROM message_indices mi
		WHERE mi.name = $1 AND mi.identity = 'acp-client'
		ORDER BY (SELECT COUNT(*) FROM messages m WHERE m.idx_id = mi.id) DESC, mi.id DESC
		LIMIT 1`, name)
	var workspaceID string
	if err := row.Scan(&workspaceID); err != nil || workspaceID == "" {
		return "", false
	}
	return workspaceID, true
}

// listSessionsPageSize bounds one session/list page; a var so tests can
// exercise paging without minting hundreds of sessions.
var listSessionsPageSize = 100

// sessionListRow is one session/list candidate before pagination: the
// internal index id, the ACP session name, and the freshest message time
// (hasTime=false when the session has no messages yet).
type sessionListRow struct {
	internalID string
	name       string
	updatedAt  time.Time
	hasTime    bool
}

// sessionListRowLess is the freshest-first total order session/list returns:
// rows with a message time sort by it descending, rows without one sort after
// all rows that have one, and every tie falls back to internal id — the order
// must be total or the pagination cursor is ambiguous.
func sessionListRowLess(a, b sessionListRow) bool {
	if a.hasTime != b.hasTime {
		return a.hasTime
	}
	if a.hasTime && !a.updatedAt.Equal(b.updatedAt) {
		return a.updatedAt.After(b.updatedAt)
	}
	return a.internalID > b.internalID
}

// listSessionsCursor encodes a page boundary as the sort key of the last row
// the page returned: "<unixnano>|<internal id>", with an empty time part for
// rows that have no messages. Opaque to clients. Encoding the full sort key —
// not just the id — lets listSessionsResume position strictly after the
// boundary even when that row gained a fresher timestamp or was deleted
// between pages.
func listSessionsCursor(r sessionListRow) string {
	ts := ""
	if r.hasTime {
		ts = strconv.FormatInt(r.updatedAt.UnixNano(), 10)
	}
	return ts + "|" + r.internalID
}

// listSessionsResume returns the index of the first row sorting strictly
// after the cursor's boundary key, i.e. where the next page starts. A cursor
// that decodes to a key no longer present still positions correctly; a
// malformed cursor degrades to comparing by internal id alone.
func listSessionsResume(rows []sessionListRow, cursor string) int {
	tsPart, id, found := strings.Cut(cursor, "|")
	boundary := sessionListRow{internalID: id}
	if !found {
		boundary.internalID = cursor
	} else if tsPart != "" {
		if ns, err := strconv.ParseInt(tsPart, 10, 64); err == nil {
			boundary.updatedAt = time.Unix(0, ns)
			boundary.hasTime = true
		}
	}
	for i, r := range rows {
		if sessionListRowLess(boundary, r) {
			return i
		}
	}
	return len(rows)
}

func (t *Transport) ListSessions(ctx context.Context, req libacp.ListSessionsRequest) (libacp.ListSessionsResponse, error) {
	exec := t.deps.DB.WithoutTransaction()

	// The ACP session id is the message-index NAME (session/new mints it and
	// agentservice resolves loads by name); mi.id is contenox-internal. Rows
	// without a name predate ACP naming and cannot be loaded, so they are not
	// listed. Ordering and pagination happen in Go, not SQL: the roster must
	// come back freshest-first over MAX(added_at), but mi.id is a random UUID
	// (useless for ORDER BY) and the two schema dialects disagree on timestamp
	// representation, so a portable SQL keyset is not worth it. The
	// per-workspace roster is small; only the returned page pays the
	// title/cwd lookups. The cwd filter applies after pagination, so a
	// filtered page may carry fewer items but the cursor still advances.
	rows, err := exec.QueryContext(ctx, `
		SELECT mi.id, mi.name,
		       (SELECT MAX(m.added_at) FROM messages m WHERE m.idx_id = mi.id)
		FROM message_indices mi
		WHERE mi.workspace_id = $1
		  AND mi.identity = 'acp-client'
		  AND mi.name IS NOT NULL AND mi.name != ''`, t.workspaceID())
	if err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: list sessions: %w", err)
	}
	defer rows.Close()

	var all []sessionListRow
	for rows.Next() {
		var row sessionListRow
		var updatedAt any
		if err := rows.Scan(&row.internalID, &row.name, &updatedAt); err != nil {
			return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: scan session: %w", err)
		}
		row.updatedAt, row.hasTime = parseDBTime(updatedAt)
		all = append(all, row)
	}
	if err := rows.Err(); err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: rows: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return sessionListRowLess(all[i], all[j]) })

	start := 0
	if req.Cursor != "" {
		start = listSessionsResume(all, req.Cursor)
	}
	end := min(start+listSessionsPageSize, len(all))

	store := runtimetypes.New(exec)
	chatMgr := chatservice.NewManager(t.workspaceID())
	var sessions []libacp.SessionInfo
	for _, row := range all[start:end] {
		info := libacp.SessionInfo{
			SessionID: libacp.SessionID(row.name),
			Title:     t.sessionListTitle(ctx, chatMgr, exec, row.internalID, row.name),
			Cwd:       t.sessionCwd(ctx, store, libacp.SessionID(row.name)),
		}
		// External sessions carry their agent attribution in `_meta` so a client
		// (the beam fleet view) can tell which registered agent runs each one.
		if agentName := t.readSessionAgent(ctx, store, libacp.SessionID(row.name)); agentName != "" {
			info.Meta = agentMetaJSON(agentName)
		}
		if row.hasTime {
			info.UpdatedAt = row.updatedAt.UTC().Format(time.RFC3339)
		}
		if req.Cwd != "" && info.Cwd != "" && info.Cwd != req.Cwd {
			continue
		}
		sessions = append(sessions, info)
	}

	resp := libacp.ListSessionsResponse{Sessions: sessions}
	if end < len(all) {
		resp.NextCursor = listSessionsCursor(all[end-1])
	}
	return resp, nil
}

// sessionListTitle derives a session/list Title from the session's first
// user message — the same "subject" heuristic internalchatapi's chat listing
// used before it was retired in favor of ACP: it describes what the chat is
// about, unlike the last message which can be an assistant error or raw tool
// JSON. Falls back to the session name (fallback) when there is no stored
// user message yet, or on read failure — session/list must never error out
// over a title.
func (t *Transport) sessionListTitle(ctx context.Context, mgr *chatservice.Manager, exec libdb.Exec, internalSessionID, fallback string) string {
	if title := firstUserMessageTitle(ctx, mgr, exec, internalSessionID); title != "" {
		return title
	}
	return fallback
}

// firstUserMessageTitle derives a humane session title from the session's
// first non-empty user message, whitespace-collapsed and clipped to
// sessionListTitleMaxLen. Returns "" when the session has no stored user
// message yet or on read failure — the shared heuristic behind both the
// session/list Title and the live post-turn session_info_update Title, so a
// client's tab/sidebar label matches whether it learned the title from a
// re-list or a live push.
func firstUserMessageTitle(ctx context.Context, mgr *chatservice.Manager, exec libdb.Exec, internalSessionID string) string {
	msgs, err := mgr.ListMessages(ctx, exec, internalSessionID)
	if err != nil {
		return ""
	}
	for _, m := range msgs {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			return truncateSessionListTitle(strings.TrimSpace(m.Content))
		}
	}
	return ""
}

// sessionInfoTitle derives the live Title pushed on a prompt turn's
// session_info_update from the session's first user message, reusing the
// session/list heuristic. Empty when the session has no user message yet (or
// when there is no DB to read) — callers omit the Title in that case so the
// notification stays a pure freshness (updatedAt) ping.
func (t *Transport) sessionInfoTitle(ctx context.Context, internalSessionID string) string {
	if t.deps.DB == nil || internalSessionID == "" {
		return ""
	}
	mgr := chatservice.NewManager(t.workspaceID())
	return firstUserMessageTitle(ctx, mgr, t.deps.DB.WithoutTransaction(), internalSessionID)
}

const acpSessionCwdKVPrefix = "acp:session_cwd:"

type sessionCwdRecord struct {
	Cwd string `json:"cwd"`
}

// persistSessionCwd records the session's cwd durably so session/list can
// report it (the spec requires cwd on SessionInfo) and filter by it across
// process restarts — the in-memory session map is empty in a fresh process.
func (t *Transport) persistSessionCwd(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID, cwd string) {
	if cwd == "" {
		return
	}
	raw, err := json.Marshal(sessionCwdRecord{Cwd: cwd})
	if err != nil {
		return
	}
	if err := store.SetKV(ctx, acpSessionCwdKVPrefix+string(sid), raw); err != nil {
		reportErr, _, end := t.tracker().Start(ctx, "persist_cwd", "acp_session", "session_id", string(sid))
		reportErr(err)
		end()
	}
}

// sessionCwd resolves a session's cwd: live entry first, then the durable KV
// record. Empty when neither knows (sessions created before cwd persistence).
func (t *Transport) sessionCwd(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) string {
	t.sessionMu.Lock()
	entry, ok := t.sessions[sid]
	t.sessionMu.Unlock()
	if ok && entry.Cwd != "" {
		return entry.Cwd
	}
	var rec sessionCwdRecord
	if err := store.GetKV(ctx, acpSessionCwdKVPrefix+string(sid), &rec); err != nil {
		return ""
	}
	return rec.Cwd
}

// parseDBTime normalizes MAX(added_at) across drivers: SQLite hands back
// strings (layout depends on how the value was written), Postgres a time.Time.
func parseDBTime(v any) (time.Time, bool) {
	switch tv := v.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return tv, true
	case []byte:
		return parseDBTimeString(string(tv))
	case string:
		return parseDBTimeString(tv)
	}
	return time.Time{}, false
}

func parseDBTimeString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		// time.Time.String() — what the sqlite driver stores when a time.Time
		// is bound to a TIMESTAMP column. Until this layout was handled, every
		// session/list row lost its updatedAt and the sidebar sort collapsed
		// to random-UUID order.
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}
