package agentinstance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/agenthost"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
)

// defaultKillGrace bounds how long an external instance's teardown waits for its
// subprocess to exit on stdin-close before killing it. Persistent ACP agents
// (testy, claude-code-acp) never exit on stdin-close, so a short grace keeps
// Stop/Close from stalling the full acpexec default per instance.
const defaultKillGrace = 2 * time.Second

// ErrNotFound is returned for an unknown instance id. It is a sentinel so callers
// can branch on errors.Is(err, ErrNotFound).
var ErrNotFound = errors.New("agentinstance: instance not found")

// EventKind classifies a lifecycle Event.
type EventKind string

const (
	// EventStateChange fires on every instance state transition (Event.State
	// carries the new state).
	EventStateChange EventKind = "state_change"
	// EventAttach fires when a viewer attaches to a session (Event.Controller
	// reports whether it became the controller).
	EventAttach EventKind = "attach"
	// EventDetach fires when a viewer detaches from a session.
	EventDetach EventKind = "detach"
)

// Event is one instance-lifecycle event. It is SELF-CONTAINED — every field a
// consumer needs is on the Event, so a sink reacts to it without calling back
// into the Manager (and thus cannot deadlock or race registration). This is the
// substrate a future scheduler (cron/bus → Start) and beam's live fleet view
// both hang off: the Manager fires an Event on every state change / attach /
// detach, and those consumers subscribe via WithEventSink.
type Event struct {
	Kind       EventKind        `json:"kind"`
	InstanceID string           `json:"instanceId"`
	AgentID    string           `json:"agentId"`
	AgentName  string           `json:"agentName"`
	State      string           `json:"state,omitempty"`      // EventStateChange
	SessionID  libacp.SessionID `json:"sessionId,omitempty"`  // EventAttach / EventDetach
	ViewerID   string           `json:"viewerId,omitempty"`   // EventAttach / EventDetach
	Controller bool             `json:"controller,omitempty"` // EventAttach
	Time       time.Time        `json:"time"`
}

// EventSink receives every lifecycle Event the Manager fires. It is called
// synchronously on the goroutine that produced the event (a state transition, an
// attach, a detach), so it MUST NOT block or call back into the Manager; enqueue
// and return. A future scheduler/fleet-view adapts its own subscription on top of
// this one seam.
type EventSink func(Event)

// FleetEntry is one row of the config+runtime join List returns: a DECLARED
// agent, annotated with its live instances (empty when the agent is declared but
// not running). It is the fleet surface's substrate — it shows declared-but-idle
// agents, not only live ones.
type FleetEntry struct {
	AgentID   string           `json:"agentId"`
	AgentName string           `json:"agentName"`
	Kind      string           `json:"kind"`
	Instances []InstanceStatus `json:"instances"`
}

// Running reports whether this declared agent has at least one live instance.
func (e FleetEntry) Running() bool { return len(e.Instances) > 0 }

