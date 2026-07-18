package acpsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	libacp "github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agenthost"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/version"
)

// AgentMetaKey is the session/new (and session/list) `_meta` key a client uses
// to bind a session to a REGISTERED external ACP agent instead of the native
// task-chain engine: `{"contenox.agent": "<registered agent name>"}`. Absent =
// native chain path, byte-for-byte the historical behavior. It is a contenox
// extension living in the spec's reserved `_meta` namespace (the same precedent
// as WorkspaceConfigOptionsMetaKey); conformant clients that don't recognize it
// ignore `_meta` entirely.
const AgentMetaKey = "contenox.agent"

// AgentModeConfigOptionID is the reserved SessionConfigOption id under which an
// external session surfaces the DOWNSTREAM agent's session Modes (its
// SessionModeState) as a single synthetic "select" picker in the upstream
// client's toolbar. The ACP spec models session modes (session/set_mode +
// SessionModeState) and config options (session/set_config_option +
// SessionConfigOption) as two distinct surfaces; contenox does not expose a
// first-class mode toggle to its clients, so a downstream agent that advertises
// modes only (claude-code-acp: default/acceptEdits/plan/dontAsk/bypassPermissions,
// zero configOptions) would otherwise render an empty toolbar. Mapping the modes
// onto one synthetic config option — id AgentModeConfigOptionID, label "Mode",
// type "select", each availableMode as a value(id)→label(name), currentValue the
// currentModeId — lets the existing config-option picker render it; a set on this
// id is translated back to session/set_mode (see externalDriver.SetConfigOption),
// and a downstream current_mode_update is relayed as a config_option_update over
// this same id. It lives in contenox's reserved dotted namespace so it never
// collides with a downstream agent's own option ids.
const AgentModeConfigOptionID = "contenox.agent-mode"

