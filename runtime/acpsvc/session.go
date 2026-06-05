package acpsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	libacp "github.com/contenox/agent/libacp"
	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/agentservice"
	"github.com/contenox/agent/runtime/runtimetypes"
	"github.com/contenox/agent/runtime/taskengine"
)

const mcpNamePrefix = "acp-"

func mcpNameFor(sessionID libacp.SessionID, original string) string {
	return mcpNamePrefix + string(sessionID) + "-" + original
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
	workspaceID := t.workspaceID()
	if resolved, ok := t.resolveSessionWorkspace(ctx, string(req.SessionID)); ok {
		workspaceID = resolved
	}

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
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
		Cwd:               req.Cwd,
		InternalSessionID: contenoxSessionID,
		Agent:             ag,
		McpServerNames:    registered,
		Think:             t.thinkDefault(),
	}
	t.sessionMu.Lock()
	t.sessions[req.SessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = req.SessionID
	t.sessionMu.Unlock()

	t.replayMessages(ctx, req.SessionID, messages)
	// Emit the slash-command menu only after the session/load result is on the
	// wire (see sendAvailableCommands) so the client can resolve the session.
	libacp.AfterResponse(ctx, func() { t.sendAvailableCommands(ctx, req.SessionID) })

	reportChange(string(req.SessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
		"message_count":       len(messages),
	})
	return libacp.LoadSessionResponse{}, nil
}

func (t *Transport) replayMessages(ctx context.Context, sessionID libacp.SessionID, messages []taskengine.Message) {
	_, reportChange, end := t.tracker().Start(ctx, "replay", "acp_session", "session_id", string(sessionID), "message_count", len(messages))
	defer end()

	var users, assistantText, toolCalls, toolResults int
	for _, m := range messages {
		switch m.Role {
		case "user":
			if m.Content == "" {
				continue
			}
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sessionID,
				Update:    libacp.NewUserMessageChunk(m.Content),
			})
			users++
		case "assistant":
			if m.Thinking != "" {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sessionID,
					Update:    libacp.NewAgentThoughtChunk(m.Thinking),
				})
			}
			if m.Content != "" {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sessionID,
					Update:    libacp.NewAgentMessageChunk(m.Content),
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
// "Setup Contenox" terminal auth method can configure a model.
func errSetupRequired() error {
	return libacp.NewError(libacp.ErrInvalidParams, "contenox is not configured yet: no default-model is set. Run the \"Setup Contenox\" auth method (or `contenox acp --setup`), then reconnect.")
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

	workspaceID := t.workspaceID()

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
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
		Cwd:               req.Cwd,
		InternalSessionID: contenoxSessionID,
		Agent:             ag,
		McpServerNames:    registered,
		Think:             t.thinkDefault(),
	}
	t.sessionMu.Lock()
	t.sessions[sessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = sessionID
	t.sessionMu.Unlock()

	// A client learns this new session's id only from the session/new result;
	// emitting available_commands_update before that result makes the client drop
	// it as an unknown session (and the slash-command menu never appears). Defer
	// it until libacp has written the result.
	libacp.AfterResponse(ctx, func() { t.sendAvailableCommands(ctx, sessionID) })

	reportChange(string(sessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
		"workspace_id":        workspaceID,
	})
	return libacp.NewSessionResponse{SessionID: sessionID}, nil
}

func (t *Transport) registerMcpServers(ctx context.Context, store runtimetypes.Store, sessionID libacp.SessionID, servers []libacp.McpServer) ([]string, error) {
	var registered []string
	for _, srv := range servers {
		if err := srv.Validate(); err != nil {
			t.cleanupMcpServers(ctx, store, registered)
			return nil, fmt.Errorf("acpsvc: invalid mcp server %q: %w", srv.Name, err)
		}
		name := mcpNameFor(sessionID, srv.Name)
		row := mcpRowFromLibacp(name, srv)
		if err := store.UpsertMCPServerByName(ctx, row); err != nil {
			t.cleanupMcpServers(ctx, store, registered)
			return nil, fmt.Errorf("acpsvc: register mcp server %q: %w", srv.Name, err)
		}
		registered = append(registered, name)
	}
	return registered, nil
}

func (t *Transport) cleanupMcpServers(ctx context.Context, store runtimetypes.Store, names []string) {
	for _, name := range names {
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

	t.sessionMu.Lock()
	entries := make([]*sessionEntry, 0, len(t.sessions))
	for _, e := range t.sessions {
		entries = append(entries, e)
	}
	t.sessions = make(map[libacp.SessionID]*sessionEntry)
	t.contenoxToACPID = make(map[string]libacp.SessionID)
	t.sessionMu.Unlock()

	for _, e := range entries {
		t.cleanupMcpServers(ctx, store, e.McpServerNames)
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

func (t *Transport) ListSessions(ctx context.Context, req libacp.ListSessionsRequest) (libacp.ListSessionsResponse, error) {
	exec := t.deps.DB.WithoutTransaction()

	rows, err := exec.QueryContext(ctx, `
		SELECT mi.id, mi.workspace_id, COALESCE(mi.name, ''),
		       COALESCE(
		         (SELECT MAX(m.added_at) FROM messages m WHERE m.idx_id = mi.id),
		         ''
		       )
		FROM message_indices mi
		WHERE mi.workspace_id = $1
		  AND mi.identity = 'acp-client'
		ORDER BY mi.id DESC`, t.workspaceID())
	if err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []libacp.SessionInfo
	for rows.Next() {
		var id, workspaceID, name, updatedAt string
		if err := rows.Scan(&id, &workspaceID, &name, &updatedAt); err != nil {
			return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: scan session: %w", err)
		}

		info := libacp.SessionInfo{
			SessionID: libacp.SessionID(id),
		}
		if name != "" {
			info.Title = name
		}
		if updatedAt != "" {
			info.UpdatedAt = updatedAt
		}

		// Filter by cwd if requested.
		if req.Cwd != "" {
			t.sessionMu.Lock()
			entry, ok := t.sessions[info.SessionID]
			t.sessionMu.Unlock()
			if ok {
				info.Cwd = entry.Cwd
				if info.Cwd != req.Cwd {
					continue
				}
			}
		}

		sessions = append(sessions, info)
	}
	if err := rows.Err(); err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: rows: %w", err)
	}

	return libacp.ListSessionsResponse{Sessions: sessions}, nil
}
