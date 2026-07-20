package fleetservice

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

// ─── fakeManager: a hand-rolled agentinstance.Manager double ───────────────
//
// It records every call fleetservice.Dispatch/Stop/Cancel makes so a test can
// assert the orchestration (teardown-on-failure, cancel fan-out) without
// spawning a real subprocess — the agentinstance package's own tests already
// cover the kernel's real behavior; this package tests the POLICY wrapped
// around it.

type cancelCall struct {
	instanceID string
	sessionID  libacp.SessionID
}

type fakeManager struct {
	mu sync.Mutex

	startID       string
	startErr      error
	startCalls    []string
	startedAgents []*runtimetypes.Agent

	openID    libacp.SessionID
	openErr   error
	openSpecs []agentinstance.SessionSpec

	promptErr      error
	promptStarted  chan struct{}
	promptReleased chan struct{}
	promptDone     chan struct{}
	promptBlocks   []libacp.ContentBlock

	stopCalls []string

	cancelErr   error
	cancelCalls []cancelCall

	statuses map[string]agentinstance.InstanceStatus
}

func (m *fakeManager) Start(_ context.Context, agentName string) (string, error) {
	m.mu.Lock()
	m.startCalls = append(m.startCalls, agentName)
	m.mu.Unlock()
	if m.startErr != nil {
		return "", m.startErr
	}
	return m.startID, nil
}

// StartResolved records the spawn under the resolved record's NAME, so every
// existing starts() assertion keeps reading the same way. Dispatch calls this one
// — it already resolved the agent to make the Enabled decision, and re-resolving
// by name in the kernel would reopen the TOCTOU window that check exists to close.
func (m *fakeManager) StartResolved(_ context.Context, agent *runtimetypes.Agent) (string, error) {
	m.mu.Lock()
	name := ""
	if agent != nil {
		name = agent.Name
	}
	m.startCalls = append(m.startCalls, name)
	m.startedAgents = append(m.startedAgents, agent)
	m.mu.Unlock()
	if m.startErr != nil {
		return "", m.startErr
	}
	return m.startID, nil
}

func (m *fakeManager) OpenSession(_ context.Context, _ string, spec agentinstance.SessionSpec) (libacp.SessionID, error) {
	m.mu.Lock()
	m.openSpecs = append(m.openSpecs, spec)
	m.mu.Unlock()
	if m.openErr != nil {
		return "", m.openErr
	}
	return m.openID, nil
}

func (m *fakeManager) Prompt(_ context.Context, _ string, _ libacp.SessionID, blocks []libacp.ContentBlock) (libacp.StopReason, error) {
	m.mu.Lock()
	m.promptBlocks = blocks
	m.mu.Unlock()
	if m.promptStarted != nil {
		close(m.promptStarted)
	}
	if m.promptReleased != nil {
		<-m.promptReleased
	}
	if m.promptDone != nil {
		close(m.promptDone)
	}
	return libacp.StopReasonEndTurn, m.promptErr
}

func (m *fakeManager) Stop(instanceID string) error {
	m.mu.Lock()
	m.stopCalls = append(m.stopCalls, instanceID)
	m.mu.Unlock()
	return nil
}

func (m *fakeManager) Cancel(instanceID string, sessionID libacp.SessionID) error {
	m.mu.Lock()
	m.cancelCalls = append(m.cancelCalls, cancelCall{instanceID: instanceID, sessionID: sessionID})
	m.mu.Unlock()
	return m.cancelErr
}

func (m *fakeManager) Get(instanceID string) (agentinstance.InstanceStatus, error) {
	st, ok := m.statuses[instanceID]
	if !ok {
		return agentinstance.InstanceStatus{}, fmt404(instanceID)
	}
	return st, nil
}

func fmt404(instanceID string) error {
	return &notFoundErr{instanceID: instanceID}
}

type notFoundErr struct{ instanceID string }

func (e *notFoundErr) Error() string { return "agentinstance: " + e.instanceID + ": not found" }
func (e *notFoundErr) Unwrap() error { return agentinstance.ErrNotFound }

