package contenoxcli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// Reuses fakeFleetService / fakeMissionService / setupFleetTestServer from
// fleet_cmd_test.go (same package): `mission fire` is served by /fleet/dispatch,
// and `mission list|show` by /missions, so both fakes are mounted already.

func newMissionFireTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "fire", Args: cobra.NoArgs, RunE: runMissionFire}
	addServeClientFlags(c)
	c.Flags().String("agent", "", "")
	c.Flags().String("intent", "", "")
	c.Flags().String("policy", "", "")
	c.Flags().String("cwd", "", "")
	c.Flags().Bool("json", false, "")
	c.Flags().BoolP("quiet", "q", false, "")
	c.Flags().Bool("wait", false, "")
	c.Flags().Duration("wait-timeout", 5*time.Minute, "")
	c.Flags().Duration("wait-interval", 2*time.Second, "")
	// --db lets a test point the config-default resolution at a temp database
	// instead of the operator's real ~/.contenox/local.db (resolveDBPath reads
	// this flag). The real command inherits it as a root persistent flag.
	c.Flags().String("db", "", "")
	return c
}

// exitCodeOf extracts the exit status a command chose: 0 for a nil error, or the
// code carried by the *exitError `mission fire --wait` returns.
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var ee *exitError
	require.ErrorAs(t, err, &ee)
	return ee.code
}

func newMissionListTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: runMissionList}
	addServeClientFlags(c)
	c.Flags().Int("limit", 100, "")
	c.Flags().Bool("json", false, "")
	return c
}

func newMissionShowTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "show", Args: cobra.ExactArgs(1), RunE: runMissionShow}
	addServeClientFlags(c)
	c.Flags().Int("limit", 100, "")
	c.Flags().Bool("json", false, "")
	return c
}

// ─── fire ─────────────────────────────────────────────────────────────────

func TestUnit_MissionFire_DispatchesAndPrintsIDs(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	var got fleetservice.DispatchRequest
	fleet.dispatch = func(req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
		got = req
		return fleetservice.DispatchResult{InstanceID: "inst-9", SessionID: "sess-9", MissionID: "m-9"}, nil
	}

	out, err := runTestCmd(t, newMissionFireTestCmd(), "--server", srv.URL,
		"--agent", "reviewer", "--intent", "triage the failing CI run", "--policy", "hitl-policy-strict.json")
	require.NoError(t, err)

	// The request the server received carries exactly the flags, and NO parent
	// session id — an operator firing from the shell supervises directly.
	require.Equal(t, "reviewer", got.AgentName)
	require.Equal(t, "triage the failing CI run", got.Intent)
	require.Equal(t, "hitl-policy-strict.json", got.HITLPolicyName)
	require.Empty(t, got.ParentSessionID)

	for _, want := range []string{"Mission fired.", "m-9", "inst-9", "sess-9", "contenox mission show m-9"} {
		require.Contains(t, out, want)
	}
}

func TestUnit_MissionFire_JSONEmitsResult(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.dispatch = func(fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
		return fleetservice.DispatchResult{InstanceID: "inst-9", SessionID: "sess-9", MissionID: "m-9"}, nil
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), "--server", srv.URL,
		"--agent", "reviewer", "--intent", "go", "--policy", "p.json", "--json")
	require.NoError(t, err)

	var res fleetservice.DispatchResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Equal(t, "m-9", res.MissionID)
}

