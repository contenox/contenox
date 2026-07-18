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

	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/mcpserverservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// miniCatalog is a tiny but shape-accurate ACP registry catalog used to drive
// `agent add <registry-id>` from the local cache (no network). claude-acp is
// npx so it resolves identically on any test host; goose is a binary entry.
const miniCatalog = `{
  "version": "1.0.0",
  "agents": [
    {"id":"claude-acp","name":"Claude Agent","version":"0.59.0","description":"ACP wrapper for Claude",
     "distribution":{"npx":{"package":"@agentclientprotocol/claude-agent-acp@0.59.0"}}},
    {"id":"goose","name":"goose","version":"1.43.0","description":"local extensible agent",
     "distribution":{"binary":{"linux-x86_64":{"archive":"http://example/goose.tar","cmd":"./goose","args":["acp"]}}}}
  ]
}`

// agentTestRoot builds an isolated root command carrying the persistent --db
// flag (as the real rootCmd does) with sub attached, so tests exercise the real
// RunE logic without touching the package-global rootCmd's flag state.
func agentTestRoot(sub *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "contenox", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("db", "", "SQLite database path")
	root.AddCommand(sub)
	return root
}

func newAddCmd() *cobra.Command {
	c := &cobra.Command{Use: "add", Args: cobra.MinimumNArgs(1), RunE: runAgentAdd}
	c.Flags().String("name", "", "")
	c.Flags().Bool("refresh", false, "")
	return c
}

func openServiceAt(t *testing.T, dbPath string) (context.Context, agentregistryservice.Service, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDBAt(ctx, dbPath)
	require.NoError(t, err)
	return ctx, agentregistryservice.New(db), func() { _ = db.Close() }
}

// ─── dispatch / reservation ─────────────────────────────────────────────────

func TestUnit_agentIsReservedSubcommand(t *testing.T) {
	require.True(t, reservedSubcommands["agent"], `"agent" must be reserved so it dispatches as a subcommand`)
	require.True(t, firstNonFlagIsReserved([]string{"agent", "list"}))
	require.True(t, firstNonFlagIsReserved([]string{"--db", "/tmp/x", "agent", "search"}))
}

// ─── manual add ─────────────────────────────────────────────────────────────

func TestUnit_AgentAdd_Manual_RoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")

	root := agentTestRoot(newAddCmd())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "add", "local-bot", "--", "/usr/local/bin/my-acp-agent", "--stdio", "--flag"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()

	got, err := svc.GetByName(ctx, "local-bot")
	require.NoError(t, err)
	require.Equal(t, agentSourceManual, derefOr(got.Source, ""))
	require.Nil(t, got.RegistryID)
	require.Equal(t, runtimetypes.AgentKindExternalACP, got.Kind)

	cfg, err := got.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, runtimetypes.ExternalACPTransportStdio, cfg.Transport)
	require.Equal(t, "/usr/local/bin/my-acp-agent", cfg.Command)
	require.Equal(t, []string{"--stdio", "--flag"}, cfg.Args)
}

func TestUnit_AgentAdd_Manual_NoCommandAfterDash_Errors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAddCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "add", "local-bot", "--"})
	require.Error(t, root.Execute(), "'add <name> --' with no command must be rejected")
}

func TestUnit_AgentAdd_Registry_TooManyPositionals_Errors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAddCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	// No '--' → registry form, which takes exactly one id.
	root.SetArgs([]string{"--db", dbPath, "add", "foo", "bar"})
	require.Error(t, root.Execute())
}

func TestUnit_AgentAdd_Manual_NameFlagRejected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	root := agentTestRoot(newAddCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "add", "local-bot", "--name", "alias", "--", "/bin/echo"})
	require.Error(t, root.Execute(), "--name must be rejected in the manual form")
}

// ─── registry add (from local cache, no network) ────────────────────────────

func TestUnit_AgentAdd_Registry_NPX_FromCache(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agents.db")
	// Seed the catalog cache next to the db so Fetch reads it without a network hit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent-registry.json"), []byte(miniCatalog), 0o644))

	root := agentTestRoot(newAddCmd())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "add", "claude-acp"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()

	got, err := svc.GetByName(ctx, "claude-acp")
	require.NoError(t, err)
	require.Equal(t, agentSourceRegistry, derefOr(got.Source, ""))
	require.Equal(t, "claude-acp", derefOr(got.RegistryID, ""))
	require.Equal(t, "0.59.0", derefOr(got.RegistryVersion, ""))

	cfg, err := got.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "npx", cfg.Command)
	require.Equal(t, []string{"-y", "@agentclientprotocol/claude-agent-acp@0.59.0"}, cfg.Args)
}