// The remaining Manager methods are unused by fleetservice; no-op them to
// satisfy the interface.
func (m *fakeManager) Attach(context.Context, string, libacp.SessionID, agentinstance.Viewer) (bool, error) {
	return false, nil
}
func (m *fakeManager) Detach(string, libacp.SessionID, string) error { return nil }
func (m *fakeManager) List(context.Context) ([]agentinstance.FleetEntry, error) {
	return nil, nil
}
func (m *fakeManager) CloseSession(string, libacp.SessionID) error { return nil }
func (m *fakeManager) SetConfigOption(context.Context, string, libacp.SessionID, string, libacp.SessionConfigOptionValue) error {
	return nil
}
func (m *fakeManager) SessionConfigOptions(string, libacp.SessionID) ([]libacp.SessionConfigOption, error) {
	return nil, nil
}
func (m *fakeManager) AvailableCommands(string, libacp.SessionID) ([]libacp.AvailableCommand, error) {
	return nil, nil
}
func (m *fakeManager) Close() error { return nil }

func (m *fakeManager) starts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.startCalls...)
}

func (m *fakeManager) stops() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.stopCalls...)
}

func (m *fakeManager) cancels() []cancelCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]cancelCall(nil), m.cancelCalls...)
}

var _ agentinstance.Manager = (*fakeManager)(nil)

// ─── setup helpers ──────────────────────────────────────────────────────────

// setupRegistryDB gives a test a real sqlite-backed agentregistryservice /
// missionservice pair — both are validated-CRUD registries this package
// depends on for real, so exercising them for real is cheap (sqlite, no
// subprocess) and catches drift the store's own validate() would reject.
func setupRegistryDB(t *testing.T) (context.Context, libdb.DBManager) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "fleetservice.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

// registerAgent declares an external_acp agent named name with the given
// enabled flag, via the real registry service.
func registerAgent(t *testing.T, ctx context.Context, agents agentregistryservice.Service, name string, enabled bool) {
	t.Helper()
	agent := &runtimetypes.Agent{Name: name, Enabled: enabled}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "/bin/true",
	}))
	require.NoError(t, agents.Create(ctx, agent))
}

// countingRegistry wraps a real agentregistryservice.Service and counts
// GetByName calls — the read every spawn path used to make TWICE (once for the
// Enabled decision, once inside the kernel's Start).
type countingRegistry struct {
	agentregistryservice.Service
	mu     sync.Mutex
	byName int
}

func (r *countingRegistry) GetByName(ctx context.Context, name string) (*runtimetypes.Agent, error) {
	r.mu.Lock()
	r.byName++
	r.mu.Unlock()
	return r.Service.GetByName(ctx, name)
}

func (r *countingRegistry) reads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byName
}

// ─── Dispatch: Enabled policy ───────────────────────────────────────────────

func TestFleetService_Dispatch_DisabledAgentRefused(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", false)

	man := &fakeManager{startID: "inst-1", openID: "sess-1"}
	svc := New(man, agents, nil, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default"})
	require.Error(t, err)
	require.ErrorIs(t, err, apiframework.ErrConflict, "a disabled agent is refused as a 4xx conflict")
	require.Contains(t, err.Error(), "disabled")
	require.Empty(t, man.starts(), "a refused dispatch must never bring an instance up")
}

func TestFleetService_Dispatch_UnknownAgentPropagatesNotFound(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	// No agent registered.

	man := &fakeManager{startID: "inst-1", openID: "sess-1"}
	svc := New(man, agents, nil, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "ghost", Intent: "do the thing", HITLPolicyName: "default"})
	require.Error(t, err)
	require.ErrorIs(t, err, libdb.ErrNotFound)
	require.Empty(t, man.starts())
}

// ─── Dispatch: happy path ───────────────────────────────────────────────────