// syntheticModeOption maps a downstream SessionModeState onto the single synthetic
// "Mode" select the upstream toolbar renders (see AgentModeConfigOptionID). Returns
// ok=false when there are no modes to surface (nil state or empty availableModes),
// so an agent that advertises no modes yields no synthetic option.
func syntheticModeOption(m *libacp.SessionModeState) (libacp.SessionConfigOption, bool) {
	if m == nil || len(m.AvailableModes) == 0 {
		return libacp.SessionConfigOption{}, false
	}
	values := make([]libacp.SessionConfigValue, 0, len(m.AvailableModes))
	for _, mode := range m.AvailableModes {
		values = append(values, libacp.SessionConfigValue{
			Value:       mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return libacp.SessionConfigOption{
		ID:           AgentModeConfigOptionID,
		Name:         "Mode",
		Type:         libacp.SessionConfigOptionTypeSelect,
		CurrentValue: m.CurrentModeID,
		Options:      libacp.NewSessionConfigValues(values),
	}, true
}

// externalKillGrace bounds how long a spawned downstream agent's teardown waits
// for it to exit on stdin-close before killing it. Persistent agents (most
// editor adapters) never exit on stdin-close, so a short grace keeps CloseSession
// / connection teardown from stalling on the default (see agenthost.KillGrace).
const externalKillGrace = 2 * time.Second

// parseAgentMeta extracts the AgentMetaKey value from a request `_meta`. Missing
// key, malformed json, or a non-string value all read as "" (no external agent),
// so a client that ships unrelated `_meta` still lands on the native path.
func parseAgentMeta(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(meta, &m) != nil {
		return ""
	}
	raw, ok := m[AgentMetaKey]
	if !ok {
		return ""
	}
	var name string
	if json.Unmarshal(raw, &name) != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

// agentMetaJSON builds the `{"contenox.agent": name}` object echoed on the
// session/new response and each external session/list entry.
func agentMetaJSON(name string) json.RawMessage {
	return mustJSON(map[string]any{AgentMetaKey: name})
}

// sessionAgentRecord is the durable KV shape for an external session's agent
// name, stored alongside the cwd KV so session/list attribution and the external
// prompt path survive reconnects (the in-memory session map is empty after a
// process restart or on a fresh connection).
type sessionAgentRecord struct {
	Agent string `json:"agent"`
}

const acpSessionAgentKVPrefix = "acp:session_agent:"

// acpSessionAgentCommandsKVPrefix and acpSessionAgentConfigOptionsKVPrefix store an
// external session's downstream-advertised surface (its slash-command menu and its
// config-option pickers) durably alongside the agent-name key. A session/load or
// session/resume does NOT resurrect the downstream process (that happens lazily on
// the next prompt — see ensureSpawned), so the bridge's live cache dies with the
// pre-load connection; these keys let a reopened external session restore its menu
// and pickers immediately, without waiting for the first prompt's respawn. The
// synthetic mode select (mapped from the downstream's session Modes) is folded into
// the persisted config-option set — no separate key — so a reopened session's toolbar
// keeps its mode picker too. Fresh live values overwrite them on every (re)spawn and
// on each config-option / mode change — live truth wins. Both are deleted with the
// agent-name key on session delete.
const (
	acpSessionAgentCommandsKVPrefix      = "acp:session_agent_commands:"
	acpSessionAgentConfigOptionsKVPrefix = "acp:session_agent_configoptions:"
)

func (t *Transport) persistSessionAgent(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID, agentName string) {
	if agentName == "" {
		return
	}
	raw, err := json.Marshal(sessionAgentRecord{Agent: agentName})
	if err != nil {
		return
	}
	if err := store.SetKV(ctx, acpSessionAgentKVPrefix+string(sid), raw); err != nil {
		reportErr, _, end := t.tracker().Start(ctx, "persist_agent", "acp_session", "session_id", string(sid))
		reportErr(err)
		end()
	}
}

func (t *Transport) readSessionAgent(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) string {
	var rec sessionAgentRecord
	if err := store.GetKV(ctx, acpSessionAgentKVPrefix+string(sid), &rec); err != nil {
		return ""
	}
	return rec.Agent
}

// persistSessionAgentCommands durably records an external session's downstream
// slash-command menu (full-replacement per spec) keyed by the upstream session id,
// so a later session/load can re-emit it before the downstream is respawned.
// Best-effort, mirroring persistSessionAgent; builds its own store so the bridge
// (which holds no store) can call it.
func (t *Transport) persistSessionAgentCommands(ctx context.Context, sid libacp.SessionID, cmds []libacp.AvailableCommand) {
	if t.deps.DB == nil {
		return
	}
	raw, err := json.Marshal(cmds)
	if err != nil {
		return
	}
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	if err := store.SetKV(ctx, acpSessionAgentCommandsKVPrefix+string(sid), raw); err != nil {
		reportErr, _, end := t.tracker().Start(ctx, "persist_agent_commands", "acp_session", "session_id", string(sid))
		reportErr(err)
		end()
	}
}

// persistSessionAgentConfigOptions durably records an external session's downstream
// config-option set (full-replacement per spec) keyed by the upstream session id, so
// a later session/load can carry them in its response before the downstream is
// respawned. Best-effort; builds its own store for the same reason as above.
func (t *Transport) persistSessionAgentConfigOptions(ctx context.Context, sid libacp.SessionID, opts []libacp.SessionConfigOption) {
	if t.deps.DB == nil {
		return
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		return
	}
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	if err := store.SetKV(ctx, acpSessionAgentConfigOptionsKVPrefix+string(sid), raw); err != nil {
		reportErr, _, end := t.tracker().Start(ctx, "persist_agent_config_options", "acp_session", "session_id", string(sid))
		reportErr(err)
		end()
	}
}

// readSessionAgentCommands returns the persisted downstream slash-command menu for
// an external session, or nil when none was stored or on a read failure.
func (t *Transport) readSessionAgentCommands(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) []libacp.AvailableCommand {
	var cmds []libacp.AvailableCommand
	if err := store.GetKV(ctx, acpSessionAgentCommandsKVPrefix+string(sid), &cmds); err != nil {
		return nil
	}
	return cmds
}

// readSessionAgentConfigOptions returns the persisted downstream config-option set
// for an external session, or nil when none was stored or on a read failure.
func (t *Transport) readSessionAgentConfigOptions(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) []libacp.SessionConfigOption {
	var opts []libacp.SessionConfigOption
	if err := store.GetKV(ctx, acpSessionAgentConfigOptionsKVPrefix+string(sid), &opts); err != nil {
		return nil
	}
	return opts
}

// acpSessionHITLPolicyKVPrefix stores an external session's contenox-NATIVE HITL
// policy selection (the per-session approval policy the runtime enforces for its
// runtime-mediated actions — the terminal bridge, future fs — and that drives
// beam's file-explorer HITL labels) durably alongside the agent-name key. The
// native chain path keeps this selection only in-memory on the sessionEntry
// (reset to the sentinel default on every rebuild); an external session persists
// it because — like its downstream config-option surface — its toolbar is
// restored from persistence on a session/load, before any prompt respawns the
// downstream. Deleted with the agent-name key on session delete.
const acpSessionHITLPolicyKVPrefix = "acp:session_hitl_policy:"

type sessionHITLPolicyRecord struct {
	Policy string `json:"policy"`
}

// persistSessionHITLPolicy durably records an external session's contenox-native
// HITL policy selection keyed by the upstream session id. Best-effort, mirroring
// persistSessionAgentConfigOptions; builds its own store so a driver holding no
// store can call it.
func (t *Transport) persistSessionHITLPolicy(ctx context.Context, sid libacp.SessionID, policy string) {
	if t.deps.DB == nil {
		return
	}
	raw, err := json.Marshal(sessionHITLPolicyRecord{Policy: policy})
	if err != nil {
		return
	}
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	if err := store.SetKV(ctx, acpSessionHITLPolicyKVPrefix+string(sid), raw); err != nil {
		reportErr, _, end := t.tracker().Start(ctx, "persist_hitl_policy", "acp_session", "session_id", string(sid))
		reportErr(err)
		end()
	}
}

// readSessionHITLPolicy returns the persisted contenox-native HITL policy selection
// for an external session, or "" when none was stored or on a read failure.
func (t *Transport) readSessionHITLPolicy(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) string {
	var rec sessionHITLPolicyRecord
	if err := store.GetKV(ctx, acpSessionHITLPolicyKVPrefix+string(sid), &rec); err != nil {
		return ""
	}
	return rec.Policy
}

// externalBridge is the DOWNSTREAM-connection libacp.Client for an external-
// agent-backed session. It relays the downstream agent's session/update stream
// up to the connected upstream client (remapping only the session id to the
// upstream ACP session, passing the Update through as-is), forwards
// session/request_permission up to the upstream client (beam answers it), and
// services the terminal/* client-callback family (external_terminal.go) by
// mapping onto the runtime's own shell-session machinery, so a downstream
// agent's shell commands run through the RUNTIME's terminals and stream live
// into beam's terminal panel — the same path the `!` passthrough uses. fs/* is
// still refused with MethodNotFound via the embedded UnimplementedClient
// (withholding a filesystem harness for the downstream agent is a deliberate
// v1 limit of this slice).
type externalBridge struct {
	libacp.UnimplementedClient

	t          *Transport
	upstreamID libacp.SessionID

	// mu guards capture, bound, and cachedCommands. capture is the per-turn
	// accumulator of the downstream agent's agent_message_chunk text:
	// externalDriver.Prompt sets a fresh builder before the downstream
	// session/prompt and reads it back after, so the reply can be persisted for
	// session/load replay. SessionUpdate runs on the downstream read-loop
	// goroutine; externalDriver.Prompt runs on the upstream request goroutine.
	mu      sync.Mutex
	capture *strings.Builder

	// bound reports whether the upstream client can resolve upstreamID yet — i.e.
	// whether the session/new response carrying this session id has reached it. A
	// downstream available_commands_update relayed BEFORE that point references a
	// session the client has not learned and is silently dropped (see beam's
	// handleNotification), which is exactly why the downstream agent's slash menu
	// never rendered. While unbound the menu is cached, not relayed; markBound (run
	// via libacp.AfterResponse from the external session/new handler) flips this and
	// flushes the cached menu, mirroring sendAvailableCommands' ordering contract. A
	// bridge spawned lazily after a session/load starts bound: its upstream session
	// already exists, so live relay reaches the client directly.
	bound bool
	// cachedCommands is the latest downstream available_commands_update payload
	// (full-replacement semantics per spec). Kept so the menu can be (re)emitted the
	// moment the upstream session becomes resolvable.
	cachedCommands []libacp.AvailableCommand

	// configOptions is the downstream agent's own advertised config-option set
	// (full-replacement per spec), seeded from the downstream session/new response
	// and replaced by each downstream config_option_update (and each confirmed
	// session/set_config_option). externalDriver.ConfigOptions returns it verbatim,
	// so the upstream client renders the downstream agent's real pickers.
	configOptions []libacp.SessionConfigOption
	// configReceived records that a LIVE downstream config-option payload (a
	// config_option_update, or a session/set_config_option confirmation) has
	// superseded the session/new seed, so a late seed write never clobbers it.
	configReceived bool
	// configOptionsPending mirrors cachedCommands' gating for config_option_update:
	// a downstream update arriving BEFORE the upstream session/new response is on the
	// wire references a session the client cannot resolve yet, so it is cached and
	// re-emitted by markBound instead of relayed live and dropped. It also gates a
	// mode change: a current_mode_update is surfaced as a config_option_update over
	// the synthetic mode id, so the same pre-bind hold applies.
	configOptionsPending bool

	// modeState is the downstream agent's session Modes (SessionModeState), seeded
	// from the downstream session/new response and kept current by each downstream
	// current_mode_update and each confirmed session/set_mode. It is folded into the
	// driver's config-option output as the synthetic AgentModeConfigOptionID select
	// (mode first, ahead of the downstream's own configOptions). nil when the
	// downstream advertises no modes (e.g. a bare agent) — no synthetic option then.
	modeState *libacp.SessionModeState
	// modeReceived records that a LIVE mode change (a current_mode_update, or a
	// confirmed session/set_mode) has landed, so a late session/new seed cannot
	// clobber the current mode — mirroring configReceived for the config options.
	modeReceived bool
	// pendingModeID holds a currentModeId from a current_mode_update that raced
	// AHEAD of the session/new seed (so modeState — which carries availableModes — is
	// not built yet); seedModes applies it once the availableModes arrive.
	pendingModeID string

	// termMu guards terminals, the live set of downstream-created terminals for
	// this session keyed by the bridge-minted terminal id. It is independent of mu
	// (which guards the update-relay caches) so terminal lifecycle never contends
	// with the session/update stream. Each bridgeTerminal owns a shell-session
	// scrollback watcher and is torn down on terminal/release, terminal/kill, or
	// connection/session teardown (closeAllTerminals). See external_terminal.go.
	termMu    sync.Mutex
	terminals map[string]*bridgeTerminal
}

// SessionUpdate relays a downstream session/update to the upstream client and,
// when a turn's capture is active, accumulates agent_message_chunk text for
// history persistence.
func (b *externalBridge) SessionUpdate(ctx context.Context, n libacp.SessionNotification) error {
	// available_commands_update carries the downstream agent's slash-command menu
	// (full-replacement per spec). Cache the latest, and relay live only once the
	// upstream client can resolve this session — a menu relayed before the
	// session/new response is dropped as referencing an unknown session, so while
	// unbound it is held and re-emitted by markBound after the response.
	if n.Update.SessionUpdate == libacp.SessionUpdateAvailableCommands {
		b.mu.Lock()
		b.cachedCommands = n.Update.AvailableCommands
		relay := b.bound
		b.mu.Unlock()
		// Persist the menu (live truth) so a later session/load can re-emit it before
		// the downstream is respawned. Independent of bind state — the menu can be
		// stored the moment it arrives even if the upstream relay is still gated.
		b.persistCommands(ctx, n.Update.AvailableCommands)
		if relay {
			b.t.relayExternalUpdate(ctx, b.upstreamID, n.Update)
		}
		return nil
	}
	// config_option_update carries the downstream agent's own config pickers
	// (full-replacement per spec). Cache the latest as the driver's live option set
	// and relay only once the upstream session is resolvable — same pre-bind gating
	// as the command menu (a pre-bind update is held and flushed by markBound).
	if n.Update.SessionUpdate == libacp.SessionUpdateConfigOption {
		b.mu.Lock()
		b.configReceived = true
		b.configOptions = n.Update.ConfigOptions
		merged := b.buildConfigOptionsLocked()
		relay := b.bound
		if !relay {
			b.configOptionsPending = true
		}
		b.mu.Unlock()
		// Persist the pickers (live truth, synthetic mode option folded in) so a later
		// session/load can carry them in its response before the downstream respawns.
		b.persistConfigOptions(ctx, merged)
		if relay {
			b.t.relayExternalUpdate(ctx, b.upstreamID, libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateConfigOption,
				ConfigOptions: merged,
			})
		}
		return nil
	}
	// current_mode_update carries the downstream agent's newly-current session mode.
	// contenox surfaces the downstream's modes as the synthetic AgentModeConfigOptionID
	// config option (not a first-class client mode toggle), so this is TRANSLATED into a
	// config_option_update carrying the refreshed synthetic set — the raw
	// current_mode_update is not forwarded. Same pre-bind gating as the config pickers:
	// while unbound it is held and re-emitted by markBound after the session/new result.
	if n.Update.SessionUpdate == libacp.SessionUpdateCurrentMode {
		b.mu.Lock()
		b.modeReceived = true
		if b.modeState != nil {
			b.modeState.CurrentModeID = n.Update.CurrentModeID
		} else {
			// Raced ahead of the session/new seed; remember the new current for seedModes.
			b.pendingModeID = n.Update.CurrentModeID
		}
		merged := b.buildConfigOptionsLocked()
		relay := b.bound
		if !relay {
			b.configOptionsPending = true
		}
		b.mu.Unlock()
		b.persistConfigOptions(ctx, merged)
		if relay {
			b.t.relayExternalUpdate(ctx, b.upstreamID, libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateConfigOption,
				ConfigOptions: merged,
			})
		}
		return nil
	}
	b.t.relayExternalUpdate(ctx, b.upstreamID, n.Update)
	if n.Update.SessionUpdate == libacp.SessionUpdateAgentMessageChunk {
		if c := n.Update.Content; c != nil && c.Type == string(libacp.ContentKindText) {
			b.mu.Lock()
			if b.capture != nil {
				b.capture.WriteString(c.Text)
			}
			b.mu.Unlock()
		}
	}
	return nil
}

// markBound records that the upstream client can now resolve this session (its
// session/new response is on the wire) and flushes the latest cached downstream
// available_commands_update. Scheduled via libacp.AfterResponse from the external
// session/new handler so the menu reaches the client strictly AFTER the result —
// the same ordering contract sendAvailableCommands documents for the native menu.
// Idempotent, and a no-op when the downstream has advertised no menu yet: a menu
// that arrives after this point relays live, since the session is now bound.
func (b *externalBridge) markBound(ctx context.Context) {
	b.mu.Lock()
	if b.bound {
		b.mu.Unlock()
		return
	}
	b.bound = true
	cmds := b.cachedCommands
	flushConfig := b.configOptionsPending
	var configOpts []libacp.SessionConfigOption
	if flushConfig {
		configOpts = b.buildConfigOptionsLocked()
	}
	b.mu.Unlock()
	if cmds != nil {
		b.t.relayExternalUpdate(ctx, b.upstreamID, libacp.SessionUpdate{
			SessionUpdate:     libacp.SessionUpdateAvailableCommands,
			AvailableCommands: cmds,
		})
	}
	// A config_option_update or current_mode_update that raced ahead of the
	// session/new response (cached pre-bind, unlike the seed which the response
	// already carried) is flushed now that the client can resolve this session — as a
	// config_option_update carrying the merged synthetic-mode-first set.
	if flushConfig {
		b.t.relayExternalUpdate(ctx, b.upstreamID, libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateConfigOption,
			ConfigOptions: configOpts,
		})
	}
}

// seedConfigOptions records the downstream session/new response's advertised
// config options as the initial set, unless a live downstream update has already
// superseded the seed (the downstream read-loop can deliver a config_option_update
// concurrently with spawnExternal capturing the response).
func (b *externalBridge) seedConfigOptions(opts []libacp.SessionConfigOption) {
	b.mu.Lock()
	if !b.configReceived {
		b.configOptions = opts
	}
	b.mu.Unlock()
}

// applyConfigOptions adopts a downstream-confirmed option set (from a
// session/set_config_option response), marking the live set as received so a
// later seed cannot clobber it.
func (b *externalBridge) applyConfigOptions(opts []libacp.SessionConfigOption) {
	b.mu.Lock()
	b.configReceived = true
	b.configOptions = opts
	b.mu.Unlock()
}

// seedModes records the downstream session/new response's SessionModeState as the
// initial mode set (copied, so later currentModeId mutations stay local). A
// current_mode_update that raced ahead of this seed left its new currentModeId in
// pendingModeID; it is folded in so the raced update is not lost. Never overwrites an
// already-established modeState.
func (b *externalBridge) seedModes(m *libacp.SessionModeState) {
	if m == nil {
		return
	}
	b.mu.Lock()
	if b.modeState == nil {
		cp := *m
		if b.modeReceived && b.pendingModeID != "" {
			cp.CurrentModeID = b.pendingModeID
		}
		b.modeState = &cp
	}
	b.mu.Unlock()
}

// applyMode adopts an upstream-confirmed session mode (from a session/set_mode the
// driver forwarded downstream) into the synthetic option's currentValue. The
// set_mode response carries no state, so the requested modeId is authoritative; the
// downstream's own current_mode_update, if it also emits one, reconfirms it.
func (b *externalBridge) applyMode(modeID string) {
	b.mu.Lock()
	b.modeReceived = true
	if b.modeState != nil {
		b.modeState.CurrentModeID = modeID
	} else {
		b.pendingModeID = modeID
	}
	b.mu.Unlock()
}

// buildConfigOptionsLocked assembles the driver's full upstream config-option set:
// the synthetic downstream-mode select FIRST (when the downstream advertises modes),
// then the downstream agent's own config options. Caller holds b.mu.
func (b *externalBridge) buildConfigOptionsLocked() []libacp.SessionConfigOption {
	opt, ok := syntheticModeOption(b.modeState)
	if !ok {
		return b.configOptions
	}
	out := make([]libacp.SessionConfigOption, 0, len(b.configOptions)+1)
	out = append(out, opt)
	out = append(out, b.configOptions...)
	return out
}

// snapshotConfigOptions returns the driver's current upstream option set: the
// synthetic downstream-mode select first (when present), then the downstream agent's
// own options.
func (b *externalBridge) snapshotConfigOptions() []libacp.SessionConfigOption {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buildConfigOptionsLocked()
}

// persistCommands durably records the latest downstream command menu keyed by this
// bridge's upstream session id, so a later session/load restores it before a respawn.
func (b *externalBridge) persistCommands(ctx context.Context, cmds []libacp.AvailableCommand) {
	b.t.persistSessionAgentCommands(ctx, b.upstreamID, cmds)
}

// persistConfigOptions durably records the latest downstream config-option set keyed
// by this bridge's upstream session id, so a later session/load restores the pickers
// before a respawn.
func (b *externalBridge) persistConfigOptions(ctx context.Context, opts []libacp.SessionConfigOption) {
	b.t.persistSessionAgentConfigOptions(ctx, b.upstreamID, opts)
}

// RequestPermission forwards the downstream agent's permission request to the
// upstream client, remapping the session id. The upstream client (beam) already
// answers session/request_permission — this reuses the exact path AskApproval
// uses for the native engine, calling the upstream connection directly.
func (b *externalBridge) RequestPermission(ctx context.Context, req libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error) {
	if b.t.conn == nil {
		return libacp.RequestPermissionResponse{}, libacp.InternalError("acpsvc: no upstream connection to relay permission to")
	}
	req.SessionID = b.upstreamID
	return b.t.conn.RequestPermission(ctx, req)
}

func (b *externalBridge) beginCapture() {
	b.mu.Lock()
	b.capture = &strings.Builder{}
	b.mu.Unlock()
}

func (b *externalBridge) finishCapture() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.capture == nil {
		return ""
	}
	s := b.capture.String()
	b.capture = nil
	return s
}