func TestUnit_AgentAdd_Registry_Alias(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agents.db")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent-registry.json"), []byte(miniCatalog), 0o644))

	root := agentTestRoot(newAddCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "add", "claude-acp", "--name", "my-claude"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()

	got, err := svc.GetByName(ctx, "my-claude")
	require.NoError(t, err)
	require.Equal(t, "claude-acp", derefOr(got.RegistryID, ""), "alias renames the agent but records the registry id as provenance")
}

func TestUnit_AgentAdd_Registry_UnknownID_Errors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agents.db")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent-registry.json"), []byte(miniCatalog), 0o644))

	root := agentTestRoot(newAddCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "add", "does-not-exist"})
	require.Error(t, root.Execute())
}

// ─── edit validates + persists ──────────────────────────────────────────────

func newEditCmd() *cobra.Command {
	c := &cobra.Command{Use: "edit", Args: cobra.ExactArgs(1), RunE: agentEditCmd.RunE}
	c.Flags().String("config-file", "", "")
	return c
}

func seedManualAgent(t *testing.T, dbPath, name, command string) {
	t.Helper()
	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	a := &runtimetypes.Agent{Name: name, Enabled: true}
	require.NoError(t, a.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   command,
	}))
	source := agentSourceManual
	a.Source = &source
	require.NoError(t, svc.Create(ctx, a))
}

func TestUnit_AgentEdit_ConfigFileStdin_ValidPersists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedManualAgent(t, dbPath, "editme", "old-command")

	root := agentTestRoot(newEditCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(`{"transport":"stdio","command":"new-command","args":["--x"]}`))
	root.SetArgs([]string{"--db", dbPath, "edit", "editme", "--config-file", "-"})
	require.NoError(t, root.Execute())

	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	got, err := svc.GetByName(ctx, "editme")
	require.NoError(t, err)
	cfg, err := got.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "new-command", cfg.Command)
	require.Equal(t, []string{"--x"}, cfg.Args)
	// Provenance must be untouched by edit.
	require.Equal(t, agentSourceManual, derefOr(got.Source, ""))
}

func TestUnit_AgentEdit_ConfigFileStdin_InvalidRejected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedManualAgent(t, dbPath, "editme", "old-command")

	root := agentTestRoot(newEditCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	// stdio transport without a command must fail ExternalACPConfig.Validate().
	root.SetIn(strings.NewReader(`{"transport":"stdio"}`))
	root.SetArgs([]string{"--db", dbPath, "edit", "editme", "--config-file", "-"})
	require.Error(t, root.Execute())

	// The stored config must be unchanged after a rejected edit.
	ctx, svc, done := openServiceAt(t, dbPath)
	defer done()
	got, err := svc.GetByName(ctx, "editme")
	require.NoError(t, err)
	cfg, err := got.ExternalACPConfig()
	require.NoError(t, err)
	require.Equal(t, "old-command", cfg.Command)
}

func TestUnit_AgentEdit_ConfigFile_MalformedJSONRejected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedManualAgent(t, dbPath, "editme", "old-command")

	root := agentTestRoot(newEditCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(`{not json`))
	root.SetArgs([]string{"--db", dbPath, "edit", "editme", "--config-file", "-"})
	require.Error(t, root.Execute())
}

// ─── list / show ────────────────────────────────────────────────────────────

func TestUnit_AgentList_And_Show(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedManualAgent(t, dbPath, "shown-agent", "my-agent-cmd")

	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: agentListCmd.RunE}
	root := agentTestRoot(list)
	var listBuf bytes.Buffer
	root.SetOut(&listBuf)
	root.SetErr(&listBuf)
	root.SetArgs([]string{"--db", dbPath, "list"})
	require.NoError(t, root.Execute())
	require.Contains(t, listBuf.String(), "shown-agent")
	require.Contains(t, listBuf.String(), agentSourceManual)

	show := &cobra.Command{Use: "show", Args: cobra.ExactArgs(1), RunE: agentShowCmd.RunE}
	rootShow := agentTestRoot(show)
	var showBuf bytes.Buffer
	rootShow.SetOut(&showBuf)
	rootShow.SetErr(&showBuf)
	rootShow.SetArgs([]string{"--db", dbPath, "show", "shown-agent"})
	require.NoError(t, rootShow.Execute())
	out := showBuf.String()
	require.Contains(t, out, "my-agent-cmd")
	require.Contains(t, out, "config_json")
}

// ─── enable / disable ───────────────────────────────────────────────────────

func TestUnit_AgentEnableDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	seedManualAgent(t, dbPath, "toggle-agent", "cmd")

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

// ─── pure helpers ───────────────────────────────────────────────────────────

