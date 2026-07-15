package acpsvc

import (
	"context"
	"strconv"
	"strings"
	"sync"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
)

type Deps struct {
	Engine             *enginesvc.Engine
	DB                 libdb.DBManager
	ChainRegistry      *ChainRegistry
	DefaultModel       string
	DefaultProvider    string
	DefaultAltModel    string
	DefaultAltProvider string
	DefaultMaxTokens   string
	DefaultThink       string
	WorkspaceID        string
	// ContenoxDir is the active .contenox directory, used to locate auxiliary
	// chains (e.g. chain-compact.json for the /compact command).
	ContenoxDir string

	// KnownPolicies are the HITL policy preset names shown by /policy when
	// listing. Display only — empty just omits the list.
	KnownPolicies []string
	// HITLDefaultPolicyName is the policy the engine falls back to when no
	// override is set, shown by /policy so the status is accurate. Display only.
	HITLDefaultPolicyName string

	// UpdateBanner is an optional one-shot message sent to the client as an
	// agent_message_chunk on the first session created or loaded. Empty = no banner.
	UpdateBanner string

	// EnvSetup enables the env_var auth method: in setup-only mode initialize
	// advertises Vars as the environment the client should collect/set, and
	// authenticate with the env method calls Complete to finish setup
	// non-interactively from the current environment. Nil disables the method.
	EnvSetup *EnvSetupSpec
}

// EnvSetupSpec describes environment-variable-based setup (the non-interactive
// sibling of the terminal setup wizard).
type EnvSetupSpec struct {
	Vars     []libacp.AuthEnvVar
	Complete func(ctx context.Context) error
}

type sessionEntry struct {
	mu                sync.Mutex
	WorkspaceID       string
	Cwd               string
	InternalSessionID string
	Agent             agentservice.Agent
	McpServerNames    []string
	Provider          string
	Model             string
	Think             string
	// EffectiveTokenLimit is the user-chosen (or chain default) context budget for this session.
	// It is clamped at set time (and on use) to the model's ContextLength when the model reports >0.
	// 0 means "use chain default / unlimited". This is the value shown in usage indicators as "size".
	EffectiveTokenLimit int
}

type Transport struct {
	deps Deps
	conn *libacp.AgentSideConnection
	// connectionID scopes client-supplied MCP servers to this ACP connection so
	// two clients loading the same session cannot overwrite each other's tools.
	connectionID string

	initMu     sync.Mutex
	clientInfo *libacp.Implementation
	clientCaps libacp.ClientCapabilities

	sessionMu       sync.Mutex
	sessions        map[libacp.SessionID]*sessionEntry
	contenoxToACPID map[string]libacp.SessionID

	// cfgMu guards the live model/provider, which the /model and /provider
	// commands mutate while concurrent prompts read them. The values seed from
	// Deps at construction; Deps.DefaultModel/DefaultProvider are not read again.
	cfgMu              sync.Mutex
	defaultModel       string
	defaultProvider    string
	defaultAltModel    string
	defaultAltProvider string
	defaultMaxTokens   string
	defaultThink       string

	permMu      sync.Mutex
	permPending map[string]struct{}

	toolCallMu     sync.Mutex
	toolCallStatus map[string]libacp.ToolCallStatus
	// toolCallSeq / toolCallOpen disambiguate repeated invocations of a tool
	// that has no engine-minted ApprovalID (declarative `tools` tasks): the
	// name alone would reuse one wire id for every run, merging their cards and
	// pinning the status at the never-downgrade rank of the first completion.
	toolCallSeq  map[string]int
	toolCallOpen map[string]int

	bannerMu      sync.Mutex
	pendingBanner string
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
			deps:               deps,
			conn:               conn,
			connectionID:       newSessionID("conn"),
			sessions:           make(map[libacp.SessionID]*sessionEntry),
			contenoxToACPID:    make(map[string]libacp.SessionID),
			toolCallStatus:     make(map[string]libacp.ToolCallStatus),
			defaultModel:       deps.DefaultModel,
			defaultProvider:    deps.DefaultProvider,
			defaultAltModel:    deps.DefaultAltModel,
			defaultAltProvider: deps.DefaultAltProvider,
			defaultMaxTokens:   deps.DefaultMaxTokens,
			defaultThink:       deps.DefaultThink,
			pendingBanner:      deps.UpdateBanner,
		}
	}
}