// relayExternalUpdate forwards a downstream agent's session/update to the
// connected upstream client, remapping only the session id. Unlike sendUpdate it
// applies no tool-call normalization: the downstream agent is a conformant ACP peer
// and owns its own tool-call framing. The one enrichment is on config_option_update:
// contenox's own per-session HITL policy select is appended so the relayed set stays
// consistent with the set_config_option response (see externalConfigOptionsForRelay).
func (t *Transport) relayExternalUpdate(ctx context.Context, upstreamID libacp.SessionID, upd libacp.SessionUpdate) {
	if t.conn == nil {
		return
	}
	if upd.SessionUpdate == libacp.SessionUpdateConfigOption {
		upd.ConfigOptions = t.externalConfigOptionsForRelay(upstreamID, upd.ConfigOptions)
	}
	reportErr, _, end := t.tracker().Start(ctx, "relay", "acp_session_update",
		"session_id", string(upstreamID), "kind", string(upd.SessionUpdate))
	defer end()
	if err := t.conn.SessionUpdate(libacp.SessionNotification{SessionID: upstreamID, Update: upd}); err != nil {
		reportErr(err)
	}
}

// externalConfigOptionsForRelay appends contenox's OWN per-session HITL policy select
// to a downstream/merged config-option set bound for a relayed config_option_update.
// The upstream client applies a config_option_update as a WHOLESALE replacement of the
// session's options (beam's reducer), so a relay carrying only the downstream/synthetic
// surface would blank the HITL picker mid-session — and with it the file-explorer HITL
// labels — until the next set_config_option response restored it. Appending here keeps
// every relay path (downstream update, confirmed set, markBound flush, lazy-respawn
// push) consistent with that response. Falls back to the bare set when the session is
// not resolvable (it always is for a live relay).
func (t *Transport) externalConfigOptionsForRelay(sid libacp.SessionID, downstream []libacp.SessionConfigOption) []libacp.SessionConfigOption {
	sess, ok := t.sessionFor(sid)
	if !ok {
		return downstream
	}
	out := make([]libacp.SessionConfigOption, 0, len(downstream)+1)
	out = append(out, downstream...)
	out = append(out, t.hitlPolicyConfigOption(sess))
	return out
}

