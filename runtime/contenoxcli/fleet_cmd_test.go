package contenoxcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/internal/fleetapi"
	"github.com/contenox/runtime/runtime/internal/missionapi"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// ─── test scaffolding: hand-fed fakes behind the REAL fleetapi/missionapi
// handlers, mounted at "/api" exactly like `contenox serve` mounts them. This
// exercises the serveClient wrappers against the real wire contract without
// spawning subprocesses or a `contenox serve` process — the narrow Service
// interfaces make the fakes trivial (fleet-consolidation.md's testing note for
// C3/M4). ────────────────────────────────────────────────────────────────────

// fakeFleetService is a hand-fed fleetservice.Service. Unknown ids surface as
// agentinstance.ErrNotFound so the fleetapi handlers map them to 404 exactly as
// the real service would.
type fakeFleetService struct {
	entries   []agentinstance.FleetEntry
	statuses  map[string]agentinstance.InstanceStatus
	dispatch  func(fleetservice.DispatchRequest) (fleetservice.DispatchResult, error)
	stopped   []string
	cancelled []cancelCall
}

type cancelCall struct{ instance, session string }

func (f *fakeFleetService) List(context.Context) ([]agentinstance.FleetEntry, error) {
	return f.entries, nil
}

func (f *fakeFleetService) Get(_ context.Context, instanceID string) (agentinstance.InstanceStatus, error) {
	if s, ok := f.statuses[instanceID]; ok {
		return s, nil
	}
	return agentinstance.InstanceStatus{}, agentinstance.ErrNotFound
}

func (f *fakeFleetService) Dispatch(_ context.Context, req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
	if f.dispatch != nil {
		return f.dispatch(req)
	}
	return fleetservice.DispatchResult{}, nil
}

func (f *fakeFleetService) Stop(_ context.Context, instanceID string) error {
	f.stopped = append(f.stopped, instanceID)
	return nil
}

func (f *fakeFleetService) Cancel(_ context.Context, instanceID, sessionID string) error {
	if _, ok := f.statuses[instanceID]; !ok && len(f.statuses) > 0 {
		return agentinstance.ErrNotFound
	}
	f.cancelled = append(f.cancelled, cancelCall{instanceID, sessionID})
	return nil
}

// fakeMissionService is a hand-fed missionservice.Service. Only the read paths
// the CLI exercises carry behavior; the mutations exist to satisfy the
// interface.
type fakeMissionService struct {
	missions []*missionservice.Mission
	reports  map[string][]*missionservice.Report
}

func (f *fakeMissionService) Create(context.Context, *missionservice.Mission) error { return nil }

func (f *fakeMissionService) Get(_ context.Context, id string) (*missionservice.Mission, error) {
	for _, m := range f.missions {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, libdb.ErrNotFound
}

func (f *fakeMissionService) List(_ context.Context, _ *time.Time, _ int) ([]*missionservice.Mission, error) {
	return f.missions, nil
}

func (f *fakeMissionService) Update(context.Context, *missionservice.Mission) error { return nil }
func (f *fakeMissionService) Delete(context.Context, string) error                  { return nil }

func (f *fakeMissionService) Finish(context.Context, string, missionservice.Status, string) (*missionservice.Mission, error) {
	return nil, nil
}

func (f *fakeMissionService) SetPlan(context.Context, string, []missionservice.PlanEntry, string) (*missionservice.Mission, error) {
	return nil, nil
}

func (f *fakeMissionService) Bind(context.Context, string, string, string) (*missionservice.Mission, error) {
	return nil, nil
}

func (f *fakeMissionService) Heartbeat(context.Context, string, string) (*missionservice.Mission, error) {
	return nil, nil
}

func (f *fakeMissionService) GetByInstance(context.Context, string) (*missionservice.Mission, error) {
	return nil, nil
}

func (f *fakeMissionService) AddReport(context.Context, string, *missionservice.Report) error {
	return nil
}

func (f *fakeMissionService) ListReports(_ context.Context, id string, _ int) ([]*missionservice.Report, error) {
	return f.reports[id], nil
}

func setupFleetTestServer(t *testing.T) (*fakeFleetService, *fakeMissionService, *httptest.Server) {
	t.Helper()
	fleet := &fakeFleetService{statuses: map[string]agentinstance.InstanceStatus{}}
	missions := &fakeMissionService{reports: map[string][]*missionservice.Report{}}

	apiMux := http.NewServeMux()
	fleetapi.AddRoutes(apiMux, fleet)
	missionapi.AddRoutes(apiMux, missions)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))

	srv := httptest.NewServer(rootMux)
	t.Cleanup(srv.Close)
	return fleet, missions, srv
}

// ─── test command builders (standalone, mirroring the real wiring) ──────────

func newFleetListTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: runFleetList}
	addServeClientFlags(c)
	c.Flags().Bool("json", false, "")
	return c
}

func newFleetShowTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "show", Args: cobra.ExactArgs(1), RunE: runFleetShow}
	addServeClientFlags(c)
	c.Flags().Bool("json", false, "")
	return c
}

func newFleetStopTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "stop", Args: cobra.ExactArgs(1), RunE: runFleetStop}
	addServeClientFlags(c)
	return c
}

func newFleetCancelTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "cancel", Args: cobra.ExactArgs(1), RunE: runFleetCancel}
	addServeClientFlags(c)
	c.Flags().String("session", "", "")
	return c
}

func runTestCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// ─── list ───────────────────────────────────────────────────────────────────

func TestUnit_FleetList_EmptyRendersHint(t *testing.T) {
	_, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newFleetListTestCmd(), "--server", srv.URL)
	require.NoError(t, err)
	require.Equal(t, "(no agents declared)\n", out)
}

