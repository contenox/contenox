package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	libacp "github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/runtime/agentservice"
	"github.com/contenox/contenox/runtime/runtimetypes"
	"github.com/contenox/contenox/runtime/taskengine"
)

const mcpNamePrefix = "acp-"

func mcpNameFor(sessionID libacp.SessionID, original string) string {
	return mcpNamePrefix + string(sessionID) + "-" + original
}

func (t *Transport) LoadSession(ctx context.Context, req libacp.LoadSessionRequest) (libacp.LoadSessionResponse, error) {
	if req.SessionID == "" {
		return libacp.LoadSessionResponse{}, libacp.NewError(libacp.ErrInvalidParams, "sessionId is required")
	}
	workspaceID := deriveWorkspaceID(req.SessionID, t.clientIdentity())

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	registered, err := t.registerMcpServers(ctx, store, req.SessionID, req.McpServers)
	if err != nil {
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
		return libacp.LoadSessionResponse{}, libacp.NewErrorf(libacp.ErrInvalidParams, "load session %q: %v", req.SessionID, err)
	}

	entry := &sessionEntry{
		WorkspaceID:       workspaceID,
		Cwd:               req.Cwd,
		InternalSessionID: contenoxSessionID,
		Agent:             ag,
		McpServerNames:    registered,
	}
	t.sessionMu.Lock()
	t.sessions[req.SessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = req.SessionID
	t.sessionMu.Unlock()

	t.replayMessages(req.SessionID, messages)

	return libacp.LoadSessionResponse{}, nil
}

func (t *Transport) replayMessages(sessionID libacp.SessionID, messages []taskengine.Message) {
	for _, m := range messages {
		switch m.Role {
		case "user":
			if m.Content == "" {
				continue
			}
			_ = t.conn.SessionUpdate(libacp.SessionNotification{
				SessionID: sessionID,
				Update:    libacp.NewUserMessageChunk(m.Content),
			})
		case "assistant":
			if m.Thinking != "" {
				_ = t.conn.SessionUpdate(libacp.SessionNotification{
					SessionID: sessionID,
					Update:    libacp.NewAgentThoughtChunk(m.Thinking),
				})
			}
			if m.Content != "" {
				_ = t.conn.SessionUpdate(libacp.SessionNotification{
					SessionID: sessionID,
					Update:    libacp.NewAgentMessageChunk(m.Content),
				})
			}
		}
	}
}

func (t *Transport) NewSession(ctx context.Context, req libacp.NewSessionRequest) (libacp.NewSessionResponse, error) {
	internalID := newSessionID()
	sessionID := libacp.SessionID(internalID)

	workspaceID := deriveWorkspaceID(sessionID, t.clientIdentity())

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	registered, err := t.registerMcpServers(ctx, store, sessionID, req.McpServers)
	if err != nil {
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
		return libacp.NewSessionResponse{}, fmt.Errorf("acpsvc: agent.SessionNew: %w", err)
	}

	entry := &sessionEntry{
		WorkspaceID:       workspaceID,
		Cwd:               req.Cwd,
		InternalSessionID: contenoxSessionID,
		Agent:             ag,
		McpServerNames:    registered,
	}
	t.sessionMu.Lock()
	t.sessions[sessionID] = entry
	t.contenoxToACPID[contenoxSessionID] = sessionID
	t.sessionMu.Unlock()

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

func newSessionID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
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

func (t *Transport) ListSessions(ctx context.Context, req libacp.ListSessionsRequest) (libacp.ListSessionsResponse, error) {
	exec := t.deps.DB.WithoutTransaction()

	// Query all sessions from ACP-derived workspaces.
	// ACP workspaces are prefixed with "acp-" (see deriveWorkspaceID).
	rows, err := exec.QueryContext(ctx, `
		SELECT mi.id, mi.workspace_id, COALESCE(mi.name, ''),
		       COALESCE(
		         (SELECT MAX(m.added_at) FROM messages m WHERE m.idx_id = mi.id),
		         ''
		       )
		FROM message_indices mi
		WHERE mi.workspace_id LIKE 'acp-%'
		  AND mi.identity = 'acp-client'
		ORDER BY mi.id DESC`)
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