// externalDriver drives a session against a REGISTERED downstream ACP agent
// (spawned/bridged via runtime/agenthost) instead of the native chain engine. It
// owns the live downstream connection state — spawned lazily (session/new spawns
// eagerly; the first prompt after a session/load respawns) and torn down on
// Close.
type externalDriver struct {
	t         *Transport
	agentName string

	// upstreamID is the ACP session id this driver serves. Kept so a config change
	// routed through the NATIVE per-session path (contenox's own HITL policy select)
	// can persist the selection under the session's key even when no downstream is
	// spawned (a loaded session before its first prompt has a nil bridge). Set once
	// at construction, never mutated.
	upstreamID libacp.SessionID

	// mu guards the live downstream connection: set at spawn time and re-set
	// lazily on the first prompt after a session/load (which does not resurrect
	// the downstream process). nil handle means not currently spawned.
	mu           sync.Mutex
	handle       *agenthost.Handle
	downstreamID libacp.SessionID
	bridge       *externalBridge
}

// AgentName returns the registered downstream agent name, echoed in session/new's
// `_meta` and used for session/list attribution.
func (d *externalDriver) AgentName() string { return d.agentName }

// AvailableCommands returns nil: an external session bypasses contenox's slash
// commands and relays the downstream agent's own menu live.
func (d *externalDriver) AvailableCommands() []libacp.AvailableCommand { return nil }

