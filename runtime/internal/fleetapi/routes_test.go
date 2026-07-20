package fleetapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/apiframework"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/stretchr/testify/require"
)

// fakeFleet is a hand-rolled fleetservice.Service double. These tests prove
// only ROUTING, status-code mapping, and DTO (de)serialization — the
// orchestration itself (Enabled policy, teardown-on-failure, cancel fan-out)
// is fleetservice's own responsibility and is covered by
// runtime/fleetservice's tests.
type fakeFleet struct {
	mu sync.Mutex

	listEntries []agentinstance.FleetEntry
	listErr     error

	byID   map[string]agentinstance.InstanceStatus
	getErr error // returned instead of the default ErrNotFound when set

	dispatchResult fleetservice.DispatchResult
	dispatchErr    error
	dispatchReqs   []fleetservice.DispatchRequest

	stopErr   error
	stopCalls []string

	cancelErr   error
	cancelCalls []cancelArgs
}

type cancelArgs struct {
	instanceID string
	sessionID  string
}

func (f *fakeFleet) List(context.Context) ([]agentinstance.FleetEntry, error) {
	return f.listEntries, f.listErr
}

func (f *fakeFleet) Get(_ context.Context, id string) (agentinstance.InstanceStatus, error) {
	st, ok := f.byID[id]
	if !ok {
		if f.getErr != nil {
			return agentinstance.InstanceStatus{}, f.getErr
		}
		return agentinstance.InstanceStatus{}, fmt.Errorf("fleetapi-test: %q: %w", id, agentinstance.ErrNotFound)
	}
	return st, nil
}

func (f *fakeFleet) Dispatch(_ context.Context, req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
	f.mu.Lock()
	f.dispatchReqs = append(f.dispatchReqs, req)
	f.mu.Unlock()
	if f.dispatchErr != nil {
		return fleetservice.DispatchResult{}, f.dispatchErr
	}
	return f.dispatchResult, nil
}

func (f *fakeFleet) Stop(_ context.Context, id string) error {
	f.mu.Lock()
	f.stopCalls = append(f.stopCalls, id)
	f.mu.Unlock()
	return f.stopErr
}

func (f *fakeFleet) Cancel(_ context.Context, instanceID, sessionID string) error {
	f.mu.Lock()
	f.cancelCalls = append(f.cancelCalls, cancelArgs{instanceID: instanceID, sessionID: sessionID})
	f.mu.Unlock()
	return f.cancelErr
}

func (f *fakeFleet) dispatches() []fleetservice.DispatchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fleetservice.DispatchRequest(nil), f.dispatchReqs...)
}

func (f *fakeFleet) cancels() []cancelArgs {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cancelArgs(nil), f.cancelCalls...)
}

var _ fleetservice.Service = (*fakeFleet)(nil)

func setupFleetAPI(t *testing.T, f fleetservice.Service) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	AddRoutes(mux, f)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ─── List ────────────────────────────────────────────────────────────────

func TestUnit_FleetAPI_ListReturnsConfigRuntimeJoin(t *testing.T) {
	started := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	running := agentinstance.InstanceStatus{
		ID:         "inst-1",
		AgentID:    "agent-1",
		AgentName:  "runner",
		Kind:       "external_acp",
		State:      agentinstance.StateRunning,
		Sessions:   2,
		Viewers:    1,
		StartedAt:  started,
		SessionIDs: []string{"sess-a", "sess-b"},
	}
	srv := setupFleetAPI(t, &fakeFleet{listEntries: []agentinstance.FleetEntry{
		{AgentID: "agent-1", AgentName: "runner", Kind: "external_acp", Instances: []agentinstance.InstanceStatus{running}},
		{AgentID: "agent-2", AgentName: "idle-bot", Kind: "chain"},
	}})

	resp, err := http.Get(srv.URL + "/fleet")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var entries []agentinstance.FleetEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&entries))
	require.Len(t, entries, 2)

	require.Equal(t, "runner", entries[0].AgentName)
	require.Len(t, entries[0].Instances, 1)
	got := entries[0].Instances[0]
	require.Equal(t, "inst-1", got.ID)
	require.Equal(t, agentinstance.StateRunning, got.State)
	require.Equal(t, 2, got.Sessions)
	require.Equal(t, []string{"sess-a", "sess-b"}, got.SessionIDs)
	require.True(t, got.StartedAt.Equal(started))

	require.Equal(t, "idle-bot", entries[1].AgentName)
	require.Empty(t, entries[1].Instances)
}

