package contenoxcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/approvalapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// ─── test scaffolding: a real hitlservice+approvalapi behind httptest,
// mounted at "/api" exactly like `contenox serve` mounts it (server.go's
// registerProductRoutes + serve_cmd.go's rootMux.Handle("/api/",
// http.StripPrefix("/api", apiMux))). This exercises serveClient against the
// real wire contract without spawning a `contenox serve` process. ───────────

func setupApprovalsTestServer(t *testing.T) (context.Context, runtimetypes.Store, hitlservice.Service, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "approvals.db")
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store := runtimetypes.New(db.WithoutTransaction())
	svc := hitlservice.NewWithDefaultPolicy(hitlservice.NewFSPolicySource(t.TempDir()), runtimetypes.LocalTenantID, store, libtracker.NoopTracker{}, "")

	apiMux := http.NewServeMux()
	approvalapi.AddRoutes(apiMux, svc)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))

	srv := httptest.NewServer(rootMux)
	t.Cleanup(srv.Close)
	return ctx, store, svc, srv
}

func seedApproval(t *testing.T, ctx context.Context, store runtimetypes.Store, id string, createdAt time.Time) *runtimetypes.HITLApproval {
	t.Helper()
	return seedApprovalWithExpiry(t, ctx, store, id, createdAt, createdAt.Add(time.Hour))
}

func seedApprovalWithExpiry(t *testing.T, ctx context.Context, store runtimetypes.Store, id string, createdAt, expiresAt time.Time) *runtimetypes.HITLApproval {
	t.Helper()
	diff := "--- a\n+++ b\n"
	rule := 1
	row := &runtimetypes.HITLApproval{
		ID:          id,
		ToolsName:   "local_fs",
		ToolName:    "write_file",
		ArgsSummary: "/workspace/main.go",
		Diff:        &diff,
		PolicyName:  "hitl-policy-default.json",
		MatchedRule: &rule,
		OnTimeout:   "deny",
		State:       runtimetypes.HITLApprovalPending,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	}
	require.NoError(t, store.CreateHITLApproval(ctx, row))
	return row
}

func newApprovalsListTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: runApprovalsList}
	addServeClientFlags(c)
	c.Flags().Int("limit", 100, "")
	c.Flags().Bool("json", false, "")
	return c
}

func newApprovalsAnswerTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "answer", Args: cobra.ExactArgs(1), RunE: runApprovalsAnswer}
	addServeClientFlags(c)
	c.Flags().Bool("approve", false, "")
	c.Flags().Bool("deny", false, "")
	c.Flags().Bool("json", false, "")
	c.MarkFlagsMutuallyExclusive("approve", "deny")
	c.MarkFlagsOneRequired("approve", "deny")
	return c
}

// ─── serveClient ────────────────────────────────────────────────────────

func TestUnit_NewServeClient_DefaultsToServeAddrPort(t *testing.T) {
	c := newApprovalsListTestCmd()
	c.SetArgs([]string{})
	require.NoError(t, c.ParseFlags(nil))

	client, err := newServeClient(c)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:32123", client.baseURL)
	require.Empty(t, client.token)
}

func TestUnit_NewServeClient_ServerFlagOverridesDefault(t *testing.T) {
	c := newApprovalsListTestCmd()
	require.NoError(t, c.ParseFlags([]string{"--server", "http://example.local:9000/", "--token", "flag-token"}))

	client, err := newServeClient(c)
	require.NoError(t, err)
	require.Equal(t, "http://example.local:9000", client.baseURL, "a trailing slash must be trimmed")
	require.Equal(t, "flag-token", client.token)
}

func TestUnit_NewServeClient_EnvVarsUsedWhenFlagsAbsent(t *testing.T) {
	t.Setenv(envServeURL, "http://from-env:1234")
	t.Setenv(envServeToken, "env-token")

	c := newApprovalsListTestCmd()
	require.NoError(t, c.ParseFlags(nil))

	client, err := newServeClient(c)
	require.NoError(t, err)
	require.Equal(t, "http://from-env:1234", client.baseURL)
	require.Equal(t, "env-token", client.token)
}