func TestUnit_FleetList_BoardShowsUnitsWithMissionIntent(t *testing.T) {
	fleet, missions, srv := setupFleetTestServer(t)
	fleet.entries = []agentinstance.FleetEntry{
		{
			AgentID:   "a1",
			AgentName: "reviewer",
			Kind:      "external_acp",
			Instances: []agentinstance.InstanceStatus{
				{ID: "inst-1", AgentName: "reviewer", Kind: "external_acp", State: agentinstance.StateRunning, Sessions: 1, Viewers: 0},
			},
		},
		// A declared-but-idle agent: no instances, so it renders as "idle".
		{AgentID: "a2", AgentName: "planner", Kind: "chain"},
	}
	missions.missions = []*missionservice.Mission{
		{ID: "m1", InstanceID: "inst-1", Intent: "triage the failing CI run", Status: missionservice.StatusOpen},
	}

	out, err := runTestCmd(t, newFleetListTestCmd(), "--server", srv.URL)
	require.NoError(t, err)

	for _, want := range []string{
		"AGENT", "KIND", "STATE", "INSTANCE", "SESSIONS", "VIEWERS", "INTENT",
		"reviewer", "external_acp", "running", "inst-1",
		"triage the failing CI run", // the mission intent joined onto the unit
		"planner", "chain", "idle",  // the idle agent still appears
	} {
		require.Contains(t, out, want)
	}
}

func TestUnit_FleetList_JSONEmitsRawEntries(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.entries = []agentinstance.FleetEntry{
		{AgentID: "a1", AgentName: "reviewer", Kind: "external_acp"},
	}
	out, err := runTestCmd(t, newFleetListTestCmd(), "--server", srv.URL, "--json")
	require.NoError(t, err)

	var got []agentinstance.FleetEntry
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 1)
	require.Equal(t, "reviewer", got[0].AgentName)
}

// A unit with no mission bound to it renders its INTENT as absent ("-") rather
// than blanking the row — the join is best-effort presentation over the
// load-bearing unit data.
func TestUnit_FleetList_UnitWithNoMissionRendersDashIntent(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.entries = []agentinstance.FleetEntry{
		{AgentName: "reviewer", Kind: "external_acp", Instances: []agentinstance.InstanceStatus{
			{ID: "inst-1", Kind: "external_acp", State: agentinstance.StateRunning},
		}},
	}
	out, err := runTestCmd(t, newFleetListTestCmd(), "--server", srv.URL)
	require.NoError(t, err)
	require.Contains(t, out, "reviewer")
	require.Contains(t, out, "inst-1")
	// The single data row ends with the absent-intent marker.
	require.Regexp(t, `inst-1.*-\n?$`, strings.TrimSpace(out))
}

// ─── show ───────────────────────────────────────────────────────────────────

func TestUnit_FleetShow_RendersStatus(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.statuses["inst-1"] = agentinstance.InstanceStatus{
		ID: "inst-1", AgentName: "reviewer", Kind: "external_acp",
		State: agentinstance.StateRunning, Sessions: 2, Viewers: 1,
		StartedAt:  time.Now().UTC(),
		SessionIDs: []string{"sess-a", "sess-b"},
	}

	out, err := runTestCmd(t, newFleetShowTestCmd(), "--server", srv.URL, "inst-1")
	require.NoError(t, err)
	for _, want := range []string{"inst-1", "reviewer", "external_acp", "running", "sess-a", "sess-b"} {
		require.Contains(t, out, want)
	}
}

func TestUnit_FleetShow_JSONEmitsRawStatus(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.statuses["inst-1"] = agentinstance.InstanceStatus{ID: "inst-1", State: agentinstance.StateRunning}
	out, err := runTestCmd(t, newFleetShowTestCmd(), "--server", srv.URL, "--json", "inst-1")
	require.NoError(t, err)
	var got agentinstance.InstanceStatus
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, "inst-1", got.ID)
}

func TestUnit_FleetShow_UnknownIDFailsNonZero(t *testing.T) {
	_, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newFleetShowTestCmd(), "--server", srv.URL, "no-such-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-id")
	require.NotContains(t, out, "Instance:")
}

// ─── stop ───────────────────────────────────────────────────────────────────

func TestUnit_FleetStop_ConfirmsAndCallsService(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newFleetStopTestCmd(), "--server", srv.URL, "inst-1")
	require.NoError(t, err)
	require.Equal(t, "Instance inst-1 stopped.\n", out)
	require.Equal(t, []string{"inst-1"}, fleet.stopped)
}

// ─── cancel ─────────────────────────────────────────────────────────────────

func TestUnit_FleetCancel_AllSessions(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newFleetCancelTestCmd(), "--server", srv.URL, "inst-1")
	require.NoError(t, err)
	require.Contains(t, out, "Cancelled every in-flight turn on instance inst-1")
	require.Equal(t, []cancelCall{{"inst-1", ""}}, fleet.cancelled)
}

func TestUnit_FleetCancel_SpecificSession(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newFleetCancelTestCmd(), "--server", srv.URL, "--session", "sess-a", "inst-1")
	require.NoError(t, err)
	require.Contains(t, out, "Cancelled session sess-a on instance inst-1")
	require.Equal(t, []cancelCall{{"inst-1", "sess-a"}}, fleet.cancelled)
}

func TestUnit_FleetCancel_UnknownInstanceFailsNonZero(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	// A non-empty status map with a miss makes the fake return ErrNotFound,
	// which fleetapi maps to 404.
	fleet.statuses["other"] = agentinstance.InstanceStatus{ID: "other"}
	_, err := runTestCmd(t, newFleetCancelTestCmd(), "--server", srv.URL, "no-such-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-id")
}