// ConfigOptions returns the DOWNSTREAM agent's own advertised config options —
// prefixed by the synthetic "Mode" select mapped from the downstream's session
// Modes (see AgentModeConfigOptionID) when it advertises any — captured from its
// session/new response and kept current by config_option_update / current_mode_update
// relays and confirmed set_config_option / set_mode calls, and SUFFIXED by
// contenox's OWN per-session HITL policy select. The model/think/token chain selects
// stay suppressed (an external session does not drive the chain), but the HITL policy
// is a real per-session capability: the runtime gates the foreign agent's
// runtime-mediated actions (terminal bridge, future fs) under it and it drives beam's
// file-explorer HITL labels, so it is appended after the downstream surface. The
// downstream part is absent before the downstream is spawned (a loaded session before
// its first prompt has a nil bridge) — the HITL select still shows, and the lazy
// respawn pushes a config_option_update to restore the downstream pickers.
func (d *externalDriver) ConfigOptions(_ context.Context, sess *sessionEntry) []libacp.SessionConfigOption {
	d.mu.Lock()
	bridge := d.bridge
	d.mu.Unlock()
	var base []libacp.SessionConfigOption
	if bridge != nil {
		base = bridge.snapshotConfigOptions()
	}
	// A fresh slice avoids aliasing the bridge's backing array (snapshotConfigOptions
	// may return it directly), so appending here never corrupts the bridge's set.
	out := make([]libacp.SessionConfigOption, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, d.t.hitlPolicyConfigOption(sess))
	return out
}

// SetConfigOption forwards an upstream config-option change to the downstream
// agent's session/set_config_option and adopts the option set it confirms, so the
// upstream SetSessionConfigOption response (built from ConfigOptions) reflects the
// downstream's authoritative value. A downstream config_option_update, if the agent
// also emits one, relays through the bridge and updates the same cache. The value
// union (string/boolean) is forwarded intact so a boolean downstream option keeps
// its wire type. Contenox performs no upstream validation: the downstream owns its
// option semantics and rejects an unknown id/value with its own error.
func (d *externalDriver) SetConfigOption(ctx context.Context, sess *sessionEntry, configID string, value libacp.SessionConfigOptionValue) error {
	// contenox's OWN per-session HITL policy is enforced by the RUNTIME, not the
	// downstream agent: route it through the NATIVE per-session path (the exact
	// setSessionConfigOption the native driver uses — validate + store on the
	// session), never forwarding it downstream (the agent knows no such id), and
	// persist it so the selection survives a session/load that rebuilds the entry
	// with the sentinel default. This works before the downstream is (re)spawned (a
	// nil bridge after a load): the native path needs no downstream connection, so
	// the picker is settable on a loaded session too.
	if configID == configIDHITLPolicy {
		if err := d.t.setSessionConfigOption(ctx, sess, configID, value.AsString()); err != nil {
			return err
		}
		d.t.persistSessionHITLPolicy(ctx, d.upstreamID, sess.hitlPolicy())
		return nil
	}

	d.mu.Lock()
	handle, downstreamID, bridge := d.handle, d.downstreamID, d.bridge
	d.mu.Unlock()
	if handle == nil || bridge == nil {
		return libacp.NewError(libacp.ErrInvalidParams, "external agent session is not active")
	}
	// The synthetic mode option is not a real downstream config option: a set on its
	// reserved id translates to the downstream's session/set_mode, and the confirmed
	// mode is adopted into the synthetic option's currentValue. Every other id forwards
	// to the downstream's session/set_config_option unchanged.
	if configID == AgentModeConfigOptionID {
		if _, err := handle.Conn.SetSessionMode(ctx, libacp.SetSessionModeRequest{
			SessionID: downstreamID,
			ModeID:    value.AsString(),
		}); err != nil {
			return err
		}
		bridge.applyMode(value.AsString())
		// Persist the merged set (synthetic mode first) so a session/load reflects the
		// new mode even before the next prompt respawns the downstream.
		bridge.persistConfigOptions(ctx, bridge.snapshotConfigOptions())
		return nil
	}
	resp, err := handle.Conn.SetSessionConfigOption(ctx, libacp.SetSessionConfigOptionRequest{
		SessionID: downstreamID,
		ConfigID:  configID,
		Value:     value,
	})
	if err != nil {
		return err
	}
	bridge.applyConfigOptions(resp.ConfigOptions)
	// Persist the merged set (synthetic mode option folded in alongside the confirmed
	// downstream options) so a session/load reflects the new value even before the
	// next prompt respawns the downstream.
	bridge.persistConfigOptions(ctx, bridge.snapshotConfigOptions())
	return nil
}