func TestUnit_NewServeClient_FlagWinsOverEnv(t *testing.T) {
	t.Setenv(envServeURL, "http://from-env:1234")

	c := newApprovalsListTestCmd()
	require.NoError(t, c.ParseFlags([]string{"--server", "http://from-flag:5678"}))

	client, err := newServeClient(c)
	require.NoError(t, err)
	require.Equal(t, "http://from-flag:5678", client.baseURL)
}

func TestUnit_NewServeClient_InvalidServerURLErrors(t *testing.T) {
	c := newApprovalsListTestCmd()
	require.NoError(t, c.ParseFlags([]string{"--server", "not a url"}))

	_, err := newServeClient(c)
	require.Error(t, err)
}

func TestUnit_ServeClient_DoDecodesJSONAndSendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.Equal(t, "/api/ping", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &serveClient{baseURL: srv.URL, token: "sekret", http: srv.Client()}
	var out struct {
		OK bool `json:"ok"`
	}
	require.NoError(t, client.get(context.Background(), "/ping", &out))
	require.True(t, out.OK)
	require.Equal(t, "Bearer sekret", gotAuth)
}

func TestUnit_ServeClient_DoMapsNon2xxToServeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"message":"already answered","type":"invalid_request_error","code":"conflict"}}`))
	}))
	defer srv.Close()

	client := &serveClient{baseURL: srv.URL, http: srv.Client()}
	err := client.get(context.Background(), "/x", nil)
	require.Error(t, err)

	var serveErr *ServeError
	require.ErrorAs(t, err, &serveErr)
	require.Equal(t, http.StatusConflict, serveErr.StatusCode)
	require.Equal(t, "already answered", serveErr.Message)
}

func TestUnit_ServeClient_DoUnreachableServerReportsConnectError(t *testing.T) {
	client := &serveClient{baseURL: "http://127.0.0.1:1", http: &http.Client{Timeout: 2 * time.Second}}
	err := client.get(context.Background(), "/x", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "contenox serve")
}

// ─── list ───────────────────────────────────────────────────────────────

func TestUnit_ApprovalsList_EmptyInboxPrintsHint(t *testing.T) {
	_, _, _, srv := setupApprovalsTestServer(t)

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "(no pending approvals)\n", buf.String())
}

func TestUnit_ApprovalsList_TableShowsDecisionFields(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-1", time.Now().UTC())

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	for _, want := range []string{
		"appr-1",
		"local_fs.write_file",
		"/workspace/main.go",
		"hitl-policy-default.json",
		"yes (3 lines)", // the diff column: presence + size, not the raw diff text
	} {
		require.Contains(t, out, want)
	}
}

func TestUnit_ApprovalsList_NewestFirst(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	base := time.Now().UTC().Add(-time.Hour)
	seedApproval(t, ctx, store, "older", base)
	seedApproval(t, ctx, store, "newer", base.Add(time.Minute))

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	require.Less(t, strings.Index(out, "newer"), strings.Index(out, "older"), "newest-first ordering must reach the CLI table")
}

func TestUnit_ApprovalsList_JSONEmitsRawRecords(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-json", time.Now().UTC())

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--json"})
	require.NoError(t, cmd.Execute())

	var got []*runtimetypes.HITLApproval
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	require.Equal(t, "appr-json", got[0].ID)
	require.NotNil(t, got[0].Diff, "--json must carry the full diff, unlike the table's presence-only column")
}

func TestUnit_ApprovalsList_JSONEmptyInboxIsEmptyArrayNotHintText(t *testing.T) {
	_, _, _, srv := setupApprovalsTestServer(t)

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--json"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "[]\n", buf.String())
}

func TestUnit_ApprovalsList_UnreachableServerFailsWithNonZeroExit(t *testing.T) {
	cmd := newApprovalsListTestCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", "http://127.0.0.1:1"})
	err := cmd.Execute()
	require.Error(t, err)
}

// ─── answer ─────────────────────────────────────────────────────────────

func TestUnit_ApprovalsAnswer_ApproveResolvesTheRowAndPrintsConfirmation(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-approve", time.Now().UTC())

	cmd := newApprovalsAnswerTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--approve", "appr-approve"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "Approval appr-approve approved.\n", buf.String())

	got, err := store.GetHITLApproval(ctx, "appr-approve")
	require.NoError(t, err)
	require.Equal(t, runtimetypes.HITLApprovalApproved, got.State)
}

func TestUnit_ApprovalsAnswer_DenyResolvesTheRowAndPrintsConfirmation(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-deny", time.Now().UTC())

	cmd := newApprovalsAnswerTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--deny", "appr-deny"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "Approval appr-deny denied.\n", buf.String())

	got, err := store.GetHITLApproval(ctx, "appr-deny")
	require.NoError(t, err)
	require.Equal(t, runtimetypes.HITLApprovalDenied, got.State)
}

func TestUnit_ApprovalsAnswer_JSONOutput(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-json-answer", time.Now().UTC())

	cmd := newApprovalsAnswerTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--approve", "--json", "appr-json-answer"})
	require.NoError(t, cmd.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, "appr-json-answer", got["id"])
	require.Equal(t, true, got["approved"])
}

func TestUnit_ApprovalsAnswer_UnknownIDFailsNonZero(t *testing.T) {
	_, _, _, srv := setupApprovalsTestServer(t)

	cmd := newApprovalsAnswerTestCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--approve", "no-such-id"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestUnit_ApprovalsAnswer_AlreadyResolvedFailsNonZeroAndSaysSo(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-twice", time.Now().UTC())

	first := newApprovalsAnswerTestCmd()
	first.SetOut(&bytes.Buffer{})
	first.SetArgs([]string{"--server", srv.URL, "--approve", "appr-twice"})
	require.NoError(t, first.Execute())

	second := newApprovalsAnswerTestCmd()
	second.SilenceUsage, second.SilenceErrors = true, true
	var buf bytes.Buffer
	second.SetOut(&buf)
	second.SetErr(&buf)
	second.SetArgs([]string{"--server", srv.URL, "--deny", "appr-twice"})
	err := second.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already answered")

	// The first answer must stand.
	got, getErr := store.GetHITLApproval(ctx, "appr-twice")
	require.NoError(t, getErr)
	require.Equal(t, runtimetypes.HITLApprovalApproved, got.State)
}

// TestUnit_ApprovalsAnswer_ExpiredFailsNonZeroAndSaysSo seeds a row whose
// deadline has already passed, sweeps it (exactly what
// serve_cmd.go's startHITLApprovalSweeper does on its own tick) so it is
// already in the terminal 'expired' state, and then proves `contenox
// approvals answer` reports that distinctly rather than as a generic
// failure or a false success.
func TestUnit_ApprovalsAnswer_ExpiredFailsNonZeroAndSaysSo(t *testing.T) {
	ctx, store, svc, srv := setupApprovalsTestServer(t)
	past := time.Now().UTC().Add(-2 * time.Hour)
	seedApprovalWithExpiry(t, ctx, store, "appr-expired", past, past.Add(time.Hour))

	n, err := svc.SweepExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	cmd := newApprovalsAnswerTestCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL, "--approve", "appr-expired"})
	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

