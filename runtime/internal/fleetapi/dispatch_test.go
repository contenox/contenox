package fleetapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/require"
)

// dispatchStub embeds the read-path stubManager for the interface's no-op
// defaults and overrides the lifecycle methods dispatch drives, recording every
// call. Prompt optionally blocks on release so a test can prove the handler
// returns before the first turn completes.
type dispatchStub struct {
	stubManager

	mu         sync.Mutex
	startID    string
	startErr   error
	startCalls []string
	openID     libacp.SessionID
	openErr    error
	openSpecs  []agentinstance.SessionSpec
	stopCalls  []string

	promptErr      error
	promptStarted  chan struct{}
	promptReleased chan struct{}
	promptDone     chan struct{}
}

func (d *dispatchStub) Start(_ context.Context, agentName string) (string, error) {
	d.mu.Lock()
	d.startCalls = append(d.startCalls, agentName)
	d.mu.Unlock()
	if d.startErr != nil {
		return "", d.startErr
	}
	return d.startID, nil
}

func (d *dispatchStub) OpenSession(_ context.Context, _ string, spec agentinstance.SessionSpec) (libacp.SessionID, error) {
	d.mu.Lock()
	d.openSpecs = append(d.openSpecs, spec)
	d.mu.Unlock()
	if d.openErr != nil {
		return "", d.openErr
	}
	return d.openID, nil
}

func (d *dispatchStub) Prompt(_ context.Context, _ string, _ libacp.SessionID, _ []libacp.ContentBlock) (libacp.StopReason, error) {
	if d.promptStarted != nil {
		close(d.promptStarted)
	}
	if d.promptReleased != nil {
		<-d.promptReleased
	}
	if d.promptDone != nil {
		close(d.promptDone)
	}
	return libacp.StopReasonEndTurn, d.promptErr
}

func (d *dispatchStub) Stop(instanceID string) error {
	d.mu.Lock()
	d.stopCalls = append(d.stopCalls, instanceID)
	d.mu.Unlock()
	return nil
}

func (d *dispatchStub) starts() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.startCalls...)
}

func (d *dispatchStub) specs() []agentinstance.SessionSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]agentinstance.SessionSpec(nil), d.openSpecs...)
}

// stubMissions records Create/Bind so a test can assert a mission was created and
// bound to both ids. The remaining Service methods are unused by dispatch.
type stubMissions struct {
	mu        sync.Mutex
	nextID    string
	createErr error
	bindErr   error
	created   []*missionservice.Mission
	binds     []bindCall
}

type bindCall struct {
	id         string
	sessionID  string
	instanceID string
}

func (s *stubMissions) Create(_ context.Context, m *missionservice.Mission) error {
	if s.createErr != nil {
		return s.createErr
	}
	if m.ID == "" {
		m.ID = s.nextID
	}
	s.mu.Lock()
	s.created = append(s.created, m)
	s.mu.Unlock()
	return nil
}

func (s *stubMissions) Bind(_ context.Context, id, sessionID, instanceID string) (*missionservice.Mission, error) {
	if s.bindErr != nil {
		return nil, s.bindErr
	}
	s.mu.Lock()
	s.binds = append(s.binds, bindCall{id: id, sessionID: sessionID, instanceID: instanceID})
	s.mu.Unlock()
	return &missionservice.Mission{ID: id, SessionIDs: []string{sessionID}, InstanceIDs: []string{instanceID}}, nil
}

func (s *stubMissions) Get(context.Context, string) (*missionservice.Mission, error) { return nil, nil }
func (s *stubMissions) List(context.Context, *time.Time, int) ([]*missionservice.Mission, error) {
	return nil, nil
}
func (s *stubMissions) Update(context.Context, *missionservice.Mission) error { return nil }
func (s *stubMissions) Delete(context.Context, string) error                  { return nil }

func (s *stubMissions) createdCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.created)
}

