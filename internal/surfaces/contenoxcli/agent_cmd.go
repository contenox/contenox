package contenoxcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/contenox/contenox/internal/services/agenthost"
	"github.com/contenox/contenox/internal/services/agentregistryservice"
	"github.com/contenox/contenox/internal/services/mcpserverservice"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libacp/acpexec"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

const agentSourceManual = runtimetypes.AgentSourceManual

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Register, inspect, and drive declared or external ACP agents.",
	Long: `Manage the agents the runtime can spawn and drive.

An agent is a Markdown file with a YAML frontmatter header:

  ---
  name: reviewer
  description: Reviews a file for correctness problems
  tools: Read, Glob, Grep
  ---

  You are a code reviewer. Read the file you are asked about, then list the
  problems you can point at in what you actually read.

Write them in .contenox/agents/ (or ~/.contenox/agents/ for every project).
Agents you already keep in .claude/agents/ or .agents/agents/ are read where
they are — nothing to move or convert. Every one is registered automatically;
this command inspects them and toggles their enabled state.

An installed program that speaks ACP can be registered directly:

  contenox agent add claude -- npx -y @zed-industries/claude-code-acp
  contenox agent check claude

External agents run as sandboxed subprocesses. Use 'agent edit' to set their
argv, environment, working directory, or forwarded MCP-server allowlist.

What a declaration cannot say — context budget, retries, shell allowlists, what
needs a human — lives in agents.toml beside them.

` + toolGrantGrammar + `

Omitting the frontmatter's tools line is the same grant as "*".

` + askWaitGrammar + `

Examples:
  contenox agent add local-bot -- /usr/local/bin/my-acp-agent
  contenox agent check local-bot
  contenox agent edit local-bot
  contenox agent list
  contenox agent show reviewer
  contenox agent disable reviewer
  contenox agent remove reviewer`,
}

var agentAddCmd = &cobra.Command{
	Use:   "add <name> -- <command> [args...]",
	Short: "Register an installed external ACP agent.",
	Long: `Register a program that speaks ACP over stdio.

Everything after '--' is the exact argv Contenox spawns. Contenox records the
command; it does not install the program or fetch packages itself. Use an
explicit package version when a launcher such as npx or uvx downloads code.

Examples:
  contenox agent add claude -- npx -y @zed-industries/claude-code-acp
  contenox agent add goose -- goose acp
  contenox agent add local-bot -- /usr/local/bin/my-acp-agent --stdio`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAgentAdd,
}