// Manager owns the lifecycle of running agent instances. Every method is safe for
// concurrent use.
type Manager interface {
	// Start resolves the declared agent named agentName via the registry service
	// and brings up an instance for it. For an external_acp agent it spawns the
	// subprocess (agenthost) bound to the MANAGER's root context — not ctx — so
	// the instance outlives the request that started it; ctx governs only the
	// registry lookup. The instance is wired to its OWN internal journaling
	// harness (callers ATTACH as viewers, they no longer supply the harness). For
	// a chain (native) agent it creates a process-less instance. Returns the
	// generated instance id, or an error if the agent cannot be resolved or
	// spawned (in which case nothing is registered and no process is leaked).
	Start(ctx context.Context, agentName string) (instanceID string, err error)

	// Attach registers viewer against (instanceID, sessionID): it replays that
	// session's journal to the viewer, then joins it to the live fan-out. The
	// first viewer of a session with no controller becomes the controller
	// (controllerGranted true) and thereby answers the downstream agent's
	// permission requests; later viewers are observers (false). Returns
	// ErrNotFound for an unknown instance, or an error if the viewer id is empty
	// or already attached to that session.
	Attach(ctx context.Context, instanceID string, sessionID libacp.SessionID, viewer Viewer) (controllerGranted bool, err error)

	// Detach removes viewerID from (instanceID, sessionID)'s fan-out. If it was
	// the controller and viewers remain, the earliest-attached survivor is
	// promoted. Returns ErrNotFound for an unknown instance, or an error if the
	// viewer/session is not attached.
	Detach(instanceID string, sessionID libacp.SessionID, viewerID string) error

	// List returns the config+runtime join: every declared agent, annotated with
	// its live instances (empty = not running). ctx governs the registry read (it
	// is DB-backed, unlike the purely in-memory live side), so unlike the terse
	// primitive it takes a context and can error.
	List(ctx context.Context) ([]FleetEntry, error)

	// Get returns the status of one instance, or ErrNotFound if instanceID is
	// unknown (including one already Stopped).
	Get(instanceID string) (InstanceStatus, error)

	// OpenSession drives the downstream ACP handshake on instanceID's connection: it
	// initializes the connection once (negotiating capabilities per spec — see
	// SessionSpec.Terminal for the terminal-capability advertisement rule), then drives
	// session/new with spec, capturing the response's config options / modes / models into
	// per-session kernel state. It returns the DOWNSTREAM session id — the id the instance
	// journals and fans out under, and the id a viewer Attaches to. This is the outbound
	// counterpart of Attach: viewers OBSERVE a session's stream and answer its permissions
	// via Attach; a consumer DRIVES the downstream via these methods, holding no connection.
	// Returns ErrNotFound for an unknown instance, or an error for a native/no-connection
	// instance or a failed handshake.
	OpenSession(ctx context.Context, instanceID string, spec SessionSpec) (libacp.SessionID, error)

	// Prompt drives one downstream session/prompt turn for sessionID and returns its stop
	// reason. Every downstream session/update during the turn is journaled and fanned out to
	// the session's viewers by the instance's harness. It is cancellation-aware: a ctx
	// cancellation (or a concurrent Cancel) resolves cleanly as StopReasonCancelled with a nil
	// error. Returns ErrNotFound for an unknown instance.
	Prompt(ctx context.Context, instanceID string, sessionID libacp.SessionID, prompt []libacp.ContentBlock) (libacp.StopReason, error)

	// Cancel cancels sessionID's in-flight prompt turn on the downstream (session/cancel plus
	// the prompt-turn permission auto-resolve). Safe with no turn in flight. Returns
	// ErrNotFound for an unknown instance.
	Cancel(instanceID string, sessionID libacp.SessionID) error

	// CloseSession ends sessionID: it best-effort cancels any in-flight downstream turn, then
	// drops the session's kernel state (its captured surface and its journal + viewers). It
	// does NOT stop the instance — an instance multiplexes many sessions over one connection.
	// Returns ErrNotFound for an unknown instance.
	CloseSession(instanceID string, sessionID libacp.SessionID) error

	// SetConfigOption forwards an upstream config-option change to the downstream and adopts
	// the confirmed value into kernel state. The synthetic mode/model option ids map to
	// session/set_mode and session/set_model; every other id forwards to
	// session/set_config_option. Returns ErrNotFound for an unknown instance.
	SetConfigOption(ctx context.Context, instanceID string, sessionID libacp.SessionID, configID string, value libacp.SessionConfigOptionValue) error

	// SessionConfigOptions returns sessionID's captured config-option surface (synthetic mode
	// select + synthetic model select + the downstream agent's own options), for a transport
	// to advertise upstream. Nil for an unknown session; ErrNotFound for an unknown instance.
	SessionConfigOptions(instanceID string, sessionID libacp.SessionID) ([]libacp.SessionConfigOption, error)

	// AvailableCommands returns sessionID's captured downstream slash-command menu. Nil for an
	// unknown session; ErrNotFound for an unknown instance.
	AvailableCommands(instanceID string, sessionID libacp.SessionID) ([]libacp.AvailableCommand, error)

	// Stop tears an instance down and removes it from the registry. It sets the
	// instance's manualStop flag so the watchDog never restarts it. Idempotent:
	// stopping an unknown or already-stopped id is a no-op returning nil.
	Stop(instanceID string) error

	// Close stops every instance (tearing down all subprocesses) and cancels the
	// Manager's root context. It is the runtime-shutdown hook; after Close, Start
	// returns an error. Idempotent.
	Close() error
}