func setupDispatchAPI(t *testing.T, m agentinstance.Manager, missions missionservice.Service, roots *vfs.Factory, projectRoot string, tracker libtracker.ActivityTracker) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	AddRoutes(mux, m, missions, roots, projectRoot, tracker)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func dispatchPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url+"/fleet/dispatch", "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func TestUnit_FleetAPI_DispatchHappyPathWithPromptAndIntent(t *testing.T) {
	man := &dispatchStub{
		startID:        "inst-7",
		openID:         "sess-7",
		promptStarted:  make(chan struct{}),
		promptReleased: make(chan struct{}),
		promptDone:     make(chan struct{}),
	}
	close(man.promptReleased) // let the async prompt run to completion freely
	missions := &stubMissions{nextID: "mission-7"}

	srv := setupDispatchAPI(t, man, missions, nil, "/project/root", libtracker.NoopTracker{})

	resp := dispatchPost(t, srv.URL, DispatchRequest{
		AgentName:     "runner",
		Prompt:        "do the thing",
		MissionIntent: "ship the board",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got DispatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "inst-7", got.InstanceID)
	require.Equal(t, "sess-7", got.SessionID)
	require.Equal(t, "mission-7", got.MissionID)

	require.Equal(t, []string{"runner"}, man.starts())

	// A mission was created and bound to BOTH ids.
	require.Equal(t, 1, missions.createdCount())
	require.Equal(t, "ship the board", missions.created[0].Intent)
	require.Equal(t, "runner", missions.created[0].AgentName)
	require.Len(t, missions.binds, 1)
	require.Equal(t, bindCall{id: "mission-7", sessionID: "sess-7", instanceID: "inst-7"}, missions.binds[0])

	// The detached first prompt runs.
	select {
	case <-man.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("async prompt never ran")
	}
}

func TestUnit_FleetAPI_DispatchWithoutPromptSkipsPrompt(t *testing.T) {
	man := &dispatchStub{
		startID:       "inst-1",
		openID:        "sess-1",
		promptStarted: make(chan struct{}),
	}
	srv := setupDispatchAPI(t, man, nil, nil, "/project/root", nil)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got DispatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "inst-1", got.InstanceID)
	require.Equal(t, "sess-1", got.SessionID)
	require.Empty(t, got.MissionID)

	// No prompt supplied => no prompt goroutine spawned.
	select {
	case <-man.promptStarted:
		t.Fatal("Prompt was called for a dispatch with no prompt")
	case <-time.After(100 * time.Millisecond):
	}

	// cwd defaulted to the project root (no allowlist configured).
	specs := man.specs()
	require.Len(t, specs, 1)
	require.Equal(t, "/project/root", specs[0].Cwd)
}

func TestUnit_FleetAPI_DispatchWithoutIntentCreatesNoMission(t *testing.T) {
	man := &dispatchStub{startID: "inst-2", openID: "sess-2"}
	missions := &stubMissions{nextID: "should-not-be-used"}
	srv := setupDispatchAPI(t, man, missions, nil, "/project/root", nil)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner", Prompt: ""})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got DispatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Empty(t, got.MissionID)
	require.Equal(t, 0, missions.createdCount())
	require.Empty(t, missions.binds)
}

func TestUnit_FleetAPI_DispatchUnknownAgentMapsToNotFound(t *testing.T) {
	man := &dispatchStub{
		startErr: fmt.Errorf("agentinstance: resolve agent %q: %w", "ghost", libdb.ErrNotFound),
	}
	srv := setupDispatchAPI(t, man, nil, nil, "/project/root", nil)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "ghost"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUnit_FleetAPI_DispatchInvalidCwdRejected(t *testing.T) {
	allowed := t.TempDir()
	roots, err := vfs.NewFactory(allowed)
	require.NoError(t, err)

	man := &dispatchStub{startID: "inst-3", openID: "sess-3"}
	srv := setupDispatchAPI(t, man, nil, roots, "", nil)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner", Cwd: t.TempDir()})
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Rejected before any instance is brought up.
	require.Empty(t, man.starts())
}

func TestUnit_FleetAPI_DispatchIntentWithNilMissionsRejected(t *testing.T) {
	man := &dispatchStub{startID: "inst-4", openID: "sess-4"}
	srv := setupDispatchAPI(t, man, nil, nil, "/project/root", nil)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner", MissionIntent: "orphan intent"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Rejected before any instance is brought up.
	require.Empty(t, man.starts())
}

func TestUnit_FleetAPI_DispatchAsyncPromptDoesNotBlockResponse(t *testing.T) {
	man := &dispatchStub{
		startID:        "inst-5",
		openID:         "sess-5",
		promptStarted:  make(chan struct{}),
		promptReleased: make(chan struct{}),
		promptDone:     make(chan struct{}),
	}
	srv := setupDispatchAPI(t, man, nil, nil, "/project/root", libtracker.NoopTracker{})

	// The response returns even though Prompt is blocked on promptReleased.
	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner", Prompt: "long turn"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	// The detached prompt goroutine was spawned...
	select {
	case <-man.promptStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt goroutine never started")
	}
	// ...but the handler did NOT wait for it to finish (still blocked).
	select {
	case <-man.promptDone:
		t.Fatal("handler returned only after the prompt completed")
	default:
	}

	// Release the prompt and confirm it runs to completion.
	close(man.promptReleased)
	select {
	case <-man.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt never completed after release")
	}
}