func TestUnit_ApprovalsAnswer_RequiresApproveOrDeny(t *testing.T) {
	cmd := newApprovalsAnswerTestCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--server", "http://127.0.0.1:1", "some-id"})
	err := cmd.Execute()
	require.Error(t, err)
}

func TestUnit_ApprovalsAnswer_RejectsBothApproveAndDeny(t *testing.T) {
	cmd := newApprovalsAnswerTestCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--server", "http://127.0.0.1:1", "--approve", "--deny", "some-id"})
	err := cmd.Execute()
	require.Error(t, err)
}

// ─── table helpers (pure) ───────────────────────────────────────────────

func TestUnit_ApprovalsTableHelpers(t *testing.T) {
	require.Equal(t, "-", stringOrDash(""))
	require.Equal(t, "x", stringOrDash("x"))

	require.Equal(t, "-", intPtrOrDash(nil))
	rule := 5
	require.Equal(t, "5", intPtrOrDash(&rule))

	require.Equal(t, "-", approvalDiffColumn(nil))
	empty := "   "
	require.Equal(t, "-", approvalDiffColumn(&empty))
	oneLine := "just one line"
	require.Equal(t, "yes (1 lines)", approvalDiffColumn(&oneLine))
	threeLines := "a\nb\nc"
	require.Equal(t, "yes (3 lines)", approvalDiffColumn(&threeLines))

	require.Equal(t, "-", formatApprovalTime(time.Time{}))
}