func TestUnit_MissionFire_RequiresIntent(t *testing.T) {
	_, _, srv := setupFleetTestServer(t)
	_, err := runTestCmd(t, newMissionFireTestCmd(), "--server", srv.URL, "--agent", "reviewer", "--policy", "p.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "intent")
}

// A missing agent that the server refuses (Dispatch validation) surfaces as a
// non-zero exit. The fake's default Dispatch returns a validation-shaped error.
func TestUnit_MissionFire_ServerRefusesMissingAgent(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.dispatch = func(req fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
		if strings.TrimSpace(req.AgentName) == "" {
			return fleetservice.DispatchResult{}, apiframework.MissingParameter("agentName", "agentName is required")
		}
		return fleetservice.DispatchResult{MissionID: "m-1"}, nil
	}
	// Pass an empty --agent explicitly and a temp --db so the config-default
	// resolution opens an empty DB (no default set) rather than the real one.
	_, err := runTestCmd(t, newMissionFireTestCmd(), "--server", srv.URL,
		"--db", t.TempDir()+"/mission-fire.db", "--intent", "go", "--policy", "p.json")
	require.Error(t, err)
}

func TestUnit_MissionFire_QuietPrintsOnlyMissionID(t *testing.T) {
	fleet, _, srv := setupFleetTestServer(t)
	fleet.dispatch = func(fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
		return fleetservice.DispatchResult{InstanceID: "inst-9", SessionID: "sess-9", MissionID: "m-9"}, nil
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), "--server", srv.URL,
		"-q", "--agent", "reviewer", "--intent", "go", "--policy", "p.json")
	require.NoError(t, err)
	require.Equal(t, "m-9\n", out, "quiet mode prints only the bare mission id for $(...) capture")
}

// ─── fire --wait (the scripting primitive) ──────────────────────────────────

// seedFireForWait returns a server whose dispatch produces a known unit, so a
// --wait test can stage the terminal condition the poller will observe.
func seedFireForWait(t *testing.T) (*fakeFleetService, *fakeMissionService, string) {
	t.Helper()
	fleet, missions, srv := setupFleetTestServer(t)
	fleet.dispatch = func(fleetservice.DispatchRequest) (fleetservice.DispatchResult, error) {
		return fleetservice.DispatchResult{InstanceID: "inst-9", SessionID: "sess-9", MissionID: "m-9"}, nil
	}
	return fleet, missions, srv.URL
}

func fireWaitArgs(url string, extra ...string) []string {
	base := []string{"--server", url, "-q", "--wait",
		"--wait-interval", "10ms", "--wait-timeout", "2s",
		"--agent", "reviewer", "--intent", "go", "--policy", "p.json"}
	return append(base, extra...)
}

// A RESULT report is the decisive "completed" fallback signal: exit 0.
func TestUnit_MissionFireWait_ResultReportExitsZero(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindResult, Summary: "done"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, 0, exitCodeOf(t, err), "a result report is the completed-mission fallback signal")
	require.Contains(t, out, "m-9") // the id still printed to stdout
	require.Contains(t, out, "produced a result")
}

// A PROGRESS report is INTERMEDIATE — the unit is alive and still working — so
// --wait does NOT exit 0 on it; with the unit still running it rides out to the
// deadline (exit 3). This is the kind-aware refinement: before, any non-blocker
// report exited 0, calling a mid-flight progress note a completed mission.
func TestUnit_MissionFireWait_ProgressReportKeepsWaiting(t *testing.T) {
	fleet, missions, url := seedFireForWait(t)
	fleet.statuses["inst-9"] = agentinstance.InstanceStatus{ID: "inst-9", State: agentinstance.StateRunning}
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindProgress, Summary: "halfway there"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url, "--wait-timeout", "40ms")...)
	require.Equal(t, missionWaitTimeout, exitCodeOf(t, err), "a progress report keeps the wait open, not a success")
	require.Contains(t, out, "timed out")
}

// A FINDING report is likewise intermediate: it keeps the wait open.
func TestUnit_MissionFireWait_FindingReportKeepsWaiting(t *testing.T) {
	fleet, missions, url := seedFireForWait(t)
	fleet.statuses["inst-9"] = agentinstance.InstanceStatus{ID: "inst-9", State: agentinstance.StateRunning}
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindFinding, Summary: "spotted a race"},
	}
	_, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url, "--wait-timeout", "40ms")...)
	require.Equal(t, missionWaitTimeout, exitCodeOf(t, err), "a finding report keeps the wait open, not a success")
}