func TestUnit_AgentHelpers(t *testing.T) {
	require.Equal(t, "goose acp", renderRunCommand("goose", []string{"acp"}))
	require.Equal(t, "goose", renderRunCommand("goose", nil))

	require.Equal(t, "-", derefOr(nil, "-"))
	s := "registry"
	require.Equal(t, "registry", derefOr(&s, "-"))
	empty := ""
	require.Equal(t, "fallback", derefOr(&empty, "fallback"))

	require.Equal(t, "enabled", enabledWord(true))
	require.Equal(t, "disabled", enabledWord(false))

	require.Equal(t, "short", truncate("short", 60))
	require.Equal(t, "abcd…", truncate("abcdefgh", 5))

	pretty, err := prettyJSONBytes([]byte(`{"a":1}`))
	require.NoError(t, err)
	require.Contains(t, string(pretty), "\n  \"a\": 1")
	// empty raw formats as an empty object
	pretty2, err := prettyJSONBytes(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", string(pretty2))
}

// ─── check ──────────────────────────────────────────────────────────────────

func newCheckCmd() *cobra.Command {
	c := &cobra.Command{Use: "check", Args: cobra.MinimumNArgs(1), RunE: runAgentCheck}
	c.Flags().Duration("timeout", 2*time.Minute, "")
	return c
}

// buildStubAgentBin compiles the hermetic in-repo ACP stub agent so the check
// command has a real agent subprocess to spawn — the same binary the
// runtime/agenthost e2e drives.
func buildStubAgentBin(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "acp-stub-agent")
	out, err := exec.Command("go", "build", "-o", binPath, "github.com/contenox/runtime/libacp/cmd/acp-stub-agent").CombinedOutput()
	require.NoError(t, err, "build acp-stub-agent:\n%s", out)
	return binPath
}

// TestUnit_AgentCheck_DrivesARealTurnAgainstTheStub is the CLI-level close of
// the loop: register an agent (manual form), then `agent check` spawns it and
// drives one live prompt turn, streaming the reply. Against the hermetic stub
// the reply is deterministic ("ack").
func TestUnit_AgentCheck_DrivesARealTurnAgainstTheStub(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	stubBin := buildStubAgentBin(t)

	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "stub-check", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   stubBin,
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newCheckCmd())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "check", "stub-check"})
	require.NoError(t, root.Execute(), "output:\n%s", buf.String())

	out := buf.String()
	require.Contains(t, out, `Checking agent "stub-check"`)
	require.Contains(t, out, "ack", "the stub's streamed reply must reach stdout")
	require.Contains(t, out, "Turn completed (agent acp-stub-agent 0.0.1, stopReason=end_turn)")
}

func TestUnit_AgentCheck_DisabledAgentStillChecksWithNote(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	stubBin := buildStubAgentBin(t)

	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "stub-off", Enabled: false}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   stubBin,
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newCheckCmd())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "check", "stub-off"})
	require.NoError(t, root.Execute(), "output:\n%s", buf.String())
	require.Contains(t, buf.String(), "disabled; checking it anyway")
}

func TestUnit_AgentCheck_UnknownAgentErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")

	root := agentTestRoot(newCheckCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "check", "no-such-agent"})
	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestUnit_AgentCheck_ForwardsAllowlistedMcpServers registers an MCP server
// and an agent whose config allowlists it, then asserts the check resolves
// and reports the forwarding (the stub agent ignores the servers, so this
// pins the CLI wiring: allowlist → registry lookup → session/new, loudly).
func TestUnit_AgentCheck_ForwardsAllowlistedMcpServers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	stubBin := buildStubAgentBin(t)

	ctx, svc, done := openServiceAt(t, dbPath)
	db, err := OpenDBAt(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, mcpserverservice.New(db).Create(ctx, &runtimetypes.MCPServer{
		Name: "echo", Transport: "stdio", Command: "mcp-echo-server", ConnectTimeoutSeconds: 30,
	}))
	require.NoError(t, db.Close())

	agent := &runtimetypes.Agent{Name: "stub-mcp", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport:  runtimetypes.ExternalACPTransportStdio,
		Command:    stubBin,
		McpServers: []string{"echo"},
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newCheckCmd())
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "check", "stub-mcp"})
	require.NoError(t, root.Execute(), "output:\n%s", buf.String())
	require.Contains(t, buf.String(), "Forwarding MCP servers: echo")
	require.Contains(t, buf.String(), "Turn completed")
}

// TestUnit_AgentCheck_MissingAllowlistedMcpServerFailsLoudly pins that a
// check against an agent whose allowlist names an unregistered server fails
// before any subprocess is spawned, instead of silently checking with less
// context than the agent declared.
func TestUnit_AgentCheck_MissingAllowlistedMcpServerFailsLoudly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")

	ctx, svc, done := openServiceAt(t, dbPath)
	agent := &runtimetypes.Agent{Name: "stub-ghost-mcp", Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport:  runtimetypes.ExternalACPTransportStdio,
		Command:    "irrelevant-not-actually-spawned",
		McpServers: []string{"ghost"},
	}))
	require.NoError(t, svc.Create(ctx, agent))
	done()

	root := agentTestRoot(newCheckCmd())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--db", dbPath, "check", "stub-ghost-mcp"})
	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}