// TestFleetService_Dispatch_HappyPath drives the full flow: an instance comes
// up, a session opens, the mission is created carrying the request's envelope
// and bound to both fresh ids, and the intent runs as the unit's first turn.
// Every dispatch is a mission now, so there is no "with mission" qualifier
// left to distinguish this from any other successful dispatch.
func TestFleetService_Dispatch_HappyPath(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)
	missions := missionservice.New(db)

	man := &fakeManager{
		startID:        "inst-7",
		openID:         "sess-7",
		promptStarted:  make(chan struct{}),
		promptReleased: make(chan struct{}),
		promptDone:     make(chan struct{}),
	}
	close(man.promptReleased) // let the async prompt run to completion freely

	svc := New(man, agents, missions, nil, "/project/root", libtracker.NoopTracker{})

	result, err := svc.Dispatch(ctx, DispatchRequest{
		AgentName:      "runner",
		Intent:         "ship the board",
		HITLPolicyName: "default",
	})
	require.NoError(t, err)
	require.Equal(t, "inst-7", result.InstanceID)
	require.Equal(t, "sess-7", result.SessionID)
	require.NotEmpty(t, result.MissionID)
	require.Equal(t, []string{"runner"}, man.starts())

	// cwd defaulted to the project root (no allowlist configured).
	require.Len(t, man.openSpecs, 1)
	require.Equal(t, "/project/root", man.openSpecs[0].Cwd)

	m, err := missions.Get(ctx, result.MissionID)
	require.NoError(t, err)
	require.Equal(t, "ship the board", m.Intent)
	require.Equal(t, "runner", m.AgentName)
	require.Equal(t, "default", m.HITLPolicyName, "the mission carries the request's envelope")
	require.Equal(t, "sess-7", m.SessionID, "bound to the session Dispatch just opened")
	require.Equal(t, "inst-7", m.InstanceID, "bound to the instance Dispatch just started")

	select {
	case <-man.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("async prompt never ran")
	}
	require.Empty(t, man.stops(), "a successful dispatch must never stop the instance it just brought up")

	// The intent IS the prompt: the first turn's content is the request's
	// Intent, not some separate field.
	text, _ := libacp.FlattenContent(man.promptBlocks)
	require.Equal(t, "ship the board", text, "the intent runs as the unit's first turn")
}

// TestFleetService_Dispatch_ResolvesTheAgentExactlyOnce is the service half of
// closing the spawn-path TOCTOU. Dispatch resolves the declared agent in order to
// make the Enabled decision; it must then hand THAT record to the kernel rather
// than a name for the kernel to re-resolve. Two reads meant the check was made
// against the first and the spawn proceeded from the second, so an agent disabled
// in between still spawned — a hole in the exact check this path exists to enforce.
// (The kernel half — StartResolved performing no read of its own — is pinned in
// agentinstance.)
func TestFleetService_Dispatch_ResolvesTheAgentExactlyOnce(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)
	counting := &countingRegistry{Service: agents}

	man := &fakeManager{startID: "inst-once", openID: "sess-once"}
	svc := New(man, counting, missionservice.New(db), nil, "/project/root", nil)

	result, err := svc.Dispatch(ctx, DispatchRequest{
		AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default",
	})
	require.NoError(t, err)
	require.Equal(t, "inst-once", result.InstanceID)
	require.Equal(t, 1, counting.reads(), "one dispatch, one registry read")

	require.Len(t, man.startedAgents, 1)
	spawned := man.startedAgents[0]
	require.NotNil(t, spawned, "the kernel is handed the record, not a name to re-resolve")
	require.Equal(t, "runner", spawned.Name)
	require.True(t, spawned.Enabled, "the bytes that were judged are the bytes that are spawned")
}

func TestFleetService_Dispatch_MissingAgentNameRejected(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)

	svc := New(&fakeManager{}, agents, nil, nil, "/project/root", nil)
	_, err := svc.Dispatch(ctx, DispatchRequest{})
	require.Error(t, err)
	require.ErrorIs(t, err, apiframework.ErrMissingParameter)
}

// TestFleetService_Dispatch_IntentRequiredRejected proves the intent — the
// content of the unit's first turn — cannot be empty even when every other
// field is valid.
func TestFleetService_Dispatch_IntentRequiredRejected(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-4", openID: "sess-4"}
	svc := New(man, agents, nil, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "runner", HITLPolicyName: "default"})
	require.Error(t, err)
	require.ErrorIs(t, err, apiframework.ErrMissingParameter)
	require.Empty(t, man.starts(), "rejected before any instance is brought up")
}