// Option configures a Manager.
type Option func(*manager)

// WithStderr forwards each spawned external instance's subprocess stderr to w.
// Defaults to io.Discard.
func WithStderr(w io.Writer) Option { return func(m *manager) { m.stderr = w } }

// WithKillGrace overrides how long an external instance's teardown waits for its
// subprocess to exit on stdin-close before killing it (see defaultKillGrace).
func WithKillGrace(d time.Duration) Option { return func(m *manager) { m.killGrace = d } }

// WithJournalSize overrides the per-session replay journal size (see
// defaultJournalSize).
func WithJournalSize(n int) Option { return func(m *manager) { m.journalSize = n } }

// WithRestart enables the watchDog restart policy: an external instance whose
// subprocess dies unexpectedly is re-spawned up to limit times before the
// instance is parked in StateWarning. Default: disabled (an unexpected death is
// terminal StateError). NOTE the ACP caveat — a restart re-spawns a fresh
// subprocess that LOSES the downstream agent's conversation context; it keeps the
// fleet alive, not the conversation.
func WithRestart(limit int) Option {
	return func(m *manager) {
		m.restartEnabled = true
		m.restartLimit = limit
	}
}

// WithEventSink installs sink as the lifecycle event sink (see EventSink).
func WithEventSink(sink EventSink) Option { return func(m *manager) { m.sink = sink } }

type manager struct {
	agents      agentregistryservice.Service
	stderr      io.Writer
	killGrace   time.Duration
	journalSize int

	restartEnabled bool
	restartLimit   int
	sink           EventSink

	// rootCtx is the long-lived context every external instance's subprocess is
	// bound to, so instances outlive the caller ctx passed to Start. Close cancels
	// it, relocating ownership off any client connection onto the runtime.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	mu        sync.Mutex
	instances map[string]*instance
	closed    bool
}