// The newest report decides: a result filed AFTER an earlier progress note is the
// decisive signal, so the wait exits 0 (reports are newest-first).
func TestUnit_MissionFireWait_ResultAfterProgressExitsZero(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-2", Kind: missionservice.ReportKindResult, Summary: "shipped"},
		{ID: "r-1", Kind: missionservice.ReportKindProgress, Summary: "halfway there"},
	}
	_, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, 0, exitCodeOf(t, err), "the newest report (a result) is the decisive signal")
}

func TestUnit_MissionFireWait_BlockerExitsTwo(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindBlocker, Summary: "needs a secret"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, missionWaitBlocked, exitCodeOf(t, err))
	require.Contains(t, out, "BLOCKED")
}

func TestUnit_MissionFireWait_InstanceStoppedExitsOne(t *testing.T) {
	fleet, _, url := seedFireForWait(t)
	fleet.statuses["inst-9"] = agentinstance.InstanceStatus{ID: "inst-9", State: agentinstance.StateStopped}
	_, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, missionWaitStopped, exitCodeOf(t, err))
}

func TestUnit_MissionFireWait_InstanceErrorExitsOne(t *testing.T) {
	fleet, _, url := seedFireForWait(t)
	fleet.statuses["inst-9"] = agentinstance.InstanceStatus{ID: "inst-9", State: agentinstance.StateError}
	_, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, missionWaitStopped, exitCodeOf(t, err))
}

// The mission reached a terminal Status: --wait prefers it over reports and
// instance state. landed → 0.
func TestUnit_MissionFireWait_StatusLandedExitsZero(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-9", Status: missionservice.StatusLanded, StatusReason: "all green"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, 0, exitCodeOf(t, err), "a landed mission is the authoritative success signal")
	require.Contains(t, out, "LANDED")
	require.Contains(t, out, "all green")
}

// derailed → 1.
func TestUnit_MissionFireWait_StatusDerailedExitsOne(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-9", Status: missionservice.StatusDerailed, StatusReason: "hit a wall"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, missionWaitStopped, exitCodeOf(t, err))
	require.Contains(t, out, "DERAILED")
}

// stuck → 1 (shares the "ended without success" code with derailed; the
// distinction lives in the status, not the shell exit code).
func TestUnit_MissionFireWait_StatusStuckExitsOne(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-9", Status: missionservice.StatusStuck, StatusReason: "wedged in a loop"},
	}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, missionWaitStopped, exitCodeOf(t, err))
	require.Contains(t, out, "STUCK")
}

// Status wins over reports: a landed mission that ALSO has a blocker report
// still exits 0 — Status is checked first as the authoritative outcome.
func TestUnit_MissionFireWait_StatusPreferredOverReports(t *testing.T) {
	_, missions, url := seedFireForWait(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-9", Status: missionservice.StatusLanded},
	}
	missions.reports["m-9"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindBlocker, Summary: "stale blocker"},
	}
	_, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url)...)
	require.Equal(t, 0, exitCodeOf(t, err), "terminal status is authoritative over a lingering report")
}

// No report and a still-running unit: --wait must give up at the deadline with
// the indeterminate code rather than hang or claim success.
func TestUnit_MissionFireWait_TimeoutExitsThree(t *testing.T) {
	fleet, _, url := seedFireForWait(t)
	fleet.statuses["inst-9"] = agentinstance.InstanceStatus{ID: "inst-9", State: agentinstance.StateRunning}
	out, err := runTestCmd(t, newMissionFireTestCmd(), fireWaitArgs(url, "--wait-timeout", "40ms")...)
	require.Equal(t, missionWaitTimeout, exitCodeOf(t, err))
	require.Contains(t, out, "timed out")
}

// ─── list ───────────────────────────────────────────────────────────────────

func TestUnit_MissionList_EmptyRendersHint(t *testing.T) {
	_, _, srv := setupFleetTestServer(t)
	out, err := runTestCmd(t, newMissionListTestCmd(), "--server", srv.URL)
	require.NoError(t, err)
	require.Equal(t, "(no missions)\n", out)
}