func runAgentAdd(cmd *cobra.Command, args []string) error {
	dashPos := cmd.ArgsLenAtDash()
	if dashPos != 1 {
		return fmt.Errorf("provide one name before '--'\n\n  contenox agent add <name> -- <command> [args...]")
	}
	argv := args[dashPos:]
	if len(argv) == 0 {
		return fmt.Errorf("provide a command after '--'\n\n  contenox agent add %s -- <command> [args...]", args[0])
	}

	ctx := libtracker.WithNewRequestID(context.Background())
	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	agent := &runtimetypes.Agent{Name: args[0], Enabled: true}
	if err := agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   argv[0],
		Args:      argv[1:],
	}); err != nil {
		return err
	}
	source := agentSourceManual
	agent.Source = &source
	if err := svc.Create(ctx, agent); err != nil {
		return fmt.Errorf("failed to add agent: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Agent %q added.\n  Run command: %s\n", agent.Name, renderAgentCommand(argv[0], argv[1:]))
	return nil
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents.",
	Long: `List every registered agent as a table of id, name, source, kind, and enabled
state. If none are registered, prints a hint.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, svc, err := openAgentService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		agents, err := svc.List(ctx, nil, 100)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}
		if len(agents) == 0 {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "No agents yet.")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "An agent is one Markdown file. Write .contenox/agents/reviewer.md:")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "  ---")
			fmt.Fprintln(out, "  name: reviewer")
			fmt.Fprintln(out, "  description: Reviews a file for correctness problems")
			fmt.Fprintln(out, "  tools: Read, Glob, Grep")
			fmt.Fprintln(out, "  ---")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "  You are a code reviewer. Read the file you are asked about, then")
			fmt.Fprintln(out, "  list the problems you can point at in what you actually read.")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Then run this again. Agents already in .claude/agents/ are found too.")
			fmt.Fprintln(out, "Or register an installed ACP agent: contenox agent add <name> -- <command> [args...]")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSOURCE\tKIND\tENABLED")
		for _, a := range agents {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", a.ID, a.Name, derefOr(a.Source, "-"), a.Kind, a.Enabled)
		}
		return w.Flush()
	},
}

var agentShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show an agent's full declaration and run config.",
	Long: `Look up an agent by name and print its provenance and raw config_json.

The tools allowlist prints as it was resolved: "*" admits every connected
toolset with no exceptions, "!name" removes one, a bare name grants exactly
that one.
Provenance (source, registry id/version) is system-managed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		name := args[0]
		db, svc, err := openAgentService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		agent, err := svc.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("agent %q not found%s: %w", name, legacyChainPrefixHint(name), err)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Name:     %s\n", agent.Name)
		fmt.Fprintf(out, "ID:       %s\n", agent.ID)
		fmt.Fprintf(out, "Kind:     %s\n", agent.Kind)
		fmt.Fprintf(out, "Enabled:  %v\n", agent.Enabled)
		fmt.Fprintf(out, "Source:   %s\n", derefOr(agent.Source, "-"))
		if agent.RegistryID != nil {
			fmt.Fprintf(out, "Registry: %s@%s\n", *agent.RegistryID, derefOr(agent.RegistryVersion, "?"))
		}
		if agent.Kind == runtimetypes.AgentKindExternalACP {
			if cfg, cfgErr := agent.ExternalACPConfig(); cfgErr == nil && cfg.Command != "" {
				fmt.Fprintf(out, "Run command: %s\n", renderAgentCommand(cfg.Command, cfg.Args))
			}
		}

		pretty, err := prettyJSON(agent.ConfigJSON)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "\nConfig (config_json):")
		fmt.Fprintln(out, pretty)
		return nil
	},
}

var agentEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit an external ACP agent's run configuration.",
	Long: `Open an external ACP agent's config_json in $VISUAL or $EDITOR, validate
it, and persist it. Use --config-file to replace it non-interactively; '-' reads
JSON from stdin.

The editable fields are transport, command, args, env, cwd, url, and
mcp_servers. Only stdio transport can currently be driven.

Examples:
  contenox agent edit claude
  contenox agent edit claude --config-file claude-agent.json
  printf '%s' '{"transport":"stdio","command":"goose","args":["acp"]}' |
    contenox agent edit goose --config-file -`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentEdit,
}

