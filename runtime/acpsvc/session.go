package acpsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
)

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
		Provider:          t.provider(),
		Model:             t.model(),
		Think:             t.thinkDefault(),
	}
	t.sessionMu.Lock()
	t.sessions[req.SessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = req.SessionID
	t.sessionMu.Unlock()
	t.persistSessionCwd(ctx, store, req.SessionID, req.Cwd)

	t.clearToolCallState(req.SessionID)
	t.replayMessages(ctx, req.SessionID, messages)
	// Emit the slash-command menu only after the session/load result is on the
	// wire (see sendAvailableCommands) so the client can resolve the session.
	libacp.AfterResponse(ctx, func() {
		t.sendAvailableCommands(ctx, req.SessionID)
		if banner := t.takeBanner(); banner != "" {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: req.SessionID,
				Update:    libacp.NewAgentMessageChunk(banner),
			})
		}
	})

	reportChange(string(req.SessionID), map[string]any{
		"contenox_session_id": contenoxSessionID,
		"message_count":       len(messages),
	})
	return libacp.LoadSessionResponse{ConfigOptions: t.sessionConfigOptions(ctx, entry)}, nil
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
		Provider:          t.provider(),
		Model:             t.model(),
		Think:             t.thinkDefault(),
	}
	t.sessionMu.Lock()
	t.sessions[sessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = sessionID
	t.sessionMu.Unlock()
	t.persistSessionCwd(ctx, store, sessionID, req.Cwd)
	t.clearToolCallState(sessionID)

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

	// The ACP session id is the message-index NAME (session/new mints it and
	// agentservice resolves loads by name); mi.id is contenox-internal. Rows
	// without a name predate ACP naming and cannot be loaded, so they are not
	// listed.
	rows, err := exec.QueryContext(ctx, `
		SELECT mi.name,
		       (SELECT MAX(m.added_at) FROM messages m WHERE m.idx_id = mi.id)
		FROM message_indices mi
		WHERE mi.workspace_id = $1
		  AND mi.identity = 'acp-client'
		  AND mi.name IS NOT NULL AND mi.name != ''
		ORDER BY mi.id DESC`, t.workspaceID())
	if err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: list sessions: %w", err)
	}
	defer rows.Close()

	store := runtimetypes.New(exec)
	var sessions []libacp.SessionInfo
	for rows.Next() {
		var name string
		var updatedAt any
		if err := rows.Scan(&name, &updatedAt); err != nil {
			return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: scan session: %w", err)
		}

		info := libacp.SessionInfo{
			SessionID: libacp.SessionID(name),
			Title:     name,
			Cwd:       t.sessionCwd(ctx, store, libacp.SessionID(name)),
		}
		if ts, ok := parseDBTime(updatedAt); ok {
			info.UpdatedAt = ts.UTC().Format(time.RFC3339)
		}

		if req.Cwd != "" && info.Cwd != "" && info.Cwd != req.Cwd {
			continue
		}

		sessions = append(sessions, info)
	}
	if err := rows.Err(); err != nil {
		return libacp.ListSessionsResponse{}, fmt.Errorf("acpsvc: rows: %w", err)
	}

	return libacp.ListSessionsResponse{Sessions: sessions}, nil
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
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}
