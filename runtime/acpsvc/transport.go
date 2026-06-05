package acpsvc

import (
	"context"
	"strings"
	"sync"

	libacp "github.com/contenox/agent/libacp"
	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/agentservice"
	"github.com/contenox/agent/runtime/enginesvc"
	"github.com/contenox/agent/runtime/internal/clikv"
	"github.com/contenox/agent/runtime/runtimetypes"
)

type Deps struct {
	Engine          *enginesvc.Engine
	DB              libdb.DBManager
	ChainRegistry   *ChainRegistry
	DefaultModel    string
	DefaultProvider string
	DefaultThink    string
	WorkspaceID     string
	// ContenoxDir is the active .contenox directory, used to locate auxiliary
	// chains (e.g. chain-compact.json for the /compact command).
	ContenoxDir string

	// KnownPolicies are the HITL policy preset names shown by /policy when
	// listing. Display only — empty just omits the list.
	KnownPolicies []string
	// HITLDefaultPolicyName is the policy the engine falls back to when no
	// override is set, shown by /policy so the status is accurate. Display only.
	HITLDefaultPolicyName string
}

type sessionEntry struct {
	mu                sync.Mutex
	WorkspaceID       string
	Cwd               string
	InternalSessionID string
	Agent             agentservice.Agent
	McpServerNames    []string
	Think             string
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

	// cfgMu guards the live model/provider, which the /model and /provider
	// commands mutate while concurrent prompts read them. The values seed from
	// Deps at construction; Deps.DefaultModel/DefaultProvider are not read again.
	cfgMu           sync.Mutex
	defaultModel    string
	defaultProvider string
	defaultThink    string

	permMu      sync.Mutex
	permPending map[string]struct{}
}

func permKey(sid libacp.SessionID, toolCallID string) string {
	return string(sid) + "\x00" + toolCallID
}

func (t *Transport) markPermissionPending(sid libacp.SessionID, toolCallID string) {
	t.permMu.Lock()
	if t.permPending == nil {
		t.permPending = make(map[string]struct{})
	}
	t.permPending[permKey(sid, toolCallID)] = struct{}{}
	t.permMu.Unlock()
}

func (t *Transport) clearPermissionPending(sid libacp.SessionID, toolCallID string) {
	t.permMu.Lock()
	delete(t.permPending, permKey(sid, toolCallID))
	t.permMu.Unlock()
}

func (t *Transport) sendToolCallUpdateGuarded(ctx context.Context, sid libacp.SessionID, toolCallID string, notif libacp.SessionNotification) {
	t.permMu.Lock()
	defer t.permMu.Unlock()
	if _, pending := t.permPending[permKey(sid, toolCallID)]; pending {
		return
	}
	t.sendUpdate(ctx, notif)
}

func New(deps Deps) libacp.AgentFactory {
	return func(conn *libacp.AgentSideConnection) libacp.Agent {
		return &Transport{
			deps:            deps,
			conn:            conn,
			sessions:        make(map[libacp.SessionID]*sessionEntry),
			contenoxToACPID: make(map[string]libacp.SessionID),
			defaultModel:    deps.DefaultModel,
			defaultProvider: deps.DefaultProvider,
			defaultThink:    deps.DefaultThink,
		}
	}
}

// model returns the live default model, which /model may have changed since
// startup. Safe for concurrent reads/writes against the command handlers.
func (t *Transport) model() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	return t.defaultModel
}

// provider returns the live default provider, which /provider may have changed.
func (t *Transport) provider() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	return t.defaultProvider
}

func (t *Transport) setModel(v string) {
	t.cfgMu.Lock()
	t.defaultModel = v
	t.cfgMu.Unlock()
}

func (t *Transport) setProvider(v string) {
	t.cfgMu.Lock()
	t.defaultProvider = v
	t.cfgMu.Unlock()
}

func (t *Transport) thinkDefault() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	if t.defaultThink == "" {
		return "high"
	}
	return t.defaultThink
}

func (s *sessionEntry) think() string {
	if s == nil {
		return "high"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Think == "" {
		return "high"
	}
	return s.Think
}

func (s *sessionEntry) setThink(v string) {
	s.mu.Lock()
	s.Think = v
	s.mu.Unlock()
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

func (t *Transport) workspaceID() string {
	return t.deps.WorkspaceID
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

func resolveACPSessionID(ctx context.Context, t *Transport) libacp.SessionID {
	contenoxSessionID := sessionIDFromCtx(ctx)
	if contenoxSessionID == "" {
		return ""
	}
	acpSID, _ := t.acpSessionForContenoxID(contenoxSessionID)
	return acpSID
}

func sessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	return v
}
