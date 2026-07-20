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
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libacp/acpexec"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agenthost"
	"github.com/contenox/runtime/runtime/agentregistry"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/mcpserverservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/spf13/cobra"
)

// agent source values persisted in agents.source (provenance, not run config).
// Aliases, not copies: the values are owned by runtimetypes, which is also
// where chain-agent discovery reads them from, so the two cannot drift.
const (
	agentSourceRegistry = runtimetypes.AgentSourceRegistry
	agentSourceManual   = runtimetypes.AgentSourceManual
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage external ACP agents (search, add, list, show, edit, remove).",
	Long: `Register and manage external ACP agents the runtime can spawn and drive.

An agent is an external program that speaks the Agent Client Protocol (ACP).
contenox does NOT install agents: it only invokes ones you already have, or
that run-time fetchers like npx/uvx pull down themselves. There are exactly two
ways to register one:

  1. Seed from the ACP registry catalog:
       contenox agent search [query]        browse the catalog
       contenox agent add <registry-id>     resolve + register a catalog entry

  2. Give a bare command (everything after '--' is the argv):
       contenox agent add <name> -- <command> [args...]

Any further customization (extra args, env, cwd, endpoint transport) is done by
editing the registered JSON:

       contenox agent edit <name>           open $EDITOR on the run config

That editable JSON is the run spec. Provenance (source, registry id/version) is
system-managed and shown by 'agent show'/'agent list' but never touched by edit.

Examples:
  contenox agent search claude
  contenox agent add claude-acp
  contenox agent add goose --name my-goose
  contenox agent add local-bot -- /usr/local/bin/my-acp-agent --stdio
  contenox agent list
  contenox agent show my-goose
  contenox agent check my-goose
  contenox agent edit my-goose
  contenox agent disable my-goose
  contenox agent remove my-goose`,
}

var agentSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search the ACP agent registry catalog.",
	Long: `Fetch the ACP agent registry catalog and print matching entries as a table of
id, name, version, and description. With no query, every catalog entry is
listed. Filtering matches id, name, and description case-insensitively.

The catalog is cached locally (agent-registry.json next to the database); pass
--refresh to force a re-fetch. If the network is unavailable, the cached copy
is used.

Examples:
  contenox agent search
  contenox agent search code
  contenox agent search --refresh`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		refresh, _ := cmd.Flags().GetBool("refresh")

		client, err := openAgentRegistryClient(cmd)
		if err != nil {
			return err
		}
		reg, err := client.Fetch(ctx, refresh)
		if err != nil {
			return fmt.Errorf("failed to fetch agent registry: %w", err)
		}

		results := agentregistry.Search(reg, strings.Join(args, " "))
		if len(results) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No matching agents in the ACP registry.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tVERSION\tDESCRIPTION")
		for _, a := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Version, truncate(a.Description, 60))
		}
		return w.Flush()
	},
}

var agentAddCmd = &cobra.Command{
	Use:   "add <registry-id> [--name <alias>]  |  add <name> -- <command> [args...]",
	Short: "Register an ACP agent from the registry or from a bare command.",
	Long: `Register an external ACP agent. There are two mutually exclusive forms.

Registry form:
  contenox agent add <registry-id> [--name <alias>]

  Resolves the catalog entry for this OS/arch into a run spec and registers it
  with source=registry. npx/uvx entries become 'npx -y <package> ...' /
  'uvx <package> ...'; binary entries become the binary basename (you must have
  it on PATH — contenox never downloads it). The agent is named after the
  registry id unless --name gives an alias.

Manual form:
  contenox agent add <name> -- <command> [args...]

  Everything after '--' is the argv to spawn. Registered with source=manual.
  This is the only raw-command path; there are deliberately no
  --transport/--env/--args flags. To customize further, use 'contenox agent edit'.

Examples:
  contenox agent add claude-acp
  contenox agent add goose --name my-goose
  contenox agent add local-bot -- /usr/local/bin/my-acp-agent --stdio`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAgentAdd,
}

func runAgentAdd(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	dashPos := cmd.ArgsLenAtDash()
	alias, _ := cmd.Flags().GetString("name")

	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	if dashPos == -1 {
		// Registry form: exactly one positional (the registry id).
		if len(args) != 1 {
			return fmt.Errorf("registry form takes exactly one registry id\n\n" +
				"  contenox agent add <registry-id> [--name <alias>]\n" +
				"  contenox agent add <name> -- <command> [args...]")
		}
		return addFromRegistry(ctx, cmd, svc, args[0], alias)
	}

	// Manual form: <name> before '--', <command> [args...] after it.
	if alias != "" {
		return fmt.Errorf("--name is only for the registry form; in the manual form the name is the positional before '--'")
	}
	if dashPos != 1 {
		return fmt.Errorf("manual form takes exactly one name before '--'\n\n  contenox agent add <name> -- <command> [args...]")
	}
	name := args[0]
	argv := args[dashPos:]
	if len(argv) == 0 {
		return fmt.Errorf("provide a command after '--'\n\n  contenox agent add %s -- <command> [args...]", name)
	}
	return addManual(ctx, cmd, svc, name, argv[0], argv[1:])
}

