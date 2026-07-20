package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/spf13/cobra"
)

var approvalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Read and answer pending human-in-the-loop approvals.",
	Long: `List and answer the human-in-the-loop approvals a running 'contenox serve'
is holding pending — the inbox for asks raised by an agent working with no
attached session (dispatched fleet work, a headless API caller). A permission
request that would otherwise hang until its policy-rule timeout, or the
serve-level ceiling, is answerable here as soon as it lands.

Unlike 'contenox state'/'session', which open the local database directly,
a pending approval is a goroutine parked inside a running 'contenox serve'
process — answering it has to reach that process, not just its database. So
this command talks to serve's REST API over HTTP instead: by default
http://127.0.0.1:32123, overridable with --server/--token or the
CONTENOX_SERVER_URL/CONTENOX_SERVER_TOKEN environment variables.

Examples:
  contenox approvals list
  contenox approvals list --json
  contenox approvals answer 3f9c6e2a-1b4d --approve
  contenox approvals answer 3f9c6e2a-1b4d --deny`,
}

var approvalsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending approvals, newest first.",
	Long: `Fetch the pending human-in-the-loop approvals from a running 'contenox serve'
and print them as a table of id, tool, args summary, policy, matched rule,
diff presence, the agent/mission/instance/session the ask came from, and the
created/expires timestamps — everything an operator needs to decide, and to
always be able to name both which policy rule escalated the ask and which unit
it is holding up. The attribution columns are empty ("-") for an ask raised
outside the fleet, and the mission column is empty for an unattended session
that is not on a mission.`,
	Args: cobra.NoArgs,
	RunE: runApprovalsList,
}

var approvalsAnswerCmd = &cobra.Command{
	Use:   "answer <id> --approve|--deny",
	Short: "Approve or deny a pending approval.",
	Long: `Answer one pending approval by id (see 'contenox approvals list'). Exactly one
of --approve/--deny is required.

An id that is unknown, already answered, or expired (past its policy's
timeout and auto-resolved by serve's sweeper) fails with a non-zero exit
status describing which of those it was — answering twice is never silently
a no-op.`,
	Args: cobra.ExactArgs(1),
	RunE: runApprovalsAnswer,
}

func init() {
	addServeClientFlags(approvalsCmd)

	approvalsListCmd.Flags().Int("limit", 100, "Maximum number of pending approvals to list (0 = server default cap)")
	approvalsListCmd.Flags().Bool("json", false, "Print the raw records as JSON instead of a table")

	approvalsAnswerCmd.Flags().Bool("approve", false, "Approve the pending ask")
	approvalsAnswerCmd.Flags().Bool("deny", false, "Deny the pending ask")
	approvalsAnswerCmd.Flags().Bool("json", false, "Print the result as JSON")
	approvalsAnswerCmd.MarkFlagsMutuallyExclusive("approve", "deny")
	approvalsAnswerCmd.MarkFlagsOneRequired("approve", "deny")

	approvalsCmd.AddCommand(approvalsListCmd)
	approvalsCmd.AddCommand(approvalsAnswerCmd)
}

// ─── serveClient convenience wrappers (approvals-shaped; the client itself,
// in serveclient.go, is not) ─────────────────────────────────────────────

// listPendingApprovals fetches GET /api/approvals?limit=<limit>. limit<=0 is
// forwarded as-is (the server's own "no cap given" default applies, mirroring
// hitlservice.Service.ListPending / runtimetypes.ListHITLApprovals's own
// contract), and the result is always a non-nil slice.
func (c *serveClient) listPendingApprovals(ctx context.Context, limit int) ([]*runtimetypes.HITLApproval, error) {
	path := "/approvals?limit=" + strconv.Itoa(limit)
	var out []*runtimetypes.HITLApproval
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []*runtimetypes.HITLApproval{}
	}
	return out, nil
}