func TestUnit_MissionList_TableShowsWork(t *testing.T) {
	_, missions, srv := setupFleetTestServer(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-1", Status: missionservice.StatusOpen, AgentName: "reviewer",
			HITLPolicyName: "hitl-policy-strict.json", Intent: "triage the failing CI run",
			UpdatedAt: time.Now().UTC()},
	}
	out, err := runTestCmd(t, newMissionListTestCmd(), "--server", srv.URL)
	require.NoError(t, err)
	for _, want := range []string{
		"ID", "STATUS", "AGENT", "ENVELOPE", "UPDATED", "INTENT",
		"m-1", "open", "reviewer", "hitl-policy-strict.json", "triage the failing CI run",
	} {
		require.Contains(t, out, want)
	}
}

func TestUnit_MissionList_JSONEmitsRawRecords(t *testing.T) {
	_, missions, srv := setupFleetTestServer(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-1", Status: missionservice.StatusOpen, Intent: "go", ParentSessionID: "parent-7"},
	}
	out, err := runTestCmd(t, newMissionListTestCmd(), "--server", srv.URL, "--json")
	require.NoError(t, err)
	var got []*missionservice.Mission
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 1)
	require.Equal(t, "parent-7", got[0].ParentSessionID, "the raw JSON carries the supervision edge")
}

// ─── show ───────────────────────────────────────────────────────────────────

func TestUnit_MissionShow_RendersMissionAndReports(t *testing.T) {
	_, missions, srv := setupFleetTestServer(t)
	hb := time.Now().UTC()
	missions.missions = []*missionservice.Mission{
		{ID: "m-1", Status: missionservice.StatusOpen, AgentName: "reviewer",
			HITLPolicyName: "envelope.json", Intent: "triage the failing CI run",
			InstanceID: "inst-1", SessionID: "sess-1", ParentSessionID: "parent-7",
			LastHeartbeat: &hb, CreatedAt: hb, UpdatedAt: hb},
	}
	missions.reports["m-1"] = []*missionservice.Report{
		{ID: "r-2", Kind: missionservice.ReportKindResult, Summary: "CI fixed", CreatedAt: hb.Add(time.Minute)},
		{ID: "r-1", Kind: missionservice.ReportKindProgress, Summary: "started triage", CreatedAt: hb},
	}

	out, err := runTestCmd(t, newMissionShowTestCmd(), "--server", srv.URL, "m-1")
	require.NoError(t, err)
	for _, want := range []string{
		"m-1", "reviewer", "envelope.json", "triage the failing CI run",
		"inst-1", "sess-1", "parent-7",
		"Reports (2, newest first)", "result", "CI fixed", "progress", "started triage",
	} {
		require.Contains(t, out, want)
	}
}

func TestUnit_MissionShow_NoReportsRendersNone(t *testing.T) {
	_, missions, srv := setupFleetTestServer(t)
	missions.missions = []*missionservice.Mission{
		{ID: "m-1", Status: missionservice.StatusOpen, Intent: "go"},
	}
	out, err := runTestCmd(t, newMissionShowTestCmd(), "--server", srv.URL, "m-1")
	require.NoError(t, err)
	require.Contains(t, out, "Reports: (none)")
}

func TestUnit_MissionShow_JSONEmitsMissionAndReports(t *testing.T) {
	_, missions, srv := setupFleetTestServer(t)
	missions.missions = []*missionservice.Mission{{ID: "m-1", Status: missionservice.StatusOpen, Intent: "go"}}
	missions.reports["m-1"] = []*missionservice.Report{
		{ID: "r-1", Kind: missionservice.ReportKindProgress, Summary: "started"},
	}
	out, err := runTestCmd(t, newMissionShowTestCmd(), "--server", srv.URL, "--json", "m-1")
	require.NoError(t, err)

	var got missionShowPayload
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, "m-1", got.Mission.ID)
	require.Len(t, got.Reports, 1)
	require.Equal(t, "started", got.Reports[0].Summary)
}

func TestUnit_MissionShow_UnknownIDFailsNonZero(t *testing.T) {
	_, _, srv := setupFleetTestServer(t)
	_, err := runTestCmd(t, newMissionShowTestCmd(), "--server", srv.URL, "no-such-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-id")
}