// ─── attribution columns ───────────────────────────────────────────────────

// An inbox of one can get away with naming only the tool. An inbox of many
// cannot: two units doing the same thing produce identical rows, and the
// operator cannot tell which one is being held up. These pin that the
// attribution reaches the table, and that an ask raised outside the fleet
// renders as absent rather than as an empty column that looks like a bug.

func TestUnit_ApprovalsList_TableShowsAttribution(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	row := seedApproval(t, ctx, store, "appr-attributed", time.Now().UTC())
	require.NoError(t, store.ResolveHITLApproval(ctx, row.ID, runtimetypes.HITLApprovalDenied, nil, time.Now().UTC()))

	missionID := "mission-77"
	attributed := &runtimetypes.HITLApproval{
		ID:          "appr-fleet",
		ToolsName:   "local_fs",
		ToolName:    "write_file",
		ArgsSummary: "/workspace/main.go",
		PolicyName:  "envelope.json",
		OnTimeout:   "deny",
		State:       runtimetypes.HITLApprovalPending,
		InstanceID:  "instance-77",
		SessionID:   "session-77",
		AgentName:   "reviewer",
		MissionID:   &missionID,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, store.CreateHITLApproval(ctx, attributed))

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	for _, want := range []string{
		"AGENT", "MISSION", "INSTANCE", "SESSION",
		"reviewer", "mission-77", "instance-77", "session-77",
	} {
		require.Contains(t, out, want)
	}
}

func TestUnit_ApprovalsList_UnattributedRowRendersDashes(t *testing.T) {
	ctx, store, _, srv := setupApprovalsTestServer(t)
	seedApproval(t, ctx, store, "appr-native", time.Now().UTC())

	cmd := newApprovalsListTestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--server", srv.URL})
	require.NoError(t, cmd.Execute())

	// An ask raised by a native chain turn has no unit behind it: the columns
	// exist and read as absent, which is a different statement from "unknown".
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)

	header := splitTableRow(lines[0])
	row := splitTableRow(lines[1])
	require.Len(t, row, len(header), "every header column must have a cell: %q", lines[1])
	for _, col := range []string{"AGENT", "MISSION", "INSTANCE", "SESSION"} {
		idx := indexOfColumn(t, header, col)
		require.Equal(t, "-", row[idx], "column %s must render as absent, got %q", col, row[idx])
	}
}

// splitTableRow splits one tabwriter-padded line into its cells. Columns are
// separated by two-or-more spaces, which keeps single-space cell contents (a
// timestamp, "yes (3 lines)") intact.
func splitTableRow(line string) []string {
	return tableCellSplitter.Split(strings.TrimSpace(line), -1)
}

var tableCellSplitter = regexp.MustCompile(` {2,}`)

func indexOfColumn(t *testing.T, header []string, name string) int {
	t.Helper()
	for i, h := range header {
		if h == name {
			return i
		}
	}
	t.Fatalf("column %q not found in header %v", name, header)
	return -1
}

func TestUnit_StringPtrOrDash(t *testing.T) {
	require.Equal(t, "-", stringPtrOrDash(nil))
	empty := ""
	require.Equal(t, "-", stringPtrOrDash(&empty))
	set := "mission-1"
	require.Equal(t, "mission-1", stringPtrOrDash(&set))
}