// TestFleetService_Dispatch_EnvelopeRequiredRejected proves the HITL policy
// name — the mission's envelope — cannot be empty: a mission with no bounds
// is exactly what mission mode must not permit.
func TestFleetService_Dispatch_EnvelopeRequiredRejected(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-4", openID: "sess-4"}
	svc := New(man, agents, nil, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "runner", Intent: "do the thing"})
	require.Error(t, err)
	require.ErrorIs(t, err, apiframework.ErrMissingParameter)
	require.Empty(t, man.starts(), "rejected before any instance is brought up")
}

// ─── Dispatch: teardown-on-failure ──────────────────────────────────────────

func TestFleetService_Dispatch_TeardownOnOpenSessionFailure(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-9", openErr: context.DeadlineExceeded}
	svc := New(man, agents, nil, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default"})
	require.Error(t, err)
	require.Equal(t, []string{"inst-9"}, man.stops(), "a failed OpenSession must tear the fresh instance back down")
}

func TestFleetService_Dispatch_TeardownOnMissionBindFailure(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-11", openID: "sess-11"}
	// A mission registry that Binds against a nonexistent mission id fails
	// with libdb.ErrNotFound, forcing the same rollback OpenSession failure
	// takes.
	missions := missionservice.New(db)
	svc := New(man, agents, &bindFailingMissions{Service: missions}, nil, "/project/root", nil)

	_, err := svc.Dispatch(ctx, DispatchRequest{AgentName: "runner", Intent: "will fail to bind", HITLPolicyName: "default"})
	require.Error(t, err)
	require.Equal(t, []string{"inst-11"}, man.stops(), "a failed mission Bind must tear the fresh instance back down")
}

// bindFailingMissions wraps a real missionservice.Service but makes Bind
// always fail, to exercise Dispatch's rollback path without hand-rolling the
// rest of the Service interface.
type bindFailingMissions struct {
	missionservice.Service
}

func (b *bindFailingMissions) Bind(context.Context, string, string, string) (*missionservice.Mission, error) {
	return nil, context.DeadlineExceeded
}

// ─── Dispatch: cwd envelope ─────────────────────────────────────────────────

func TestFleetService_Dispatch_InvalidCwdRejected(t *testing.T) {
	allowed := t.TempDir()
	roots, err := vfs.NewFactory(allowed)
	require.NoError(t, err)

	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-3", openID: "sess-3"}
	svc := New(man, agents, nil, roots, "", nil)

	_, err = svc.Dispatch(ctx, DispatchRequest{
		AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default", Cwd: t.TempDir(),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, apiframework.ErrInvalidParameterValue)
	require.Empty(t, man.starts(), "rejected before any instance is brought up")
}

// TestFleetService_Dispatch_RelativeCwdRejectedWithoutAllowlist pins the
// TIGHTENING that consolidating cwd resolution onto vfs.ResolveSessionCwd
// brought: every ACP entry point already refused a non-absolute cwd before
// resolving, but this REST path did not. With no allowlist configured its
// hand-rolled predecessor returned a non-empty cwd UNCHANGED, so
// POST /fleet/dispatch {"cwd":"../.."} reached OpenSession with a relative path
// the session paths would have refused. It is now refused here too, before any
// instance is brought up.
func TestFleetService_Dispatch_RelativeCwdRejectedWithoutAllowlist(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)
	registerAgent(t, ctx, agents, "runner", true)

	man := &fakeManager{startID: "inst-rel", openID: "sess-rel"}
	// nil allowlist — the configuration the hole lived in.
	svc := New(man, agents, nil, nil, "/project/root", nil)

	for _, cwd := range []string{"../..", "relative/path", "."} {
		_, err := svc.Dispatch(ctx, DispatchRequest{
			AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default", Cwd: cwd,
		})
		require.Error(t, err, "cwd %q must be refused", cwd)
		require.ErrorIs(t, err, apiframework.ErrInvalidParameterValue)
	}
	require.Empty(t, man.starts(), "rejected before any instance is brought up")
	require.Empty(t, man.openSpecs, "no session is ever opened with a relative cwd")
}

// ─── Stop ────────────────────────────────────────────────────────────────────

func TestFleetService_Stop_DelegatesAndIsIdempotent(t *testing.T) {
	man := &fakeManager{}
	svc := New(man, agentregistryservice.New(mustDB(t)), nil, nil, "", nil)

	require.NoError(t, svc.Stop(context.Background(), "inst-1"))
	require.NoError(t, svc.Stop(context.Background(), "inst-1"), "a second Stop is a no-op, per kernel contract")
	require.NoError(t, svc.Stop(context.Background(), "never-existed"), "Stop of an unknown id is a no-op, per kernel contract")
	require.Equal(t, []string{"inst-1", "inst-1", "never-existed"}, man.stops())
}

// ─── Cancel ──────────────────────────────────────────────────────────────────

func TestFleetService_Cancel_WithSessionIDCancelsExactlyThatSession(t *testing.T) {
	man := &fakeManager{}
	svc := New(man, agentregistryservice.New(mustDB(t)), nil, nil, "", nil)

	require.NoError(t, svc.Cancel(context.Background(), "inst-1", "sess-a"))
	require.Equal(t, []cancelCall{{instanceID: "inst-1", sessionID: "sess-a"}}, man.cancels())
}

func TestFleetService_Cancel_EmptySessionIDFansOutOverAllSessions(t *testing.T) {
	man := &fakeManager{
		statuses: map[string]agentinstance.InstanceStatus{
			"inst-1": {ID: "inst-1", SessionIDs: []string{"sess-a", "sess-b", "sess-c"}},
		},
	}
	svc := New(man, agentregistryservice.New(mustDB(t)), nil, nil, "", nil)

	require.NoError(t, svc.Cancel(context.Background(), "inst-1", ""))
	got := man.cancels()
	require.Len(t, got, 3)
	var sessions []string
	for _, c := range got {
		require.Equal(t, "inst-1", c.instanceID)
		sessions = append(sessions, string(c.sessionID))
	}
	require.ElementsMatch(t, []string{"sess-a", "sess-b", "sess-c"}, sessions)
}

func TestFleetService_Cancel_EmptySessionIDNoOpWhenNoSessions(t *testing.T) {
	man := &fakeManager{
		statuses: map[string]agentinstance.InstanceStatus{
			"inst-1": {ID: "inst-1", SessionIDs: nil},
		},
	}
	svc := New(man, agentregistryservice.New(mustDB(t)), nil, nil, "", nil)

	require.NoError(t, svc.Cancel(context.Background(), "inst-1", ""))
	require.Empty(t, man.cancels(), "safe with no turn in flight: zero sessions cancels nothing")
}

func TestFleetService_Cancel_UnknownInstancePropagatesNotFound(t *testing.T) {
	man := &fakeManager{statuses: map[string]agentinstance.InstanceStatus{}}
	svc := New(man, agentregistryservice.New(mustDB(t)), nil, nil, "", nil)

	err := svc.Cancel(context.Background(), "no-such-instance", "")
	require.Error(t, err)
	require.ErrorIs(t, err, agentinstance.ErrNotFound)
}

// ─── Cancel: the fan-out against a REAL kernel ──────────────────────────────
//
// The fakeManager tests above prove the fan-out LOOP; they cannot prove what it
// fans out OVER, because the fake hands back whatever SessionIDs the test set.
// That gap hid a real bug: InstanceStatus.SessionIDs was once derived from the
// kernel's viewer hub, whose per-session state materializes only on a session's
// first delivered update or first attach — so a session that was open but had
// emitted nothing was absent from the list and silently skipped by a
// cancel-everything. On local inference that silent window is the cold model
// load and the long first reasoning pass, i.e. exactly when a cancel matters.
// This test therefore drives the real kernel and a real (hermetic) downstream.

// buildStubAgentBin compiles libacp/cmd/acp-stub-agent — the hermetic in-repo ACP
// agent, no LLM backend — into t.TempDir() and returns its path.
func buildStubAgentBin(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "acp-stub-agent")
	out, err := exec.Command("go", "build", "-o", binPath, "github.com/contenox/runtime/libacp/cmd/acp-stub-agent").CombinedOutput()
	require.NoError(t, err, "build acp-stub-agent:\n%s", out)
	return binPath
}

// cancelRecordingManager wraps a REAL agentinstance.Manager and records the Cancel
// calls fleetservice makes, delegating every method (Get included, so SessionIDs is
// the kernel's genuine answer) to the wrapped kernel. It is a spy, not a stub: the
// only thing it fakes is observability.
type cancelRecordingManager struct {
	agentinstance.Manager

	mu      sync.Mutex
	records []cancelCall
}

func (m *cancelRecordingManager) Cancel(instanceID string, sessionID libacp.SessionID) error {
	m.mu.Lock()
	m.records = append(m.records, cancelCall{instanceID: instanceID, sessionID: sessionID})
	m.mu.Unlock()
	return m.Manager.Cancel(instanceID, sessionID)
}

func (m *cancelRecordingManager) cancels() []cancelCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]cancelCall(nil), m.records...)
}