// New returns a Manager that resolves declared agents via agents. The Manager
// owns a fresh root context; call Close to tear everything down at shutdown.
func New(agents agentregistryservice.Service, opts ...Option) Manager {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	m := &manager{
		agents:      agents,
		stderr:      io.Discard,
		killGrace:   defaultKillGrace,
		journalSize: defaultJournalSize,
		rootCtx:     rootCtx,
		rootCancel:  rootCancel,
		instances:   make(map[string]*instance),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

var _ Manager = (*manager)(nil)

func (m *manager) emit(ev Event) {
	if m.sink != nil {
		m.sink(ev)
	}
}

func (m *manager) Start(ctx context.Context, agentName string) (string, error) {
	if agentName == "" {
		return "", fmt.Errorf("agentinstance: agentName is required")
	}
	// Resolve the declared agent. ctx governs only this lookup — deliberately NOT
	// the instance that follows.
	agent, err := m.agents.GetByName(ctx, agentName)
	if err != nil {
		return "", fmt.Errorf("agentinstance: resolve agent %q: %w", agentName, err)
	}

	switch agent.Kind {
	case runtimetypes.AgentKindExternalACP:
		cfg, err := agent.ExternalACPConfig()
		if err != nil {
			return "", fmt.Errorf("agentinstance: agent %q: %w", agent.Name, err)
		}
		spawner := &agenthost.ExternalACPAgent{
			Config:    *cfg,
			Stderr:    m.stderr,
			KillGrace: m.killGrace,
		}
		return m.bringUp(agent, spawner)
	case runtimetypes.AgentKindChain:
		return m.bringUp(agent, nil)
	default:
		return "", fmt.Errorf("agentinstance: agent %q has unsupported kind %q", agentName, agent.Kind)
	}
}

// bringUp builds and starts an instance for agent (spawner nil => native), then
// registers it. start() spawns OUTSIDE the registry lock, so a slow subprocess
// startup never blocks List/Get/Stop of other instances; only on success is the
// instance registered. A spawn failure registers nothing and leaks no process.
func (m *manager) bringUp(agent *runtimetypes.Agent, spawner agenthost.Agent) (string, error) {
	id := uuid.NewString()
	inst := newInstance(instanceConfig{
		id:             id,
		agentID:        agent.ID,
		agentName:      agent.Name,
		kind:           agent.Kind,
		rootCtx:        m.rootCtx,
		spawner:        spawner,
		journalSize:    m.journalSize,
		restartEnabled: m.restartEnabled,
		restartLimit:   m.restartLimit,
		onState: func(state string) {
			m.emit(Event{Kind: EventStateChange, InstanceID: id, AgentID: agent.ID, AgentName: agent.Name, State: state, Time: time.Now().UTC()})
		},
		onAttach: func(sessionID libacp.SessionID, viewerID string, controller bool) {
			m.emit(Event{Kind: EventAttach, InstanceID: id, AgentID: agent.ID, AgentName: agent.Name, SessionID: sessionID, ViewerID: viewerID, Controller: controller, Time: time.Now().UTC()})
		},
		onDetach: func(sessionID libacp.SessionID, viewerID string) {
			m.emit(Event{Kind: EventDetach, InstanceID: id, AgentID: agent.ID, AgentName: agent.Name, SessionID: sessionID, ViewerID: viewerID, Time: time.Now().UTC()})
		},
	})

	if err := inst.start(); err != nil {
		return "", fmt.Errorf("agentinstance: start agent %q: %w", agent.Name, err)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = inst.stop()
		return "", fmt.Errorf("agentinstance: manager is closed")
	}
	m.instances[id] = inst
	m.mu.Unlock()
	return id, nil
}

func (m *manager) Attach(ctx context.Context, instanceID string, sessionID libacp.SessionID, viewer Viewer) (bool, error) {
	m.mu.Lock()
	inst, ok := m.instances[instanceID]
	m.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("agentinstance: %q: %w", instanceID, ErrNotFound)
	}
	return inst.attach(ctx, sessionID, viewer)
}

func (m *manager) Detach(instanceID string, sessionID libacp.SessionID, viewerID string) error {
	m.mu.Lock()
	inst, ok := m.instances[instanceID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("agentinstance: %q: %w", instanceID, ErrNotFound)
	}
	return inst.detach(sessionID, viewerID)
}

func (m *manager) Get(instanceID string) (InstanceStatus, error) {
	m.mu.Lock()
	inst, ok := m.instances[instanceID]
	m.mu.Unlock()
	if !ok {
		return InstanceStatus{}, fmt.Errorf("agentinstance: %q: %w", instanceID, ErrNotFound)
	}
	return inst.status(), nil
}

// instance resolves instanceID to its live instance, or ErrNotFound. It is the shared
// lookup the session-driving methods use before delegating to the instance primitive.
func (m *manager) instance(instanceID string) (*instance, error) {
	m.mu.Lock()
	inst, ok := m.instances[instanceID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("agentinstance: %q: %w", instanceID, ErrNotFound)
	}
	return inst, nil
}

func (m *manager) OpenSession(ctx context.Context, instanceID string, spec SessionSpec) (libacp.SessionID, error) {
	inst, err := m.instance(instanceID)
	if err != nil {
		return "", err
	}
	return inst.openSession(ctx, spec)
}

func (m *manager) Prompt(ctx context.Context, instanceID string, sessionID libacp.SessionID, prompt []libacp.ContentBlock) (libacp.StopReason, error) {
	inst, err := m.instance(instanceID)
	if err != nil {
		return "", err
	}
	return inst.promptSession(ctx, sessionID, prompt)
}

func (m *manager) Cancel(instanceID string, sessionID libacp.SessionID) error {
	inst, err := m.instance(instanceID)
	if err != nil {
		return err
	}
	return inst.cancelSession(sessionID)
}

func (m *manager) CloseSession(instanceID string, sessionID libacp.SessionID) error {
	inst, err := m.instance(instanceID)
	if err != nil {
		return err
	}
	return inst.closeSession(sessionID)
}

func (m *manager) SetConfigOption(ctx context.Context, instanceID string, sessionID libacp.SessionID, configID string, value libacp.SessionConfigOptionValue) error {
	inst, err := m.instance(instanceID)
	if err != nil {
		return err
	}
	return inst.setConfigOption(ctx, sessionID, configID, value)
}

func (m *manager) SessionConfigOptions(instanceID string, sessionID libacp.SessionID) ([]libacp.SessionConfigOption, error) {
	inst, err := m.instance(instanceID)
	if err != nil {
		return nil, err
	}
	return inst.sessionConfigOptions(sessionID), nil
}

func (m *manager) AvailableCommands(instanceID string, sessionID libacp.SessionID) ([]libacp.AvailableCommand, error) {
	inst, err := m.instance(instanceID)
	if err != nil {
		return nil, err
	}
	return inst.availableCommands(sessionID), nil
}

func (m *manager) List(ctx context.Context) ([]FleetEntry, error) {
	// Snapshot the live side, grouped by declared-agent id.
	m.mu.Lock()
	live := make([]*instance, 0, len(m.instances))
	for _, inst := range m.instances {
		live = append(live, inst)
	}
	m.mu.Unlock()

	byAgent := make(map[string][]InstanceStatus)
	for _, inst := range live {
		st := inst.status()
		byAgent[st.AgentID] = append(byAgent[st.AgentID], st)
	}

	// Join with the declared (config) side.
	declared, err := m.listDeclared(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentinstance: list declared agents: %w", err)
	}

	entries := make([]FleetEntry, 0, len(declared))
	seen := make(map[string]bool, len(declared))
	for _, a := range declared {
		seen[a.ID] = true
		entries = append(entries, FleetEntry{
			AgentID:   a.ID,
			AgentName: a.Name,
			Kind:      a.Kind,
			Instances: byAgent[a.ID], // nil (not running) or the live set
		})
	}
	// Orphan instances (agent deleted after start) still surface, so the fleet
	// view never silently hides a running subprocess.
	for agentID, insts := range byAgent {
		if seen[agentID] {
			continue
		}
		entries = append(entries, FleetEntry{
			AgentID:   agentID,
			AgentName: insts[0].AgentName,
			Kind:      insts[0].Kind,
			Instances: insts,
		})
	}
	return entries, nil
}

// listDeclared pages through every declared agent via the registry service. The
// store filters created_at < cursor (DESC), so each page's oldest row seeds the
// next cursor. The strictly-decreasing-cursor guard defends against an
// identical-timestamp storm looping forever (at the cost of truncating such a
// tie), a pre-existing store pagination limit, not one this join introduces.
func (m *manager) listDeclared(ctx context.Context) ([]*runtimetypes.Agent, error) {
	const page = 200
	var all []*runtimetypes.Agent
	var cursor *time.Time
	for {
		batch, err := m.agents.List(ctx, cursor, page)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < page {
			break
		}
		last := batch[len(batch)-1].CreatedAt
		if cursor != nil && !last.Before(*cursor) {
			break
		}
		cursor = &last
	}
	return all, nil
}

func (m *manager) Stop(instanceID string) error {
	m.mu.Lock()
	inst, ok := m.instances[instanceID]
	if ok {
		delete(m.instances, instanceID)
	}
	m.mu.Unlock()
	if !ok {
		return nil // idempotent
	}
	return inst.stop()
}

func (m *manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	insts := make([]*instance, 0, len(m.instances))
	for id, inst := range m.instances {
		insts = append(insts, inst)
		delete(m.instances, id)
	}
	m.mu.Unlock()

	// Stop every instance, then cancel the root context as a backstop. Aggregate
	// teardown errors rather than stopping at the first, so one wedged subprocess
	// cannot hide the rest.
	var errs []error
	for _, inst := range insts {
		if err := inst.stop(); err != nil {
			errs = append(errs, err)
		}
	}
	m.rootCancel()
	return errors.Join(errs...)
}
