package fleetapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/stretchr/testify/require"
)

// stubManager implements agentinstance.Manager with canned List/Get results;
// the lifecycle methods are never reached by the read-only routes.
type stubManager struct {
	entries []agentinstance.FleetEntry
	byID    map[string]agentinstance.InstanceStatus
}

func (s *stubManager) Start(context.Context, string) (string, error) { return "", nil }

func (s *stubManager) Attach(context.Context, string, libacp.SessionID, agentinstance.Viewer) (bool, error) {
	return false, nil
}

func (s *stubManager) Detach(string, libacp.SessionID, string) error { return nil }

func (s *stubManager) List(context.Context) ([]agentinstance.FleetEntry, error) {
	return s.entries, nil
}

func (s *stubManager) Get(instanceID string) (agentinstance.InstanceStatus, error) {
	status, ok := s.byID[instanceID]
	if !ok {
		return agentinstance.InstanceStatus{}, fmt.Errorf("agentinstance: %q: %w", instanceID, agentinstance.ErrNotFound)
	}
	return status, nil
}

func (s *stubManager) OpenSession(context.Context, string, agentinstance.SessionSpec) (libacp.SessionID, error) {
	return "", nil
}

func (s *stubManager) Prompt(context.Context, string, libacp.SessionID, []libacp.ContentBlock) (libacp.StopReason, error) {
	return "", nil
}

func (s *stubManager) Cancel(string, libacp.SessionID) error       { return nil }
func (s *stubManager) CloseSession(string, libacp.SessionID) error { return nil }

func (s *stubManager) SetConfigOption(context.Context, string, libacp.SessionID, string, libacp.SessionConfigOptionValue) error {
	return nil
}

func (s *stubManager) SessionConfigOptions(string, libacp.SessionID) ([]libacp.SessionConfigOption, error) {
	return nil, nil
}

func (s *stubManager) AvailableCommands(string, libacp.SessionID) ([]libacp.AvailableCommand, error) {
	return nil, nil
}

func (s *stubManager) Stop(string) error { return nil }
func (s *stubManager) Close() error      { return nil }

func setupFleetAPI(t *testing.T, m agentinstance.Manager) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	// Read-path tests need no dispatch dependencies (mission registry, workspace
	// allowlist, project root, tracker); dispatch has its own test file.
	AddRoutes(mux, m, nil, nil, "", nil)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnit_FleetAPI_ListReturnsConfigRuntimeJoin(t *testing.T) {
	started := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	running := agentinstance.InstanceStatus{
		ID:        "inst-1",
		AgentID:   "agent-1",
		AgentName: "runner",
		Kind:      "external_acp",
		State:     agentinstance.StateRunning,
		Sessions:  2,
		Viewers:   1,
		StartedAt: started,
	}
	srv := setupFleetAPI(t, &stubManager{entries: []agentinstance.FleetEntry{
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
	require.True(t, got.StartedAt.Equal(started))

	require.Equal(t, "idle-bot", entries[1].AgentName)
	require.Empty(t, entries[1].Instances)
}

func TestUnit_FleetAPI_ListEmptyFleet(t *testing.T) {
	srv := setupFleetAPI(t, &stubManager{entries: []agentinstance.FleetEntry{}})

	resp, err := http.Get(srv.URL + "/fleet")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var buf strings.Builder
	_, err = io.Copy(&buf, resp.Body)
	require.NoError(t, err)
	require.Equal(t, "[]", strings.TrimSpace(buf.String()))
}

func TestUnit_FleetAPI_GetReturnsInstanceStatus(t *testing.T) {
	started := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	srv := setupFleetAPI(t, &stubManager{byID: map[string]agentinstance.InstanceStatus{
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
	srv := setupFleetAPI(t, &stubManager{})

	resp, err := http.Get(srv.URL + "/fleet/no-such-instance")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