func addFromRegistry(ctx context.Context, cmd *cobra.Command, svc agentregistryservice.Service, registryID, alias string) error {
	refresh, _ := cmd.Flags().GetBool("refresh")
	client, err := openAgentRegistryClient(cmd)
	if err != nil {
		return err
	}
	reg, err := client.Fetch(ctx, refresh)
	if err != nil {
		return fmt.Errorf("failed to fetch agent registry: %w", err)
	}

	entry, ok := agentregistry.Find(reg, registryID)
	if !ok {
		return fmt.Errorf("registry agent %q not found (run 'contenox agent search' to browse the catalog)", registryID)
	}

	spec, err := agentregistry.Resolve(entry, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("cannot resolve %q for %s/%s: %w", registryID, runtime.GOOS, runtime.GOARCH, err)
	}

	name := entry.ID
	if alias != "" {
		name = alias
	}

	agent := &runtimetypes.Agent{Name: name, Enabled: true}
	if err := agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   spec.Command,
		Args:      spec.Args,
		Env:       spec.Env,
	}); err != nil {
		return err
	}
	source := agentSourceRegistry
	regID := entry.ID
	regVer := entry.Version
	agent.Source = &source
	agent.RegistryID = &regID
	agent.RegistryVersion = &regVer

	if err := svc.Create(ctx, agent); err != nil {
		return fmt.Errorf("failed to add agent: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Agent %q added from registry (%s@%s, via %s).\n", name, entry.ID, entry.Version, spec.Method)
	fmt.Fprintf(out, "  Run command: %s\n", renderRunCommand(spec.Command, spec.Args))
	if spec.Note != "" {
		fmt.Fprintf(out, "  Note: %s\n", spec.Note)
	}
	return nil
}

func addManual(ctx context.Context, cmd *cobra.Command, svc agentregistryservice.Service, name, command string, argv []string) error {
	agent := &runtimetypes.Agent{Name: name, Enabled: true}
	if err := agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   command,
		Args:      argv,
	}); err != nil {
		return err
	}
	source := agentSourceManual
	agent.Source = &source

	if err := svc.Create(ctx, agent); err != nil {
		return fmt.Errorf("failed to add agent: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Agent %q added (manual).\n  Run command: %s\n", name, renderRunCommand(command, argv))
	return nil
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents.",
	Long: `List every registered agent as a table of id, name, source, kind, and enabled
state. Source is 'registry' (seeded from the ACP catalog) or 'manual' (a bare
command). If none are registered, prints a hint to run 'contenox agent add'.`,
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
			fmt.Fprintln(cmd.OutOrStdout(), "No agents registered. Run: contenox agent add <registry-id>  (or)  contenox agent add <name> -- <command>")
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
	Long: `Look up an agent by name and print its provenance, the resolved run command,
and the raw config_json. The config_json block is exactly what
'contenox agent edit' opens for editing; provenance (source, registry id/
version) is system-managed and not part of it.`,
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
			return fmt.Errorf("agent %q not found: %w", name, err)
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
				fmt.Fprintf(out, "Run command: %s\n", renderRunCommand(cfg.Command, cfg.Args))
			}
		}

		pretty, err := prettyJSON(agent.ConfigJSON)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "\nConfig (config_json — edited by 'contenox agent edit'):")
		fmt.Fprintln(out, pretty)
		return nil
	},
}

var agentEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit an agent's run config in $EDITOR.",
	Long: `Open the agent's config_json in $EDITOR (or $VISUAL; nano fallback), validate
it on save, and persist it. This is the customization escape hatch: instead of
a pile of flags, edit the JSON directly to change command/args/env/cwd or switch
transport. Provenance columns are never touched.

Non-interactively, pass --config-file <path> to replace the config from a file,
or --config-file - to read it from stdin.

Examples:
  contenox agent edit my-goose
  contenox agent edit my-goose --config-file new-config.json
  echo '{"transport":"stdio","command":"my-agent"}' | contenox agent edit my-goose --config-file -`,
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
			return fmt.Errorf("agent %q not found: %w", name, err)
		}

		current, err := prettyJSONBytes(agent.ConfigJSON)
		if err != nil {
			return err
		}

		var edited []byte
		configFile, _ := cmd.Flags().GetString("config-file")
		if configFile != "" {
			edited, err = readConfigFile(cmd, configFile)
			if err != nil {
				return err
			}
		} else {
			edited, err = captureConfigFromEditor(current)
			if err != nil {
				if errors.Is(err, errConfigUnchanged) {
					fmt.Fprintln(cmd.OutOrStdout(), "No changes; agent left unchanged.")
					return nil
				}
				return err
			}
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
		fmt.Fprintf(cmd.OutOrStdout(), "Agent %q updated.\n", name)
		return nil
	},
}

var agentRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a registered agent.",
	Long: `Delete an agent by name from the local database. This removes only the local
registration; it does not affect any binary or package the agent would spawn.`,
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
			return fmt.Errorf("agent %q not found: %w", name, err)
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
	Short: "Spawn a registered agent and drive one live prompt turn through it.",
	Long: `Resolve an agent by name, spawn it as an ACP subprocess, and drive one full
initialize → session/new → session/prompt turn against it, streaming the
agent's reply to stdout. This is how to verify a registered agent actually
works: it exercises the same client-host path the runtime itself uses
(runtime/agenthost), not a lighter fake.

The check drives one plain text turn rooted in the current working directory.
Agent-initiated callbacks (file system, terminal, permission requests) are
declined, so an agent that insists on them may stop early; answering a simple
prompt should not need any.

If the agent's config declares an mcp_servers allowlist (registered MCP
server names, see 'contenox mcp list' and 'contenox agent edit'), those
servers are forwarded to the agent in session/new exactly as a real client
session would — entries the agent's capabilities cannot consume are reported,
not silently dropped. The slash commands the agent advertises during the turn
are printed after the reply.

Everything after the name is used as the prompt; without one, the agent is
asked to confirm the connection.

Examples:
  contenox agent check my-goose
  contenox agent check claude Say hello
  contenox agent check local-bot --timeout 30s`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAgentCheck,
}

// checkHarness streams agent_message_chunk text to out as it arrives while
// recording everything via the embedded RecordingHarness, so `agent check`
// shows the reply live and can still run turn-level verification afterwards.
type checkHarness struct {
	agenthost.RecordingHarness
	out io.Writer
}

func (h *checkHarness) SessionUpdate(ctx context.Context, n libacp.SessionNotification) error {
	if n.Update.SessionUpdate == libacp.SessionUpdateAgentMessageChunk {
		if c := n.Update.Content; c != nil && c.Type == string(libacp.ContentKindText) {
			fmt.Fprint(h.out, c.Text)
		}
	}
	return h.RecordingHarness.SessionUpdate(ctx, n)
}