// takeBanner atomically reads and clears the pending update banner.
// Returns "" after the first call, ensuring the banner is sent at most once.
func (t *Transport) takeBanner() string {
	t.bannerMu.Lock()
	defer t.bannerMu.Unlock()
	b := t.pendingBanner
	t.pendingBanner = ""
	return b
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

func (t *Transport) altModel() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	return t.defaultAltModel
}

func (t *Transport) altProvider() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	return t.defaultAltProvider
}

func (t *Transport) maxTokens() string {
	t.cfgMu.Lock()
	defer t.cfgMu.Unlock()
	return t.defaultMaxTokens
}

// chainTemplateVars seeds the template vars every chain execution needs.
// The seeded chains reference {{var:alt_model|var:default_model}} (and the
// provider equivalent), so default_model/default_provider must always be
// set when a model is known — configured default first, falling back to the
// session's effective selection, matching the CLI chat path.
func (t *Transport) chainTemplateVars(sess *sessionEntry) map[string]string {
	vars := map[string]string{
		"model":    sess.modelOrDefault(t.model()),
		"provider": sess.providerOrDefault(t.provider()),
	}
	// default_model/default_provider are the recovery fallback for the seeded
	// chains' {{var:alt_model|var:default_model}}. They must be the
	// session-effective selection (vars["model"]/vars["provider"]), not the
	// transport-configured default: Zed's model dropdown sets a session-only
	// selection that never touches t.model(), so seeding from t.model() here
	// makes recovery/summarise_failure resolve a stale provider that may have no
	// models in runtime state while the main tasks use the working selection.
	if vars["model"] != "" {
		vars["default_model"] = vars["model"]
	}
	if vars["provider"] != "" {
		vars["default_provider"] = vars["provider"]
	}
	if altModel := t.altModel(); altModel != "" {
		vars["alt_model"] = altModel
	}
	if altProvider := t.altProvider(); altProvider != "" {
		vars["alt_provider"] = altProvider
	}
	if maxTokens := t.maxTokens(); maxTokens != "" {
		vars["max_tokens"] = maxTokens
	}
	return vars
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

func (t *Transport) setMaxTokens(v string) {
	t.cfgMu.Lock()
	t.defaultMaxTokens = v
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

func (s *sessionEntry) providerOrDefault(defaultProvider string) string {
	if s == nil {
		return defaultProvider
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Provider == "" {
		return defaultProvider
	}
	return s.Provider
}

func (s *sessionEntry) modelOrDefault(defaultModel string) string {
	if s == nil {
		return defaultModel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Model == "" {
		return defaultModel
	}
	return s.Model
}

func (s *sessionEntry) effectiveTokenLimit() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.EffectiveTokenLimit
}

func (s *sessionEntry) setEffectiveTokenLimit(v int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EffectiveTokenLimit = v
}

func (s *sessionEntry) setModelSelection(provider, model string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.Provider = provider
	s.Model = model
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
	if t.conn == nil {
		return
	}
	notif = t.normalizeToolCallNotification(notif)
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

func (t *Transport) normalizeToolCallNotification(notif libacp.SessionNotification) libacp.SessionNotification {
	upd := &notif.Update
	if upd.ToolCallID == "" {
		return notif
	}
	if upd.SessionUpdate != libacp.SessionUpdateToolCall && upd.SessionUpdate != libacp.SessionUpdateToolCallUpdate {
		return notif
	}

	t.toolCallMu.Lock()
	defer t.toolCallMu.Unlock()
	if t.toolCallStatus == nil {
		t.toolCallStatus = make(map[string]libacp.ToolCallStatus)
	}

	key := permKey(notif.SessionID, upd.ToolCallID)
	previousStatus, seen := t.toolCallStatus[key]
	if upd.SessionUpdate == libacp.SessionUpdateToolCallUpdate && !seen {
		upd.SessionUpdate = libacp.SessionUpdateToolCall
		if upd.Title == "" {
			upd.Title = upd.ToolCallID
		}
		if upd.Kind == "" {
			upd.Kind = libacp.ToolKindOther
		}
	}

	if seen && toolCallStatusRank(upd.Status) < toolCallStatusRank(previousStatus) {
		upd.Status = previousStatus
	}
	t.toolCallStatus[key] = upd.Status
	return notif
}

func (t *Transport) clearToolCallState(sid libacp.SessionID) {
	t.toolCallMu.Lock()
	defer t.toolCallMu.Unlock()
	prefix := string(sid) + "\x00"
	for key := range t.toolCallStatus {
		if strings.HasPrefix(key, prefix) {
			delete(t.toolCallStatus, key)
		}
	}
	for key := range t.toolCallSeq {
		if strings.HasPrefix(key, prefix) {
			delete(t.toolCallSeq, key)
		}
	}
	for key := range t.toolCallOpen {
		if strings.HasPrefix(key, prefix) {
			delete(t.toolCallOpen, key)
		}
	}
}

// toolCallWireID resolves the ACP tool-call id for an event. The engine's
// ApprovalID is already per-invocation; the name-derived fallback gets an
// invocation counter so repeated runs of one tool stay distinct cards. A
// pending event opens an invocation, the result event closes it (matching by
// key when one is open, else it is a result without a pending — its own
// invocation). The first invocation keeps the bare name, so single-run flows
// are wire-identical to before.
func (t *Transport) toolCallWireID(sid libacp.SessionID, ev taskengine.TaskEvent, closes bool) string {
	if ev.ApprovalID != "" {
		return ev.ApprovalID
	}
	base := fallbackToolCallID(ev)
	if base == "" {
		return ""
	}
	key := permKey(sid, ev.TaskID+"\x1f"+base)

	t.toolCallMu.Lock()
	defer t.toolCallMu.Unlock()
	if t.toolCallSeq == nil {
		t.toolCallSeq = make(map[string]int)
	}
	if t.toolCallOpen == nil {
		t.toolCallOpen = make(map[string]int)
	}

	if closes {
		if n, ok := t.toolCallOpen[key]; ok {
			delete(t.toolCallOpen, key)
			return invocationToolCallID(base, n)
		}
		t.toolCallSeq[key]++
		return invocationToolCallID(base, t.toolCallSeq[key])
	}
	t.toolCallSeq[key]++
	t.toolCallOpen[key] = t.toolCallSeq[key]
	return invocationToolCallID(base, t.toolCallSeq[key])
}

func invocationToolCallID(base string, n int) string {
	if n <= 1 {
		return base
	}
	return base + "#" + strconv.Itoa(n)
}

func toolCallStatusRank(status libacp.ToolCallStatus) int {
	switch status {
	case libacp.ToolCallStatusPending:
		return 1
	case libacp.ToolCallStatusInProgress:
		return 2
	case libacp.ToolCallStatusCompleted, libacp.ToolCallStatusFailed:
		return 3
	default:
		return 0
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

// sendInitialUsageUpdate sends a usage_update with the session's effective token budget as size
// (the chain token_limit or per-session override, clamped to model cap).
// Used falls back to 0 until token events arrive. This makes indicators based on the
// user-visible/controllable session budget, not raw model cap or default-max-tokens.
func (t *Transport) sendInitialUsageUpdate(ctx context.Context, sid libacp.SessionID) {
	// Prefer session's effective token limit (the "chain token-limit" the user switches)
	t.sessionMu.Lock()
	sess, hasSess := t.sessions[sid]
	t.sessionMu.Unlock()

	if hasSess && sess != nil {
		if eff := sess.effectiveTokenLimit(); eff > 0 {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sid,
				Update: libacp.SessionUpdate{
					SessionUpdate: libacp.SessionUpdateUsageUpdate,
					Size:          eff,
				},
			})
			return
		}
	}

	// Fallback to model cap (for cases where no explicit budget set yet)
	preferredModel := t.model()
	t.sessionMu.Lock()
	if entry, ok := t.sessions[sid]; ok && entry != nil {
		preferredModel = entry.modelOrDefault(t.model())
	}
	t.sessionMu.Unlock()

	for _, state := range t.runtimeStates(ctx) {
		for _, pulled := range state.PulledModels {
			if preferredModel != "" && pulled.Model == preferredModel && pulled.ContextLength > 0 {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sid,
					Update: libacp.SessionUpdate{
						SessionUpdate: libacp.SessionUpdateUsageUpdate,
						Size:          pulled.ContextLength,
					},
				})
				return
			}
		}
	}
	for _, state := range t.runtimeStates(ctx) {
		for _, pulled := range state.PulledModels {
			if pulled.ContextLength > 0 && (pulled.CanChat || pulled.CanPrompt) {
				t.sendUpdate(ctx, libacp.SessionNotification{
					SessionID: sid,
					Update: libacp.SessionUpdate{
						SessionUpdate: libacp.SessionUpdateUsageUpdate,
						Size:          pulled.ContextLength,
					},
				})
				return
			}
		}
	}
}