// answerApprovalRequest is the POST /api/approvals/{id} body — kept as a
// tiny duplicate of approvalapi.AnswerRequest's one field rather than an
// import of that internal package, the same way any other JSON API client
// would encode the wire contract without depending on the server's Go types.
type answerApprovalRequest struct {
	Approved bool `json:"approved"`
}

// answerApproval posts an answer for id. A non-2xx response comes back as a
// *ServeError (see serveclient.go) carrying serve's own status and message,
// so the id-not-found/already-resolved/expired cases are distinguishable by
// the caller without re-parsing anything.
func (c *serveClient) answerApproval(ctx context.Context, id string, approved bool) error {
	return c.post(ctx, "/approvals/"+url.PathEscape(id), answerApprovalRequest{Approved: approved}, nil)
}

// ─── list ───────────────────────────────────────────────────────────────

func runApprovalsList(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt("limit")
	approvals, err := client.listPendingApprovals(ctx, limit)
	if err != nil {
		return fmt.Errorf("list approvals: %w", err)
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(approvals)
	}

	if len(approvals) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no pending approvals)")
		return nil
	}

	// AGENT/MISSION/INSTANCE/SESSION are the attribution columns: with more than
	// one unit running, "write_file" identifies nothing, and the row has to say
	// WHOSE action is being gated. They are empty for an ask raised by a native
	// chain turn with no fleet unit behind it, which renders as "-".
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTOOL\tARGS\tPOLICY\tRULE\tDIFF\tAGENT\tMISSION\tINSTANCE\tSESSION\tCREATED\tEXPIRES")
	for _, a := range approvals {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			a.ID,
			a.ToolsName+"."+a.ToolName,
			stringOrDash(a.ArgsSummary),
			stringOrDash(a.PolicyName),
			intPtrOrDash(a.MatchedRule),
			approvalDiffColumn(a.Diff),
			stringOrDash(a.AgentName),
			stringPtrOrDash(a.MissionID),
			stringOrDash(a.InstanceID),
			stringOrDash(a.SessionID),
			formatApprovalTime(a.CreatedAt),
			formatApprovalTime(a.ExpiresAt),
		)
	}
	return w.Flush()
}

// ─── answer ─────────────────────────────────────────────────────────────

func runApprovalsAnswer(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	id := args[0]

	// --approve/--deny are enforced mutually exclusive and one-required by
	// cobra (MarkFlagsMutuallyExclusive/MarkFlagsOneRequired in init) before
	// RunE ever runs, so reading --approve alone is enough to know which was
	// given.
	approved, _ := cmd.Flags().GetBool("approve")

	client, err := newServeClient(cmd)
	if err != nil {
		return err
	}

	if err := client.answerApproval(ctx, id, approved); err != nil {
		return fmt.Errorf("answer approval %q: %w", id, err)
	}

	verb := "denied"
	if approved {
		verb = "approved"
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"id": id, "approved": approved})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Approval %s %s.\n", id, verb)
	return nil
}

// ─── table helpers ──────────────────────────────────────────────────────

func stringOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// stringPtrOrDash renders a nullable attribution column (today: mission id),
// where nil means the ask genuinely has no mission behind it rather than one
// that could not be looked up — see runtimetypes.HITLApproval.MissionID.
func stringPtrOrDash(v *string) string {
	if v == nil {
		return "-"
	}
	return stringOrDash(*v)
}

// intPtrOrDash renders a matched-rule index, or "-" when nil — which means
// the policy's default_action applied rather than a named rule (see
// runtimetypes.HITLApproval.MatchedRule's doc).
func intPtrOrDash(v *int) string {
	if v == nil {
		return "-"
	}
	return strconv.Itoa(*v)
}

// approvalDiffColumn keeps the table's columns aligned even when a diff is
// large or multi-line: it reports presence and size rather than the diff
// text itself. Use --json to read the full diff content.
func approvalDiffColumn(diff *string) string {
	if diff == nil || strings.TrimSpace(*diff) == "" {
		return "-"
	}
	lines := strings.Count(*diff, "\n") + 1
	return fmt.Sprintf("yes (%d lines)", lines)
}

func formatApprovalTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