func TestUnit_FleetAPI_ListEmptyFleet(t *testing.T) {
	srv := setupFleetAPI(t, &fakeFleet{listEntries: []agentinstance.FleetEntry{}})

	resp, err := http.Get(srv.URL + "/fleet")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var buf strings.Builder
	_, err = io.Copy(&buf, resp.Body)
	require.NoError(t, err)
	require.Equal(t, "[]", strings.TrimSpace(buf.String()))
}

// ─── Get ─────────────────────────────────────────────────────────────────

func TestUnit_FleetAPI_GetReturnsInstanceStatus(t *testing.T) {
	started := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	srv := setupFleetAPI(t, &fakeFleet{byID: map[string]agentinstance.InstanceStatus{
		"inst-1": {
			ID:        "inst-1",
			AgentID:   "agent-1",
			AgentName: "runner",
			Kind:      "external_acp",
			State:     agentinstance.StateRunning,
			Sessions:  1,
			StartedAt: started,
		},
	}})

	resp, err := http.Get(srv.URL + "/fleet/inst-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got agentinstance.InstanceStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "inst-1", got.ID)
	require.Equal(t, "runner", got.AgentName)
	require.Equal(t, agentinstance.StateRunning, got.State)
	require.True(t, got.StartedAt.Equal(started))
}

func TestUnit_FleetAPI_GetUnknownReturns404(t *testing.T) {
	srv := setupFleetAPI(t, &fakeFleet{})

	resp, err := http.Get(srv.URL + "/fleet/no-such-instance")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── Dispatch ────────────────────────────────────────────────────────────

func dispatchPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url+"/fleet/dispatch", "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func TestUnit_FleetAPI_DispatchDecodesRequestAndEncodesResult(t *testing.T) {
	f := &fakeFleet{dispatchResult: fleetservice.DispatchResult{
		InstanceID: "inst-7", SessionID: "sess-7", MissionID: "mission-7",
	}}
	srv := setupFleetAPI(t, f)

	resp := dispatchPost(t, srv.URL, DispatchRequest{
		AgentName:      "runner",
		Intent:         "ship the board",
		HITLPolicyName: "default",
		Cwd:            "/workspace",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got DispatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "inst-7", got.InstanceID)
	require.Equal(t, "sess-7", got.SessionID)
	require.Equal(t, "mission-7", got.MissionID)

	reqs := f.dispatches()
	require.Len(t, reqs, 1)
	require.Equal(t, "runner", reqs[0].AgentName)
	require.Equal(t, "ship the board", reqs[0].Intent)
	require.Equal(t, "default", reqs[0].HITLPolicyName)
	require.Equal(t, "/workspace", reqs[0].Cwd)
}

// TestUnit_FleetAPI_DispatchWithMinimalRequest proves Cwd stays the only
// optional field at the wire layer, and that MissionID — always present now
// that every dispatch is a mission — decodes on the response like any other
// field (no omitempty-shaped test needed on the response side, since the
// generic decode above already proves that).
func TestUnit_FleetAPI_DispatchWithMinimalRequest(t *testing.T) {
	f := &fakeFleet{dispatchResult: fleetservice.DispatchResult{InstanceID: "inst-1", SessionID: "sess-1", MissionID: "mission-1"}}
	srv := setupFleetAPI(t, f)

	resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner", Intent: "do the thing", HITLPolicyName: "default"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got DispatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "inst-1", got.InstanceID)
	require.Equal(t, "mission-1", got.MissionID)

	reqs := f.dispatches()
	require.Len(t, reqs, 1)
	require.Equal(t, "runner", reqs[0].AgentName)
	require.Equal(t, "do the thing", reqs[0].Intent)
	require.Equal(t, "default", reqs[0].HITLPolicyName)
	require.Empty(t, reqs[0].Cwd, "cwd stays optional at the wire layer; fleetservice resolves the default")
}

// TestUnit_FleetAPI_DispatchServiceErrorMapsToStatus proves the handler does no
// error-mapping of its own beyond apiframework.Error: whatever status
// fleetservice's error carries (via the shared error taxonomy) is what the
// wire gets.
func TestUnit_FleetAPI_DispatchServiceErrorMapsToStatus(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"unknown agent -> 404", fmt.Errorf("fleetservice: resolve agent %q: %w", "ghost", libdb.ErrNotFound), http.StatusNotFound},
		{"disabled agent -> 409", apiframework.Conflict(`agent "runner" is disabled`), http.StatusConflict},
		{"missing agentName -> 400", apiframework.MissingParameter("agentName"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupFleetAPI(t, &fakeFleet{dispatchErr: tc.err})
			resp := dispatchPost(t, srv.URL, DispatchRequest{AgentName: "runner"})
			defer resp.Body.Close()
			require.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

// ─── Stop (DELETE) ───────────────────────────────────────────────────────

func TestUnit_FleetAPI_StopCallsServiceAndReturns200(t *testing.T) {
	f := &fakeFleet{}
	srv := setupFleetAPI(t, f)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/fleet/inst-1", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "deleted", got)
	require.Equal(t, []string{"inst-1"}, f.stopCalls)
}

func TestUnit_FleetAPI_StopPropagatesServiceError(t *testing.T) {
	f := &fakeFleet{stopErr: apiframework.InternalServerError("boom")}
	srv := setupFleetAPI(t, f)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/fleet/inst-1", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// ─── Cancel ──────────────────────────────────────────────────────────────

func cancelPost(t *testing.T, url, instanceID string, body []byte) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, url+"/fleet/"+instanceID+"/cancel", reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestUnit_FleetAPI_CancelWithSessionIDForwardsIt(t *testing.T) {
	f := &fakeFleet{}
	srv := setupFleetAPI(t, f)

	body, err := json.Marshal(CancelRequest{SessionID: "sess-a"})
	require.NoError(t, err)
	resp := cancelPost(t, srv.URL, "inst-1", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, []cancelArgs{{instanceID: "inst-1", sessionID: "sess-a"}}, f.cancels())
}

func TestUnit_FleetAPI_CancelWithNoBodyMeansAllSessions(t *testing.T) {
	f := &fakeFleet{}
	srv := setupFleetAPI(t, f)

	resp := cancelPost(t, srv.URL, "inst-1", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, []cancelArgs{{instanceID: "inst-1", sessionID: ""}}, f.cancels())
}

func TestUnit_FleetAPI_CancelWithEmptyBodyMeansAllSessions(t *testing.T) {
	f := &fakeFleet{}
	srv := setupFleetAPI(t, f)

	resp := cancelPost(t, srv.URL, "inst-1", []byte{})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, []cancelArgs{{instanceID: "inst-1", sessionID: ""}}, f.cancels())
}

func TestUnit_FleetAPI_CancelUnknownInstanceReturns404(t *testing.T) {
	f := &fakeFleet{cancelErr: fmt.Errorf("fleetapi-test: %q: %w", "no-such-instance", agentinstance.ErrNotFound)}
	srv := setupFleetAPI(t, f)

	resp := cancelPost(t, srv.URL, "no-such-instance", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
