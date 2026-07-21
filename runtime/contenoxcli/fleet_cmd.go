// fleet_cmd.go is the `contenox fleet` command tree — the shell surface over
// the fleet board (fleet-consolidation.md slice C3, "the shell is this
// product's operating surface"). The verbs here operate on UNITS — list, show,
// stop, cancel — the process-management half of the fleet. WORK (firing a
// mission, reading its reports) lives under `contenox mission`, per the M4
// noun-split: firing is `mission fire` even though it is served by
// POST /fleet/dispatch, so `fleet` never grows a `dispatch` that would be a
// second name for one act.
//
// EXECUTED access decision (C3): unlike `contenox state`/`session`, which open
// the SQLite database directly, the fleet lives in `contenox serve`'s MEMORY —
// a live Manager owning subprocesses — and is reachable only over HTTP. So this
// tree is a thin client of serve's /api/fleet routes, built on the same
// serveClient `contenox approvals` introduced (serveclient.go), NOT a second
// access path. All lifecycle POLICY (the Enabled gate, teardown-on-failure,
// cancel fan-out) stays in fleetservice behind those routes; these verbs decode
// flags, call one route, and render — no orchestration is re-derived here.
//
// Output discipline, uniform with every other verb: stdout is data, diagnostics
// are stderr, `--json` is the machine contract (the raw route response), and a
// route error becomes a non-zero exit.
package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"text/tabwriter"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/spf13/cobra"
)

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Operate the fleet board: list units, inspect one, stop or cancel it.",
	Long: `Operate the fleet — the supervised agent units a running 'contenox serve'
is hosting — from the shell. These verbs act on UNITS (the running processes);
the WORK a unit was sent to do, and the reports it files back, live under
'contenox mission'.

The fleet is not in the database: it is a set of live subprocesses owned by the
serve process's in-memory Manager, so — unlike 'contenox state'/'session' —
these verbs reach serve over its REST API instead of opening the database. By
default that is http://127.0.0.1:32123; override with --server/--token or the
CONTENOX_SERVER_URL/CONTENOX_SERVER_TOKEN environment variables.

  contenox fleet list                       # the board: every declared agent + its live units
  contenox fleet list --json                # the raw /fleet records (declared agents + instances)
  contenox fleet show <instance-id>         # one unit's status (state, sessions, viewers)
  contenox fleet stop <instance-id>         # tear a unit down (idempotent)
  contenox fleet cancel <instance-id>       # cancel every in-flight turn on the unit
  contenox fleet cancel <instance-id> --session <session-id>   # cancel just that session

To FIRE a new unit at an intent, use 'contenox mission fire' — firing is a
mission, not a bare dispatch.`,
}

var fleetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the fleet board: every declared agent and its live units.",
	Long: `Print the fleet board — the config+runtime join a running 'contenox serve'
holds: every DECLARED agent (so a declared-but-idle agent is visible, not only
running ones) annotated with each live instance's kind, state, open-session and
viewer counts, and — the board's primary fact in mission mode — what the unit
was sent to do.

The INTENT column is joined in from the mission bound to each unit; it is "-"
for a unit with no mission behind it (an editor's ACP chat session, say) and for
a declared agent that is idle. --json emits the raw /fleet records (the machine
contract, without the mission join); read a unit's work as structured data with
'contenox mission list --json' instead.`,
	Args: cobra.NoArgs,
	RunE: runFleetList,
}

var fleetShowCmd = &cobra.Command{
	Use:   "show <instance-id>",
	Short: "Show one unit's status.",
	Long: `Print one instance's point-in-time status: its declared agent and kind, its
lifecycle state, when it started, how many downstream sessions are open on it,
how many viewers are attached, and the open session ids. An unknown instance id
fails with a non-zero exit status.`,
	Args: cobra.ExactArgs(1),
	RunE: runFleetShow,
}