// TestFleetService_Cancel_EmptySessionIDReachesSilentSession is the regression test
// for that bug, driven end-to-end: dispatch (whose intent-driven first turn resolves
// immediately and touches nothing this test cares about — see below), then assert a
// session-less Cancel still reaches the session even though nobody ever attached a
// viewer to it.
func TestFleetService_Cancel_EmptySessionIDReachesSilentSession(t *testing.T) {
	ctx, db := setupRegistryDB(t)
	agents := agentregistryservice.New(db)

	agent := &runtimetypes.Agent{Name: "silent-runner", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   buildStubAgentBin(t),
	}))
	require.NoError(t, agents.Create(ctx, agent))

	kernel := agentinstance.New(agents)
	t.Cleanup(func() { _ = kernel.Close() })
	man := &cancelRecordingManager{Manager: kernel}

	// Every dispatch is a mission now, so this end-to-end kernel test needs a
	// real mission registry too, even though the mission record itself is
	// incidental to what this test proves (the Cancel fan-out).
	missions := missionservice.New(db)
	svc := New(man, agents, missions, nil, t.TempDir(), libtracker.NoopTracker{})

	// The stub agent's first turn resolves quickly and leaves the session open
	// and quiet, with Dispatch attaching no viewer. This is the exact shape of
	// the black hole adopt/cancel exist for.
	result, err := svc.Dispatch(ctx, DispatchRequest{
		AgentName: "silent-runner", Intent: "do the thing", HITLPolicyName: "default",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionID)

	st, err := svc.Get(ctx, result.InstanceID)
	require.NoError(t, err)
	require.Equal(t, []string{result.SessionID}, st.SessionIDs,
		"an open-but-silent session must be visible on the fleet board")
	require.Zero(t, st.Viewers, "nothing is watching it — the condition that used to hide it")

	// THE REGRESSION: a session-less Cancel must reach that session, not skip it.
	require.NoError(t, svc.Cancel(ctx, result.InstanceID, ""))
	require.Equal(t,
		[]cancelCall{{instanceID: result.InstanceID, sessionID: libacp.SessionID(result.SessionID)}},
		man.cancels(),
		"cancel-everything must cancel the silent session")

	// Closing the session removes it from the fan-out set, so a second Cancel is a
	// genuine no-op rather than an error — the negative half of the same contract.
	require.NoError(t, kernel.CloseSession(result.InstanceID, libacp.SessionID(result.SessionID)))
	require.NoError(t, svc.Cancel(ctx, result.InstanceID, ""))
	require.Len(t, man.cancels(), 1, "a closed session is not cancelled again")
}

// ─── small local helpers (kept out of the setup section above since they are
//     one-liners used by exactly the Stop/Cancel tests, which don't need a
//     real agent registered) ───────────────────────────────────────────────

func mustDB(t *testing.T) libdb.DBManager {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "fleetservice-unused.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