func runAgentEdit(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	agent, err := svc.GetByName(ctx, args[0])
	if err != nil {
		return fmt.Errorf("agent %q not found: %w", args[0], err)
	}
	if agent.Kind != runtimetypes.AgentKindExternalACP {
		return fmt.Errorf("agent %q is %q; its declaration file is the editable source", agent.Name, agent.Kind)
	}
	current, err := prettyJSONBytes(agent.ConfigJSON)
	if err != nil {
		return err
	}

	configFile, _ := cmd.Flags().GetString("config-file")
	var edited []byte
	if configFile != "" {
		edited, err = readAgentConfig(cmd, configFile)
	} else {
		edited, err = captureAgentConfig(current)
	}
	if errors.Is(err, errAgentConfigUnchanged) {
		fmt.Fprintln(cmd.OutOrStdout(), "No changes; agent left unchanged.")
		return nil
	}
	if err != nil {
		return err
	}

	var cfg runtimetypes.ExternalACPConfig
	if err := json.Unmarshal(edited, &cfg); err != nil {
		return fmt.Errorf("invalid config JSON: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := agent.SetExternalACPConfig(cfg); err != nil {
		return err
	}
	if err := svc.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Agent %q updated.\n", agent.Name)
	return nil
}

var agentRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a registered agent.",
	Long: `Delete an agent by name from the local database. This removes only the local
registration; discovery may re-register it on the next startup if its chain
file still exists. A manually registered external agent stays removed; its
installed program is not uninstalled.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		name := args[0]
		db, svc, err := openAgentService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		agent, err := svc.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("agent %q not found%s: %w", name, legacyChainPrefixHint(name), err)
		}
		if err := svc.Delete(ctx, agent.ID); err != nil {
			return fmt.Errorf("failed to remove agent: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Agent %q removed.\n", name)
		return nil
	},
}

var agentCheckCmd = &cobra.Command{
	Use:   "check <name> [prompt...]",
	Short: "Drive one live ACP turn through a registered external agent.",
	Long: `Spawn an external ACP agent and drive initialize, session/new, and one
prompt turn through it. The reply streams to stdout. The turn is rooted in the
current directory and bounded by the global --timeout flag.

A check never grants permission requests. It returns a protocol-shaped denial,
reports the denied operation, and continues observing the turn. MCP servers in
the agent's mcp_servers allowlist are resolved from 'contenox mcp list' and
forwarded only when the agent advertises support for their transport.

Examples:
  contenox agent check claude
  contenox agent check claude Reply with your name
  contenox agent check claude --timeout 30s`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAgentCheck,
}

type agentCheckHarness struct {
	agenthost.DenyingHarness
	out io.Writer
}

func (h *agentCheckHarness) SessionUpdate(ctx context.Context, n libacp.SessionNotification) error {
	if n.Update.SessionUpdate == libacp.SessionUpdateAgentMessageChunk {
		if content := n.Update.Content; content != nil && content.Type == string(libacp.ContentKindText) {
			fmt.Fprint(h.out, content.Text)
		}
	}
	return h.DenyingHarness.SessionUpdate(ctx, n)
}

func runAgentCheck(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	agent, err := svc.GetByName(ctx, args[0])
	if err != nil {
		return fmt.Errorf("agent %q not found: %w", args[0], err)
	}
	cfg, err := agent.ExternalACPConfig()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if !agent.Enabled {
		fmt.Fprintf(out, "Note: agent %q is disabled; checking it anyway.\n", agent.Name)
	}
	fmt.Fprintf(out, "Checking agent %q: %s\n", agent.Name, renderAgentCommand(cfg.Command, cfg.Args))

	var mcpServers []libacp.McpServer
	if len(cfg.McpServers) > 0 {
		mcpServers, err = agenthost.ResolveForwardedMcpServers(ctx, mcpserverservice.New(db), cfg.McpServers)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Forwarding MCP servers: %s\n", strings.Join(cfg.McpServers, ", "))
	}
	fmt.Fprintln(out)

	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		prompt = "This is a connection check. Reply with a short confirmation."
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	timeout, _ := cmd.Flags().GetDuration("timeout")
	turnCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var agentStderr acpexec.LockedBuffer
	harness := &agentCheckHarness{out: out}
	result, err := agenthost.DriveTurn(turnCtx, agent, harness, agenthost.TurnRequest{
		Cwd:        cwd,
		Prompt:     []libacp.ContentBlock{libacp.NewTextContent(prompt)},
		ClientInfo: &libacp.Implementation{Name: "contenox", Title: "contenox agent check", Version: cliVersion()},
		McpServers: mcpServers,
		Stderr:     &agentStderr,
		KillGrace:  2 * time.Second,
	})
	if harness.MessageText() != "" {
		fmt.Fprintln(out)
	}
	if denied := harness.Denied(); len(denied) > 0 {
		fmt.Fprintf(out, "Denied %d permission ask(s) during the turn: %s\n", len(denied), strings.Join(denied, ", "))
	}
	if err != nil {
		if stderr := agentStderr.String(); stderr != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "agent stderr:\n%s\n", stderr)
		}
		return fmt.Errorf("check failed: %w", err)
	}

	tracker := &libacp.TurnTracker{}
	for _, update := range harness.Updates() {
		tracker.Observe(update)
	}
	if err := tracker.Err(result.StopReason); err != nil {
		return fmt.Errorf("check failed: %w", err)
	}
	if info := result.Initialize.AgentInfo; info != nil && info.Name != "" {
		fmt.Fprintf(out, "\nTurn completed (agent %s %s, stopReason=%s).\n", info.Name, info.Version, result.StopReason)
	} else {
		fmt.Fprintf(out, "\nTurn completed (stopReason=%s).\n", result.StopReason)
	}
	if len(result.DroppedMcpServers) > 0 {
		fmt.Fprintf(out, "MCP servers not forwarded because the agent does not support their transport: %s\n", strings.Join(result.DroppedMcpServers, ", "))
	}
	verbose, _ := cmd.Flags().GetBool("verbose")
	if commands := harness.AvailableCommands(); verbose && len(commands) > 0 {
		names := make([]string, 0, len(commands))
		for _, command := range commands {
			names = append(names, "/"+command.Name)
		}
		fmt.Fprintf(out, "Agent advertises %d command(s): %s\n", len(commands), strings.Join(names, " "))
	}
	return nil
}

var agentEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a registered agent.",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setAgentEnabled(cmd, args[0], true) },
}

var agentDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a registered agent.",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setAgentEnabled(cmd, args[0], false) },
}

func setAgentEnabled(cmd *cobra.Command, name string, enabled bool) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	agent, err := svc.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("agent %q not found%s: %w", name, legacyChainPrefixHint(name), err)
	}
	if agent.Enabled == enabled {
		fmt.Fprintf(cmd.OutOrStdout(), "Agent %q already %s.\n", name, enabledWord(enabled))
		return nil
	}
	agent.Enabled = enabled
	if err := svc.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Agent %q %s.\n", name, enabledWord(enabled))
	return nil
}

// legacyChainPrefixHint explains a name that would have resolved before declared
// agents dropped the chain- prefix from their id; empty for anything else.
func legacyChainPrefixHint(name string) string {
	bare := strings.TrimPrefix(name, "chain-")
	if bare == name || bare == "" {
		return ""
	}
	return fmt.Sprintf(" — declared agents no longer carry the chain- prefix; try %q", bare)
}

func openAgentService(cmd *cobra.Command) (libdb.DBManager, agentregistryservice.Service, error) {
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid database path: %w", err)
	}
	dbCtx := libtracker.WithNewRequestID(context.Background())
	db, err := OpenDBAt(dbCtx, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}
	agents := agentregistryservice.New(db)
	// Declarations are the source of truth, so inspecting the roster runs a
	// discovery pass first, and prints whatever that pass could not act on.
	if contenoxDir, dirErr := ResolveContenoxDir(cmd); dirErr == nil {
		printSyncProblems(cmd.ErrOrStderr(), discoverChainAgentsReporting(dbCtx, agents, contenoxDir, libtracker.NoopTracker{}, DiscoverDeps{Store: runtimetypes.New(db.WithoutTransaction())}))
	}
	return db, agents, nil
}

func prettyJSON(raw json.RawMessage) (string, error) {
	formatted, err := prettyJSONBytes(raw)
	return string(formatted), err
}

func prettyJSONBytes(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return nil, fmt.Errorf("format config JSON: %w", err)
	}
	return buf.Bytes(), nil
}

func readAgentConfig(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read config from stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	return data, nil
}

var errAgentConfigUnchanged = errors.New("agent config unchanged")

func captureAgentConfig(seed []byte) ([]byte, error) {
	file, err := os.CreateTemp("", "contenox-agent-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(seed); err != nil {
		file.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}
	before := sha256.Sum256(seed)
	if err := runEditor(path); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read temp file: %w", err)
	}
	if before == sha256.Sum256(after) {
		return nil, errAgentConfigUnchanged
	}
	return after, nil
}

func renderAgentCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return command + " " + strings.Join(args, " ")
}

func derefOr(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

func enabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func init() {
	agentEditCmd.Flags().String("config-file", "", "Read replacement config JSON from a file; '-' reads stdin")
	agentCheckCmd.Flags().Bool("verbose", false, "Show the agent's advertised commands")
	agentCmd.AddCommand(agentAddCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentShowCmd)
	agentCmd.AddCommand(agentEditCmd)
	agentCmd.AddCommand(agentCheckCmd)
	agentCmd.AddCommand(agentRemoveCmd)
	agentCmd.AddCommand(agentEnableCmd)
	agentCmd.AddCommand(agentDisableCmd)
}