// Close tears down the downstream connection (spawned subprocess) and clears the
// live external state. Idempotent and safe when nothing was spawned.
func (d *externalDriver) Close() error {
	d.mu.Lock()
	handle := d.handle
	bridge := d.bridge
	d.handle = nil
	d.bridge = nil
	d.downstreamID = ""
	d.mu.Unlock()
	// Tear down any live downstream-created terminals (their scrollback watchers)
	// before dropping the connection, so an explicit session/close or a stdio
	// Transport.Close leaks no watcher goroutines. The serve WebSocket path, which
	// never calls Transport.Close, is covered independently: each terminal's owner
	// goroutine also watches connCtx and self-terminates on connection teardown.
	if bridge != nil {
		bridge.closeAllTerminals()
	}
	if handle != nil {
		return handle.Close()
	}
	return nil
}

// storeMcpResolver adapts a runtimetypes.Store to agenthost.McpServerResolver so
// an external agent's mcp_servers allowlist can be resolved to ACP session/new
// wire shapes without acpsvc depending on mcpserverservice.
type storeMcpResolver struct {
	store runtimetypes.Store
}

func (r storeMcpResolver) GetByName(ctx context.Context, name string) (*runtimetypes.MCPServer, error) {
	return r.store.GetMCPServerByName(ctx, name)
}

// resolveExternalAgent resolves a registered agent by name and returns its
// external_acp config, rejecting an unknown or disabled agent with a clear
// JSON-RPC error (the client's session/new fails cleanly). The registry service
// is constructed from the existing DB dep — declared external agents are a
// polymorphic resource over the same store the transport already holds.
func (t *Transport) resolveExternalAgent(ctx context.Context, name string) (*runtimetypes.ExternalACPConfig, error) {
	if t.deps.DB == nil {
		return nil, libacp.InternalError("acpsvc: no database configured for external agents")
	}
	reg := agentregistryservice.New(t.deps.DB)
	agent, err := reg.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, libacp.NewErrorf(libacp.ErrInvalidParams, "unknown contenox.agent %q", name)
		}
		return nil, libacp.InternalError(fmt.Sprintf("acpsvc: resolve agent %q: %v", name, err))
	}
	if !agent.Enabled {
		return nil, libacp.NewErrorf(libacp.ErrInvalidParams, "contenox.agent %q is disabled", name)
	}
	cfg, err := agent.ExternalACPConfig()
	if err != nil {
		return nil, libacp.NewErrorf(libacp.ErrInvalidParams, "contenox.agent %q: %v", name, err)
	}
	return cfg, nil
}

// filterMcpForCaps mirrors agenthost's filter semantics: stdio is the protocol
// baseline and always passes; http and sse are gated on the downstream agent's
// initialize-advertised mcpCapabilities. Servers it cannot consume are dropped
// rather than forwarded to be silently ignored.
func filterMcpForCaps(servers []libacp.McpServer, caps libacp.McpCapabilities) ([]libacp.McpServer, []string) {
	kept := make([]libacp.McpServer, 0, len(servers))
	var dropped []string
	for _, srv := range servers {
		switch srv.Kind() {
		case libacp.McpServerKindHTTP:
			if !caps.HTTP {
				dropped = append(dropped, srv.Name)
				continue
			}
		case libacp.McpServerKindSSE:
			if !caps.SSE {
				dropped = append(dropped, srv.Name)
				continue
			}
		}
		kept = append(kept, srv)
	}
	return kept, dropped
}