var fleetStopCmd = &cobra.Command{
	Use:   "stop <instance-id>",
	Short: "Tear a unit down.",
	Long: `Stop an instance — tear its subprocess down and drop it from the board. Stop
is idempotent by the kernel's contract: stopping an unknown or already-stopped
instance is a success, not an error, so a script may call it without a
preceding existence check.`,
	Args: cobra.ExactArgs(1),
	RunE: runFleetStop,
}

var fleetCancelCmd = &cobra.Command{
	Use:   "cancel <instance-id>",
	Short: "Cancel a unit's in-flight prompt turn(s).",
	Long: `Cancel an in-flight prompt turn on an instance WITHOUT tearing the instance
down (for that, use 'contenox fleet stop'). With no --session, it fans out over
every session currently open on the unit and cancels each — "stop everything
running on this unit". With --session it cancels exactly that session, which is
safe even with no turn in flight. An unknown instance id fails with a non-zero
exit status.`,
	Args: cobra.ExactArgs(1),
	RunE: runFleetCancel,
}

func init() {
	// One set of --server/--token on the parent; every fleet subcommand inherits
	// it — the same shared helper `contenox approvals` registers (serveclient.go).
	addServeClientFlags(fleetCmd)

	fleetListCmd.Flags().Bool("json", false, "Print the raw /fleet records as JSON instead of the board table")
	fleetShowCmd.Flags().Bool("json", false, "Print the raw instance status as JSON instead of a summary")
	fleetCancelCmd.Flags().String("session", "", "Cancel only this session id (default: every session open on the unit)")

	fleetCmd.AddCommand(fleetListCmd)
	fleetCmd.AddCommand(fleetShowCmd)
	fleetCmd.AddCommand(fleetStopCmd)
	fleetCmd.AddCommand(fleetCancelCmd)
}

// ─── serveClient wrappers (fleet-shaped; the client itself is resource-agnostic) ───

// listFleet fetches GET /fleet — the config+runtime join of every declared
// agent annotated with its live instances. The result is always a non-nil
// slice so an empty fleet renders as an empty board rather than a nil deref.
func (c *serveClient) listFleet(ctx context.Context) ([]agentinstance.FleetEntry, error) {
	var out []agentinstance.FleetEntry
	if err := c.get(ctx, "/fleet", &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []agentinstance.FleetEntry{}
	}
	return out, nil
}

