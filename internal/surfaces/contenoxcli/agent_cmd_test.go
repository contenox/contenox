package contenoxcli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/libsandbox"
	"github.com/contenox/contenox/internal/services/agentregistryservice"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// agentTestRoot builds an isolated root command carrying the persistent --db
// flag (as the real rootCmd does) with sub attached, so tests exercise the real
// RunE logic without touching the package-global rootCmd's flag state.
func agentTestRoot(sub *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "contenox", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("db", "", "SQLite database path")
	root.AddCommand(sub)
	return root
}

func openServiceAt(t *testing.T, dbPath string) (context.Context, agentregistryservice.Service, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDBAt(ctx, dbPath)
	require.NoError(t, err)
	return ctx, agentregistryservice.New(db), func() { _ = db.Close() }
}

// seedChainAgent persists a chain-kind agent (the runtime's only agent kind)
// so the inspect/toggle commands have something to operate on.
func seedChainAgent(t *testing.T, dbPath, name, path string) {
	t.Helper()
	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	a := &runtimetypes.Agent{Name: name, Enabled: true}
	require.NoError(t, a.SetChainConfig(runtimetypes.ChainConfig{Path: path}))
	source := runtimetypes.AgentSourceDiscovered
	a.Source = &source
	require.NoError(t, svc.Create(ctx, a))
}

func TestUnit_agentIsReservedSubcommand(t *testing.T) {
	require.True(t, reservedSubcommands["agent"], `"agent" must be reserved so it dispatches as a subcommand`)
	require.True(t, firstNonFlagIsReserved([]string{"agent", "list"}))
	require.True(t, firstNonFlagIsReserved([]string{"--db", "/tmp/x", "agent", "show"}))
}

func newAgentAddTestCmd() *cobra.Command {
	return &cobra.Command{Use: "add", Args: cobra.MinimumNArgs(1), RunE: runAgentAdd}
}

func TestUnit_AgentAdd_ManualRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAgentAddTestCmd())
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--db", dbPath, "add", "external-reviewer", "--", "review-agent", "--stdio"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	agent, err := svc.GetByName(ctx, "external-reviewer")
	require.NoError(t, err)
	require.Equal(t, runtimetypes.AgentKindExternalACP, agent.Kind)
	require.Equal(t, runtimetypes.AgentSourceManual, derefOr(agent.Source, ""))
	cfg, err := agent.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "review-agent", cfg.Command)
	require.Equal(t, []string{"--stdio"}, cfg.Args)
	require.Contains(t, output.String(), "review-agent --stdio")
}

func TestUnit_AgentAdd_RequiresCommandAfterSeparator(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAgentAddTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "add", "external-reviewer", "--"})
	require.ErrorContains(t, root.Execute(), "provide a command")
}

func newAgentEditTestCmd() *cobra.Command {
	command := &cobra.Command{Use: "edit", Args: cobra.ExactArgs(1), RunE: runAgentEdit}
	command.Flags().String("config-file", "", "")
	return command
}

func TestUnit_AgentEdit_ConfigFromStdin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "external-reviewer", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "old-agent",
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newAgentEditTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(`{"transport":"stdio","command":"new-agent","args":["--acp"],"mcp_servers":["docs"]}`))
	root.SetArgs([]string{"--db", dbPath, "edit", "external-reviewer", "--config-file", "-"})
	require.NoError(t, root.Execute())

	ctx, svc, done = openServiceAt(t, dbPath)
	defer done()
	agent, err := svc.GetByName(ctx, "external-reviewer")
	require.NoError(t, err)
	cfg, err := agent.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "new-agent", cfg.Command)
	require.Equal(t, []string{"--acp"}, cfg.Args)
	require.Equal(t, []string{"docs"}, cfg.McpServers)
}

func TestUnit_AgentEdit_RefusesDeclaredAgent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedChainAgent(t, dbPath, "reviewer", "/home/user/.contenox/agent-reviewer.json")
	root := agentTestRoot(newAgentEditTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(`{"transport":"stdio","command":"replace-it"}`))
	root.SetArgs([]string{"--db", dbPath, "edit", "reviewer", "--config-file", "-"})
	require.ErrorContains(t, root.Execute(), "declaration file is the editable source")
}

func TestUnit_AgentEdit_InvalidConfigLeavesStoredConfigUnchanged(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "external-reviewer", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "old-agent",
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newAgentEditTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(`{"transport":"stdio"}`))
	root.SetArgs([]string{"--db", dbPath, "edit", "external-reviewer", "--config-file", "-"})
	require.ErrorContains(t, root.Execute(), "command is required")

	ctx, svc, done = openServiceAt(t, dbPath)
	defer done()
	agent, err := svc.GetByName(ctx, "external-reviewer")
	require.NoError(t, err)
	cfg, err := agent.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "old-agent", cfg.Command)
}

