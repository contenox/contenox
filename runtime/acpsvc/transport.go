package acpsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	libacp "github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/agentservice"
	"github.com/contenox/contenox/runtime/enginesvc"
	"github.com/contenox/contenox/runtime/internal/clikv"
	"github.com/contenox/contenox/runtime/runtimetypes"
)

type Deps struct {
	Engine          *enginesvc.Engine
	DB              libdb.DBManager
	ChainRegistry   *ChainRegistry
	DefaultModel    string
	DefaultProvider string
}

type sessionEntry struct {
	WorkspaceID       string
	Cwd               string
	InternalSessionID string
	Agent             agentservice.Agent
	McpServerNames    []string
}

type Transport struct {
	deps Deps
	conn *libacp.AgentSideConnection

	initMu     sync.Mutex
	clientInfo *libacp.Implementation
	clientCaps libacp.ClientCapabilities

	sessionMu       sync.Mutex
	sessions        map[libacp.SessionID]*sessionEntry
	contenoxToACPID map[string]libacp.SessionID
}

func New(deps Deps) libacp.AgentFactory {
	return func(conn *libacp.AgentSideConnection) libacp.Agent {
		return &Transport{
			deps:            deps,
			conn:            conn,
			sessions:        make(map[libacp.SessionID]*sessionEntry),
			contenoxToACPID: make(map[string]libacp.SessionID),
		}
	}
}

func (t *Transport) acpSessionForContenoxID(contenoxSessionID string) (libacp.SessionID, bool) {
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()
	sid, ok := t.contenoxToACPID[contenoxSessionID]
	return sid, ok
}

func (t *Transport) Cancel(_ context.Context, _ libacp.CancelNotification) error {
	return nil
}

func (t *Transport) clientIdentity() *libacp.Implementation {
	t.initMu.Lock()
	defer t.initMu.Unlock()
	return t.clientInfo
}

func (t *Transport) getClientCaps() libacp.ClientCapabilities {
	t.initMu.Lock()
	defer t.initMu.Unlock()
	return t.clientCaps
}

func deriveWorkspaceID(sessionID libacp.SessionID, clientInfo *libacp.Implementation) string {
	if clientInfo != nil && clientInfo.Name != "" {
		key := clientInfo.Name + "/" + clientInfo.Version
		sum := sha256.Sum256([]byte(key))
		return "acp-client-" + hex.EncodeToString(sum[:8])
	}
	return "acp-sess-" + string(sessionID)
}

func (t *Transport) sendUpdate(ctx context.Context, notif libacp.SessionNotification) {
	kind := string(notif.Update.SessionUpdate)
	kv := []any{"kind", kind, "session_id", string(notif.SessionID)}
	if notif.Update.ToolCallID != "" {
		kv = append(kv, "tool_call_id", notif.Update.ToolCallID)
	}
	if notif.Update.Status != "" {
		kv = append(kv, "status", string(notif.Update.Status))
	}
	reportErr, _, end := t.tracker().Start(ctx, "send", "acp_session_update", kv...)
	defer end()
	if err := t.conn.SessionUpdate(notif); err != nil {
		reportErr(err)
	}
}

func (t *Transport) tracker() libtracker.ActivityTracker {
	if t.deps.Engine != nil && t.deps.Engine.Tracker != nil {
		return t.deps.Engine.Tracker
	}
	return libtracker.NoopTracker{}
}

func ReadConfigValue(ctx context.Context, db libdb.DBManager, key string) string {
	store := runtimetypes.New(db.WithoutTransaction())
	return strings.TrimSpace(clikv.Read(ctx, store, key))
}