// getInstance fetches GET /fleet/{id}. A 404 comes back as a *ServeError
// carrying serve's own "instance not found" message, so an unknown id is
// distinguishable without re-parsing.
func (c *serveClient) getInstance(ctx context.Context, instanceID string) (*agentinstance.InstanceStatus, error) {
	var out agentinstance.InstanceStatus
	if err := c.get(ctx, "/fleet/"+url.PathEscape(instanceID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// stopInstance issues DELETE /fleet/{id}. Idempotent by kernel contract (200,
// not 404, for an unknown/already-stopped id).
func (c *serveClient) stopInstance(ctx context.Context, instanceID string) error {
	return c.delete(ctx, "/fleet/"+url.PathEscape(instanceID))
}

// cancelInstanceBody is the optional POST /fleet/{id}/cancel body — a tiny
// duplicate of fleetapi.CancelRequest's one field (the same way approvals
// duplicates its answer body) rather than importing the internal wire type. An
// empty SessionID cancels every session on the unit.
type cancelInstanceBody struct {
	SessionID string `json:"sessionId,omitempty"`
}

// cancelInstance posts a cancel for instanceID; sessionID empty means "every
// session on the unit". A 404 surfaces as a *ServeError.
func (c *serveClient) cancelInstance(ctx context.Context, instanceID, sessionID string) error {
	return c.post(ctx, "/fleet/"+url.PathEscape(instanceID)+"/cancel", cancelInstanceBody{SessionID: sessionID}, nil)
}

// ─── list ─────────────────────────────────────────────────────────────────

func runFleetList(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	entries, err := client.listFleet(ctx)
	if err != nil {
		return fmt.Errorf("list fleet: %w", err)
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		// The machine contract is the route's own shape (declared agents +
		// instances), NOT the intent-joined table — a script wanting a unit's
		// work reads `contenox mission list --json`, keeping the noun-split
		// intact even in JSON.
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no agents declared)")
		return nil
	}

	// Join the mission intent per unit for the board's primary column. This is
	// pure read-side PRESENTATION — a second GET and a map lookup, not
	// orchestration: the fleet route carries no intent (it is the mission's, not
	// the unit's), so the board reads it from the missions the units are bound
	// to. A missions fetch that fails must not blank the board — the units are
	// the load-bearing data — so its error only drops the intent column.
	intentByInstance := fleetMissionIntents(ctx, client)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tKIND\tSTATE\tINSTANCE\tSESSIONS\tVIEWERS\tINTENT")
	for _, e := range entries {
		if len(e.Instances) == 0 {
			// A declared-but-idle agent: no unit, so no per-instance facts and no
			// mission. Shown so the board reflects what CAN be fired, not only what
			// is running.
			fmt.Fprintf(w, "%s\t%s\tidle\t-\t-\t-\t-\n", e.AgentName, e.Kind)
			continue
		}
		for _, inst := range e.Instances {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
				e.AgentName,
				inst.Kind,
				inst.State,
				inst.ID,
				inst.Sessions,
				inst.Viewers,
				stringOrDash(intentByInstance[inst.ID]),
			)
		}
	}
	return w.Flush()
}

// fleetMissionIntents builds an instance-id -> mission-intent map for the board's
// INTENT column. It walks the mission list newest-first and keeps the FIRST
// intent seen per instance, matching missionservice.GetByInstance's "newest
// mission that claims the unit wins" resolution. Best-effort: any error (serve
// old enough to lack /missions, a transient failure) returns an empty map, and
// the board simply renders every intent as "-".
func fleetMissionIntents(ctx context.Context, client *serveClient) map[string]string {
	missions, err := client.listMissions(ctx, 0)
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(missions))
	for _, m := range missions {
		if m == nil || m.InstanceID == "" {
			continue
		}
		if _, seen := out[m.InstanceID]; !seen {
			out[m.InstanceID] = m.Intent
		}
	}
	return out
}

// ─── show ─────────────────────────────────────────────────────────────────

func runFleetShow(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	instanceID := args[0]

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	status, err := client.getInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("show instance %q: %w", instanceID, err)
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Instance:\t%s\n", status.ID)
	fmt.Fprintf(w, "Agent:\t%s\n", stringOrDash(status.AgentName))
	fmt.Fprintf(w, "Kind:\t%s\n", stringOrDash(status.Kind))
	fmt.Fprintf(w, "State:\t%s\n", stringOrDash(status.State))
	fmt.Fprintf(w, "Started:\t%s\n", formatApprovalTime(status.StartedAt))
	fmt.Fprintf(w, "Sessions:\t%d\n", status.Sessions)
	fmt.Fprintf(w, "Viewers:\t%d\n", status.Viewers)
	if len(status.SessionIDs) == 0 {
		fmt.Fprintf(w, "Session IDs:\t-\n")
	} else {
		for i, sid := range status.SessionIDs {
			label := ""
			if i == 0 {
				label = "Session IDs:"
			}
			fmt.Fprintf(w, "%s\t%s\n", label, sid)
		}
	}
	return w.Flush()
}

// ─── stop ─────────────────────────────────────────────────────────────────

func runFleetStop(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	instanceID := args[0]

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	if err := client.stopInstance(ctx, instanceID); err != nil {
		return fmt.Errorf("stop instance %q: %w", instanceID, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Instance %s stopped.\n", instanceID)
	return nil
}

// ─── cancel ───────────────────────────────────────────────────────────────

func runFleetCancel(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	instanceID := args[0]
	sessionID, _ := cmd.Flags().GetString("session")

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	if err := client.cancelInstance(ctx, instanceID, sessionID); err != nil {
		return fmt.Errorf("cancel instance %q: %w", instanceID, err)
	}
	if sessionID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Cancelled session %s on instance %s.\n", sessionID, instanceID)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Cancelled every in-flight turn on instance %s.\n", instanceID)
	}
	return nil
}