// spawnExternal resolves agentName, spawns the downstream ACP agent under a
// bridge harness, and drives the downstream initialize + session/new, returning
// the live handle and downstream session id. The subprocess is bound to the
// connection-scoped connCtx (not the request ctx), so it lives across turns and
// is torn down on Handle.Close or connection teardown. Every failure after
// Connect closes the handle so a rejected session/new never leaks a process.
//
// bound seeds the bridge's readiness to relay the downstream slash-command menu
// live: false for the eager session/new spawn (the upstream session/new response
// is not on the wire yet, so the menu is cached and re-emitted by markBound after
// it), true for a lazy respawn after a session/load (the upstream session already
// exists, so a live relay reaches the client directly).
func (t *Transport) spawnExternal(ctx context.Context, upstreamID libacp.SessionID, cwd, agentName string, bound bool) (*agenthost.Handle, libacp.SessionID, *externalBridge, error) {
	cfg, err := t.resolveExternalAgent(ctx, agentName)
	if err != nil {
		return nil, "", nil, err
	}

	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	mcpServers, err := agenthost.ResolveForwardedMcpServers(ctx, storeMcpResolver{store: store}, cfg.McpServers)
	if err != nil {
		return nil, "", nil, libacp.InternalError(fmt.Sprintf("acpsvc: resolve mcp allowlist for agent %q: %v", agentName, err))
	}

	bridge := &externalBridge{t: t, upstreamID: upstreamID, bound: bound}
	host := &agenthost.ExternalACPAgent{Config: *cfg, KillGrace: externalKillGrace}
	handle, err := host.Connect(t.connCtx, bridge)
	if err != nil {
		return nil, "", nil, libacp.InternalError(fmt.Sprintf("acpsvc: spawn agent %q: %v", agentName, err))
	}

	// Advertise the terminal client capability to the downstream agent ONLY when a
	// shell manager is present to service terminal/* — otherwise CreateTerminal
	// would answer MethodNotFound and the advertisement would be a lie. With it
	// advertised, a downstream agent (e.g. claude-code-acp) routes its shell
	// commands through the runtime's terminals (external_terminal.go) instead of
	// running them inside its own process, so beam's terminal panel shows them live.
	clientCaps := libacp.ClientCapabilities{}
	if t.deps.ShellSessions != nil {
		clientCaps.Terminal = true
	}
	init, err := handle.Conn.Initialize(ctx, libacp.InitializeRequest{
		ProtocolVersion:    libacp.ProtocolVersion,
		ClientCapabilities: clientCaps,
		ClientInfo:         &libacp.Implementation{Name: "contenox", Version: version.Get()},
	})
	if err != nil {
		_ = handle.Close()
		return nil, "", nil, libacp.InternalError(fmt.Sprintf("acpsvc: initialize agent %q: %v", agentName, err))
	}
	if init.ProtocolVersion != libacp.ProtocolVersion {
		_ = handle.Close()
		return nil, "", nil, libacp.InternalError(fmt.Sprintf("acpsvc: agent %q negotiated unsupported protocol version %d", agentName, init.ProtocolVersion))
	}

	forwarded, _ := filterMcpForCaps(mcpServers, init.AgentCapabilities.McpCapabilities)
	if forwarded == nil {
		forwarded = []libacp.McpServer{}
	}
	downstream, err := handle.Conn.NewSession(ctx, libacp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: forwarded,
	})
	if err != nil {
		_ = handle.Close()
		return nil, "", nil, libacp.InternalError(fmt.Sprintf("acpsvc: session/new against agent %q: %v", agentName, err))
	}
	// Capture the downstream agent's own advertised config options AND its session
	// modes so the upstream session/new response carries them synchronously (no timing
	// gap) and externalDriver.ConfigOptions can surface the agent's real pickers plus
	// the synthetic mode select. Seeding yields to any config_option_update /
	// current_mode_update the downstream read-loop already delivered.
	bridge.seedConfigOptions(downstream.ConfigOptions)
	bridge.seedModes(downstream.Modes)
	// Persist the current live option set (the seed — synthetic mode option folded in —
	// or a concurrently-received update; snapshot returns whichever is live) so a
	// session/load before the first prompt can restore the pickers. Every (re)spawn
	// overwrites the persisted value with fresh downstream truth.
	bridge.persistConfigOptions(ctx, bridge.snapshotConfigOptions())
	return handle, downstream.SessionID, bridge, nil
}

// ensureSpawned returns the driver's live downstream connection, spawning it
// lazily on first use. session/load does not resurrect the downstream process
// (only the transcript is replayed), so the first prompt after a load lands here
// and (re)connects — the downstream agent's own internal context restarts at that
// point, a documented v1 limit.
func (d *externalDriver) ensureSpawned(ctx context.Context, upstreamID libacp.SessionID, sess *sessionEntry) (*agenthost.Handle, libacp.SessionID, *externalBridge, error) {
	d.mu.Lock()
	handle, downstreamID, bridge := d.handle, d.downstreamID, d.bridge
	d.mu.Unlock()
	if handle != nil {
		return handle, downstreamID, bridge, nil
	}

	sess.mu.Lock()
	cwd := sess.Cwd
	sess.mu.Unlock()

	// bound=true: a lazy respawn only happens on the first prompt AFTER a
	// session/load, by which point the upstream client has long since bound this
	// session id. The downstream's menu can therefore relay live — no re-emit
	// (and no duplication) is needed, unlike the eager session/new spawn.
	handle, downstreamID, bridge, err := d.t.spawnExternal(ctx, upstreamID, cwd, d.agentName, true)
	if err != nil {
		return nil, "", nil, err
	}

	d.mu.Lock()
	// A concurrent prompt may have spawned first; keep the winner, close ours.
	if d.handle != nil {
		winner, winnerID, winnerBridge := d.handle, d.downstreamID, d.bridge
		d.mu.Unlock()
		_ = handle.Close()
		return winner, winnerID, winnerBridge, nil
	}
	d.handle = handle
	d.downstreamID = downstreamID
	d.bridge = bridge
	d.mu.Unlock()
	// This is a lazy respawn (first prompt after a session/load: session/load does
	// not resurrect the downstream, so its config options came back empty). The
	// upstream session already exists and is bound, so push the freshly reconnected
	// downstream's config options as a live config_option_update to restore the
	// pickers the reloaded session lost. Empty (an agent that advertises none, e.g.
	// claude) needs no push — the reloaded toolbar is already empty.
	if opts := bridge.snapshotConfigOptions(); len(opts) > 0 {
		d.t.relayExternalUpdate(ctx, upstreamID, libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateConfigOption,
			ConfigOptions: opts,
		})
	}
	return handle, downstreamID, bridge, nil
}