func newAgentCheckTestCmd() *cobra.Command {
	command := &cobra.Command{Use: "check", Args: cobra.MinimumNArgs(1), RunE: runAgentCheck}
	command.Flags().Duration("timeout", time.Minute, "")
	return command
}

func TestUnit_AgentCheck_UnknownAgentFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAgentCheckTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "check", "missing"})
	require.ErrorContains(t, root.Execute(), `agent "missing" not found`)
}

func TestUnit_AgentCheck_MissingMCPServerFailsBeforeSpawn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "external-reviewer", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport:  runtimetypes.ExternalACPTransportStdio,
		Command:    "must-not-run",
		McpServers: []string{"missing"},
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newAgentCheckTestCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "check", "external-reviewer"})
	require.ErrorContains(t, root.Execute(), "missing")
}

func TestUnit_AgentList_And_Show(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedChainAgent(t, dbPath, "shown-agent", "/home/user/.contenox/agent-reviewer.json")

	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: agentListCmd.RunE}
	root := agentTestRoot(list)
	var listBuf bytes.Buffer
	root.SetOut(&listBuf)
	root.SetErr(&listBuf)
	root.SetArgs([]string{"--db", dbPath, "list"})
	require.NoError(t, root.Execute())
	require.Contains(t, listBuf.String(), "shown-agent")
	require.Contains(t, listBuf.String(), runtimetypes.AgentKindChain)

	show := &cobra.Command{Use: "show", Args: cobra.ExactArgs(1), RunE: agentShowCmd.RunE}
	rootShow := agentTestRoot(show)
	var showBuf bytes.Buffer
	rootShow.SetOut(&showBuf)
	rootShow.SetErr(&showBuf)
	rootShow.SetArgs([]string{"--db", dbPath, "show", "shown-agent"})
	require.NoError(t, rootShow.Execute())
	out := showBuf.String()
	require.Contains(t, out, "/home/user/.contenox/agent-reviewer.json")
	require.Contains(t, out, "config_json")
}

func TestUnit_AgentRemove(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedChainAgent(t, dbPath, "gone-agent", "/home/user/.contenox/x.json")

	rm := &cobra.Command{Use: "remove", Args: cobra.ExactArgs(1), RunE: agentRemoveCmd.RunE}
	root := agentTestRoot(rm)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "remove", "gone-agent"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	_, err := svc.GetByName(ctx, "gone-agent")
	require.Error(t, err)
}

func TestUnit_AgentEnableDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedChainAgent(t, dbPath, "toggle-agent", "/home/user/.contenox/y.json")

	disable := &cobra.Command{Use: "disable", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return setAgentEnabled(cmd, args[0], false) }}
	root := agentTestRoot(disable)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "disable", "toggle-agent"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	got, err := svc.GetByName(ctx, "toggle-agent")
	require.NoError(t, err)
	require.False(t, got.Enabled)
	done()
}

func TestUnit_AgentHelpers(t *testing.T) {
	require.Equal(t, "goose acp", renderAgentCommand("goose", []string{"acp"}))
	require.Equal(t, "goose", renderAgentCommand("goose", nil))
	require.Equal(t, "-", derefOr(nil, "-"))
	s := "discovered"
	require.Equal(t, "discovered", derefOr(&s, "-"))
	empty := ""
	require.Equal(t, "fallback", derefOr(&empty, "fallback"))

	require.Equal(t, "enabled", enabledWord(true))
	require.Equal(t, "disabled", enabledWord(false))

	pretty, err := prettyJSON([]byte(`{"a":1}`))
	require.NoError(t, err)
	require.Contains(t, pretty, "\n  \"a\": 1")
	pretty2, err := prettyJSON(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", pretty2)
}

func TestSystem_AgentAddAndCheck_DrivesACPStub(t *testing.T) {
	if err := libsandbox.Preflight(); err != nil {
		t.Skipf("external ACP agents require the sandbox: %v", err)
	}
	workdir := t.TempDir()
	cli := filepath.Join(workdir, "contenox")
	stub := filepath.Join(workdir, "acp-stub-agent")
	build := func(target, pkg string) {
		t.Helper()
		command := exec.Command("go", "build", "-o", target, pkg)
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	build(cli, "github.com/contenox/contenox/cmd/contenox")
	build(stub, "github.com/contenox/contenox/libacp/cmd/acp-stub-agent")

	dbPath := filepath.Join(workdir, "agents.db")
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command(cli, append([]string{"--db", dbPath}, args...)...)
		command.Dir = workdir
		command.Env = append(os.Environ(), "HOME="+workdir)
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return string(output)
	}
	run("agent", "add", "stub", "--", stub)
	output := run("agent", "check", "stub", "reply briefly")
	require.Contains(t, output, `Checking agent "stub"`)
	require.Contains(t, output, "ack")
	require.Contains(t, output, "stopReason=end_turn")
}