func runAgentCheck(cmd *cobra.Command, args []string) error {
	ctx := libtracker.WithNewRequestID(context.Background())
	name := args[0]

	db, svc, err := openAgentService(cmd)
	if err != nil {
		return err
	}
	defer db.Close()

	agent, err := svc.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("agent %q not found: %w", name, err)
	}
	cfg, err := agent.ExternalACPConfig()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if !agent.Enabled {
		fmt.Fprintf(out, "Note: agent %q is disabled; checking it anyway.\n", name)
	}
	fmt.Fprintf(out, "Checking agent %q: %s\n", name, renderRunCommand(cfg.Command, cfg.Args))

	// The agent's mcp_servers allowlist is part of its declared run context:
	// a check without it would verify a different setup than the one the
	// agent actually runs with.
	var mcpServers []libacp.McpServer
	if len(cfg.McpServers) > 0 {
		mcpServers, err = agenthost.ResolveForwardedMcpServers(ctx, mcpserverservice.New(db), cfg.McpServers)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Forwarding MCP servers: %s\n", strings.Join(cfg.McpServers, ", "))
	}
	fmt.Fprintln(out)

	promptText := strings.TrimSpace(strings.Join(args[1:], " "))
	if promptText == "" {
		promptText = "This is a connection check. Reply with a short confirmation."
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	timeout, _ := cmd.Flags().GetDuration("timeout")
	turnCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var agentStderr acpexec.LockedBuffer
	harness := &checkHarness{out: out}
	res, err := agenthost.DriveTurn(turnCtx, agent, harness, agenthost.TurnRequest{
		Cwd:        cwd,
		Prompt:     []libacp.ContentBlock{libacp.NewTextContent(promptText)},
		ClientInfo: &libacp.Implementation{Name: "contenox", Title: "contenox agent check", Version: cliVersion()},
		McpServers: mcpServers,
		Stderr:     &agentStderr,
		// Persistent agents (most editor adapters) never exit on stdin-close;
		// a short grace keeps the check from stalling on teardown.
		KillGrace: 2 * time.Second,
	})
	if harness.MessageText() != "" {
		fmt.Fprintln(out)
	}
	if err != nil {
		if s := agentStderr.String(); s != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "agent stderr:\n%s\n", s)
		}
		return fmt.Errorf("check failed: %w", err)
	}

	// A normal prompt response with zero displayable output is the known
	// empty-turn interop failure — fail the check rather than report success
	// on a silent agent.
	tracker := &libacp.TurnTracker{}
	for _, n := range harness.Updates() {
		tracker.Observe(n)
	}
	if trackErr := tracker.Err(res.StopReason); trackErr != nil {
		if s := agentStderr.String(); s != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "agent stderr:\n%s\n", s)
		}
		return fmt.Errorf("check failed: %w", trackErr)
	}

	if info := res.Initialize.AgentInfo; info != nil && info.Name != "" {
		fmt.Fprintf(out, "\nTurn completed (agent %s %s, stopReason=%s).\n", info.Name, info.Version, res.StopReason)
	} else {
		fmt.Fprintf(out, "\nTurn completed (stopReason=%s).\n", res.StopReason)
	}
	if len(res.DroppedMcpServers) > 0 {
		fmt.Fprintf(out, "Note: MCP servers NOT forwarded — the agent's mcpCapabilities cannot consume their transport: %s\n",
			strings.Join(res.DroppedMcpServers, ", "))
	}
	if cmds := harness.AvailableCommands(); len(cmds) > 0 {
		names := make([]string, 0, len(cmds))
		for _, c := range cmds {
			names = append(names, "/"+c.Name)
		}
		fmt.Fprintf(out, "Agent advertises %d command(s): %s\n", len(cmds), strings.Join(names, " "))
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
		return fmt.Errorf("agent %q not found: %w", name, err)
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

// ─── helpers ────────────────────────────────────────────────────────────────

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
	return db, agentregistryservice.New(db), nil
}

// openAgentRegistryClient builds a catalog client whose cache lives next to the
// database (agent-registry.json in the same directory), so `--db`/`--data-dir`
// keep the cache alongside the agents it seeds.
func openAgentRegistryClient(cmd *cobra.Command) (*agentregistry.Client, error) {
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return nil, fmt.Errorf("invalid database path: %w", err)
	}
	cachePath := filepath.Join(filepath.Dir(dbPath), "agent-registry.json")
	return agentregistry.NewClient(cachePath), nil
}

func readConfigFile(cmd *cobra.Command, path string) ([]byte, error) {
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

var errConfigUnchanged = errors.New("config unchanged")

// captureConfigFromEditor opens $EDITOR on seed and returns the edited bytes.
// It reuses resolveEditor/runEditor (see editor.go). If the content is byte-for-
// byte unchanged, it returns errConfigUnchanged so the caller can no-op.
func captureConfigFromEditor(seed []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "contenox-agent-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if _, err := f.Write(seed); err != nil {
		f.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}
	initialHash := sha256.Sum256(seed)

	if err := runEditor(tmpPath); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}

	final, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read temp file: %w", err)
	}
	finalHash := sha256.Sum256(final)
	if bytes.Equal(initialHash[:], finalHash[:]) {
		return nil, errConfigUnchanged
	}
	return final, nil
}

func renderRunCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return command + " " + strings.Join(args, " ")
}

func prettyJSON(raw json.RawMessage) (string, error) {
	b, err := prettyJSONBytes(raw)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func init() {
	agentSearchCmd.Flags().Bool("refresh", false, "Force a re-fetch of the ACP registry catalog instead of using the local cache")
	agentAddCmd.Flags().String("name", "", "Alias for a registry agent (registry form only; defaults to the registry id)")
	agentAddCmd.Flags().Bool("refresh", false, "Force a re-fetch of the ACP registry catalog before resolving")
	agentEditCmd.Flags().String("config-file", "", "Replace the config from a file (or '-' for stdin) instead of opening $EDITOR")
	agentCheckCmd.Flags().Duration("timeout", 2*time.Minute, "How long the whole check turn may take before it is cancelled")

	agentCmd.AddCommand(agentSearchCmd)
	agentCmd.AddCommand(agentAddCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentShowCmd)
	agentCmd.AddCommand(agentCheckCmd)
	agentCmd.AddCommand(agentEditCmd)
	agentCmd.AddCommand(agentRemoveCmd)
	agentCmd.AddCommand(agentEnableCmd)
	agentCmd.AddCommand(agentDisableCmd)
}