// Prompt forwards a prompt to the session's downstream agent, bypassing
// slash-command interception and the native chain engine entirely: the prompt
// blocks go straight to the downstream session/prompt and its stopReason is
// returned. Upstream session/cancel is forwarded downstream as session/cancel,
// and the user prompt plus the concatenated downstream reply are persisted so
// session/list titles and session/load replay work.
func (d *externalDriver) Prompt(ctx context.Context, req libacp.PromptRequest, sess *sessionEntry) (libacp.PromptResponse, error) {
	t := d.t
	reportErr, reportChange, end := t.tracker().Start(ctx, "prompt", "acp_external_session", "session_id", string(req.SessionID))
	defer end()

	handle, downstreamID, bridge, err := d.ensureSpawned(ctx, req.SessionID, sess)
	if err != nil {
		reportErr(err)
		return libacp.PromptResponse{}, err
	}

	// Forward an upstream session/cancel to the downstream agent as session/cancel.
	// Registered so Transport.Cancel, a Close/Delete, or a connection drop invokes
	// it; the sync.Once keeps the deferred unregister from ever sending a stray
	// cancel after a normal completion.
	var cancelOnce sync.Once
	cancelDownstream := func() {
		cancelOnce.Do(func() { _ = handle.Conn.CancelPrompt(downstreamID) })
	}
	promptReg := t.registerPromptCancel(req.SessionID, cancelDownstream)
	defer t.unregisterPromptCancel(req.SessionID, promptReg)

	bridge.beginCapture()
	resp, promptErr := handle.Conn.Prompt(ctx, libacp.PromptRequest{
		SessionID: downstreamID,
		Prompt:    req.Prompt,
	})
	assistantText := bridge.finishCapture()

	userText, _ := flattenPromptBlocks(req.Prompt)
	t.persistExternalTurn(ctx, sess.InternalSessionID, userText, assistantText)

	if promptErr != nil {
		// A genuine user cancellation resolves as stopReason "cancelled" with no
		// JSON-RPC error, per the ACP contract (both the $/cancel_request path,
		// which cancels ctx, and the session/cancel path, which force-resolves the
		// downstream turn, land here or below).
		if errors.Is(promptErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			reportChange(string(req.SessionID), map[string]any{"stop_reason": string(libacp.StopReasonCancelled)})
			return libacp.PromptResponse{StopReason: libacp.StopReasonCancelled}, nil
		}
		reportErr(promptErr)
		return libacp.PromptResponse{}, libacp.InternalError(promptErr.Error())
	}

	// Push the post-turn session_info_update (freshness + derived title) exactly
	// as the native path does, so the client's sidebar label updates without a
	// re-list.
	libacp.AfterResponse(ctx, func() {
		update := libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateSessionInfo,
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if title := t.sessionInfoTitle(ctx, sess.InternalSessionID); title != "" {
			update.Title = title
		}
		t.sendUpdate(ctx, libacp.SessionNotification{SessionID: req.SessionID, Update: update})
	})

	reportChange(string(req.SessionID), map[string]any{"stop_reason": string(resp.StopReason)})
	return libacp.PromptResponse{StopReason: resp.StopReason}, nil
}

// persistExternalTurn records the user prompt and the downstream agent's reply
// into the same message store the native path uses (identity 'acp-client',
// keyed by the internal contenox session id), so session/list titles and
// session/load replay work for external sessions too. Fresh message IDs make
// PersistDiff's ID-based dedupe append them. Uses a cancellation-immune context
// so a cancelled turn still records what was said.
func (t *Transport) persistExternalTurn(ctx context.Context, internalSessionID, userText, assistantText string) {
	if t.deps.DB == nil || internalSessionID == "" {
		return
	}
	now := time.Now().UTC()
	var msgs []taskengine.Message
	if strings.TrimSpace(userText) != "" {
		msgs = append(msgs, taskengine.Message{ID: uuid.NewString(), Role: "user", Content: userText, Timestamp: now})
	}
	if strings.TrimSpace(assistantText) != "" {
		msgs = append(msgs, taskengine.Message{ID: uuid.NewString(), Role: "assistant", Content: assistantText, Timestamp: now.Add(time.Millisecond)})
	}
	if len(msgs) == 0 {
		return
	}
	cleanCtx := context.WithoutCancel(ctx)
	mgr := chatservice.NewManager(t.workspaceID())
	if err := mgr.PersistDiff(cleanCtx, t.deps.DB.WithoutTransaction(), internalSessionID, msgs); err != nil {
		reportErr, _, end := t.tracker().Start(cleanCtx, "persist", "acp_external_history", "session_id", internalSessionID)
		reportErr(err)
		end()
	}
}

// markExternalIfPersisted swaps a rebuilt session entry (session/load or
// session/resume) onto an external driver when a persisted agent name exists, so
// its config options come back minimal and the next prompt routes to the
// downstream agent (lazily respawned) instead of the native chain. This — with
// NewSession's `_meta` check — is the sole place the native-vs-external driver is
// chosen.
func (t *Transport) markExternalIfPersisted(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID, entry *sessionEntry) {
	name := t.readSessionAgent(ctx, store, sid)
	if name == "" {
		return
	}
	entry.driver = &externalDriver{t: t, agentName: name, upstreamID: sid}
	// Restore the persisted per-session HITL policy so the reopened toolbar's picker
	// shows the previously-chosen value (the entry was rebuilt with the sentinel
	// default) and resolveSessionHITLPolicy enforces it — the native chain keeps this
	// selection only in-memory, so unlike a native session an external one persists it.
	if policy := t.readSessionHITLPolicy(ctx, store, sid); policy != "" {
		entry.setHITLPolicy(policy)
	}
}

// reloadedConfigOptions returns the config options to advertise on a session/load or
// session/resume response. A native session dispatches to its driver (the chain-engine
// selects). An external session's downstream is NOT respawned during load, so its
// downstream surface (synthetic mode select + the agent's own pickers) comes from the
// set persisted at session/new (and each later update/set), and contenox's own HITL
// policy select rides AFTER it — its CurrentValue restored from the persisted
// per-session selection (see markExternalIfPersisted, which runs before this) so the
// reopened toolbar's picker survives the reload without waiting for a respawn.
func (t *Transport) reloadedConfigOptions(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID, entry *sessionEntry) []libacp.SessionConfigOption {
	if _, ok := entry.driver.(*externalDriver); ok {
		downstream := t.readSessionAgentConfigOptions(ctx, store, sid)
		return append(downstream, t.hitlPolicyConfigOption(entry))
	}
	return t.sessionConfigOptions(ctx, entry)
}

// reemitExternalCommandMenu schedules the persisted downstream slash-command menu to
// be relayed to the upstream client strictly AFTER the load/resume result is on the
// wire (via libacp.AfterResponse — the same ordering contract sendAvailableCommands and
// the session/new re-emit follow, since a client drops updates for a session id it has
// not yet learned), so a reopened external session shows its menu without a first prompt
// to respawn the downstream. A no-op when the session persisted no menu.
func (t *Transport) reemitExternalCommandMenu(ctx context.Context, store runtimetypes.Store, sid libacp.SessionID) {
	cmds := t.readSessionAgentCommands(ctx, store, sid)
	if len(cmds) == 0 {
		return
	}
	libacp.AfterResponse(ctx, func() {
		t.relayExternalUpdate(ctx, sid, libacp.SessionUpdate{
			SessionUpdate:     libacp.SessionUpdateAvailableCommands,
			AvailableCommands: cmds,
		})
	})
}
