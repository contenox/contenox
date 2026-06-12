package contenoxcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/updatecheck"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/reasoning"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run the Contenox ACP server over stdio.",
	Long: `Speak Agent Client Protocol over stdio so editors like Zed can run local Contenox chains.

The chain executed for each session/prompt is loaded from ~/.contenox/default-acp-chain.json
(override with the CONTENOX_ACP_CHAIN_PATH environment variable). Populate it like any other
contenox chain.

The default model is read from the global 'default-model' / 'default-provider' configuration
(set via 'contenox config set default-model …'). Logging goes to stderr; stdin/stdout are
reserved for the JSON-RPC stream.

HITL is on by default — gated tool calls route through the ACP session/request_permission
flow so the editor's UI controls approval. Pass --auto to disable (unattended/testing).`,
	RunE: runACP,
}

var acpxCmd = &cobra.Command{
	Use:   "acpx",
	Short: "Run as an ACP agent under the headless / untrusted-driver profile (OpenClaw).",
	Long: `Same Agent Client Protocol server as 'acp', for drivers that are not the
device owner — OpenClaw and other non-editor clients. It loads the hardened
hitl-policy-acpx.json (local_shell denied, web mutations denied, web reads
gated) and the chain at ~/.contenox/headless-acp-chain.json (override with
CONTENOX_ACPX_CHAIN_PATH).

Containment for the untrusted driver is the HITL policy, not an in-chain
step. IDE clients (Zed, GoLand, AionUi) should keep using 'acp'. Selection
is per-spawn: each ACP client launches its own contenox process, so the two
profiles never share state.`,
	RunE: runACPX,
}

func init() {
	for _, c := range []*cobra.Command{acpCmd, acpxCmd} {
		c.Flags().Bool("auto", false, "Non-interactive mode: disable HITL permission prompts (gated tools run unattended)")
		c.Flags().Bool("setup", false, "Run interactive setup wizard to configure provider and model, then exit.")
		c.Flags().String("workspace-id", "", "Workspace ID for new ACP sessions (default: the stable workspace from ~/.contenox/workspace.id, same as the CLI). Existing sessions are always located by their session ID regardless of workspace.")
	}
	acpCmd.Flags().Bool("experimental-acp", false, "Accepted for compatibility with ACP clients that hardcode this launch flag (e.g. AionUi); no effect.")
	_ = acpCmd.Flags().MarkHidden("experimental-acp")
}

type acpStdio struct{}

func (acpStdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (acpStdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (acpStdio) Close() error                { return os.Stdin.Close() }

type acpProfile struct {
	hitlPolicy string
	chainFile  string
	chainEnv   string
	seedChain  func(contenoxDir string) error
}

var acpProfileACP = acpProfile{
	hitlPolicy: "hitl-policy-acp.json",
	chainFile:  "default-acp-chain.json",
	chainEnv:   "CONTENOX_ACP_CHAIN_PATH",
	seedChain:  seedACPChainIfMissing,
}

var acpProfileACPX = acpProfile{
	hitlPolicy: "hitl-policy-acpx.json",
	chainFile:  headlessACPChainFilename,
	chainEnv:   "CONTENOX_ACPX_CHAIN_PATH",
	seedChain:  seedHeadlessACPChainIfMissing,
}

func runACP(cmd *cobra.Command, _ []string) error  { return runACPProfile(cmd, acpProfileACP) }
func runACPX(cmd *cobra.Command, _ []string) error { return runACPProfile(cmd, acpProfileACPX) }

func runACPProfile(cmd *cobra.Command, profile acpProfile) error {
	if setup, _ := cmd.Flags().GetBool("setup"); setup {
		return runSetup(cmd, cmd.OutOrStdout())
	}

	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, stop := signal.NotifyContext(parentCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	autoMode, _ := cmd.Flags().GetBool("auto")
	enableHITL := !autoMode

	workspaceFlag, _ := cmd.Flags().GetString("workspace-id")

	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return err
	}
	dbCtx := libtracker.WithNewRequestID(ctx)
	db, err := OpenDBAt(dbCtx, dbPath)
	if err != nil {
		return fmt.Errorf("open database %q: %w", dbPath, err)
	}
	defer db.Close()

	// Anchor seeding/telemetry to the SAME directory the ACP runtime reads from.
	// The DB (globalDBPath), the chain (LoadChainRegistryFrom) and HITL policies
	// (acpPolicySource) all resolve to $HOME/.contenox via os.UserHomeDir(),
	// ignoring ResolveContenoxDir's cwd-walk. Using that cwd-walk here meant a
	// launch from an arbitrary working directory (Zed's project dir, or the ACP
	// registry's isolated sandbox) seeded presets into <cwd>/.contenox while the
	// loaders looked in $HOME/.contenox — so the chain/policy presets were
	// silently absent and `acp` hard-errored before serving initialize.
	contenoxDir, err := globalContenoxDir()
	if err != nil {
		return fmt.Errorf("resolve contenox dir: %w", err)
	}
	workspaceID := workspaceFlag
	if workspaceID == "" {
		workspaceID = ResolveWorkspaceID(contenoxDir)
	}
	if err := writeEmbeddedHITLPolicies(contenoxDir, false); err != nil {
		return fmt.Errorf("seed HITL policy presets: %w", err)
	}
	if profile.seedChain != nil {
		if err := profile.seedChain(contenoxDir); err != nil {
			return fmt.Errorf("seed ACP chain preset: %w", err)
		}
	}

	closeLogs, err := setupTelemetryLogging(ctx, runtimetypes.New(db.WithoutTransaction()), contenoxDir)
	if err != nil {
		return fmt.Errorf("setup telemetry logging: %w", err)
	}
	defer closeLogs()

	var tracker libtracker.ActivityTracker = libtracker.NewTextActivityTracker(os.Stderr)
	var transport *acpsvc.Transport

	defaultModel := acpsvc.ReadConfigValue(ctx, db, "default-model")
	defaultProvider := acpsvc.ReadConfigValue(ctx, db, "default-provider")
	defaultAltModel := acpsvc.ReadConfigValue(ctx, db, "default-alt-model")
	defaultAltProvider := acpsvc.ReadConfigValue(ctx, db, "default-alt-provider")
	defaultMaxTokens, err := normalizeMaxTokensConfig(acpsvc.ReadConfigValue(ctx, db, "default-max-tokens"))
	if err != nil {
		return err
	}
	defaultThink := reasoning.Default
	if configuredThink := acpsvc.ReadConfigValue(ctx, db, "default-think"); configuredThink != "" {
		level, err := reasoning.Normalize(configuredThink)
		if err != nil {
			return fmt.Errorf("config default-think: %w", err)
		}
		defaultThink = level
	}

	chains, err := acpsvc.LoadChainRegistryFrom(profile.chainFile, profile.chainEnv)
	if err != nil {
		return err
	}
	_, _, end := tracker.Start(ctx, "load", "acp_chain", "source", chains.Source(), "id", chains.Default().ID)
	end()

	tools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(tracker),
		"local_fs": localtools.NewLocalFSToolsWith(
			"",
			db,
			acpsvc.NewACPFileIO(func() *acpsvc.Transport { return transport }),
			"local_fs",
			acpsvc.NewACPCwdResolver(func() *acpsvc.Transport { return transport }),
		),
		"local_shell": localtools.NewLocalExecToolsWith(
			acpsvc.NewACPCommandRunner(func() *acpsvc.Transport { return transport }),
		),
	}

	var askApproval localtools.AskApproval
	if enableHITL {
		askApproval = func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			if transport == nil {
				return false, fmt.Errorf("acpsvc: HITL approval requested before transport initialization")
			}
			return transport.AskApproval(ctx, req)
		}
	}

	// The engine requires a configured model: enginesvc.Build wires the embed/
	// task/chat executors and EnsureModels, all of which reject an empty model
	// name. When none is set yet we serve a setup-only transport instead of
	// hard-exiting: initialize/authenticate still work, so an ACP client can run
	// the "Setup Contenox" terminal auth method (`acp --setup`) to configure one.
	// Session creation returns an actionable error until then (see acpsvc).
	var engine *enginesvc.Engine
	if err := acpsvc.CleanupStaleACPManagedMCPServers(ctx, db); err != nil {
		return fmt.Errorf("cleanup stale ACP MCP servers: %w", err)
	}
	if defaultModel == "" {
		fmt.Fprintln(os.Stderr, "contenox acp: no default-model configured; serving setup-only. Run the \"Setup Contenox\" auth method or `contenox acp --setup` to configure a provider and model.")
	} else {
		cfg := enginesvc.Config{
			DefaultModel:       defaultModel,
			DefaultProvider:    defaultProvider,
			AltDefaultModel:    defaultAltModel,
			AltDefaultProvider: defaultAltProvider,
			LocalTools:         tools,
			Tracker:            tracker,
			WorkspaceID:        workspaceID,
		}
		if enableHITL {
			cfg.EnableHITL = true
			cfg.AskApproval = askApproval
			cfg.HITLPolicySource = acpPolicySource()
			cfg.HITLDefaultPolicyName = profile.hitlPolicy
		}

		engine, err = enginesvc.Build(ctx, db, cfg)
		if err != nil {
			return fmt.Errorf("build engine: %w", err)
		}
		defer engine.Stop()
	}

	updateBanner := acpUpdateBanner(dbCtx, db, contenoxDir)

	transportFactory := acpsvc.New(acpsvc.Deps{
		Engine:                engine,
		DB:                    db,
		ChainRegistry:         chains,
		DefaultModel:          defaultModel,
		DefaultProvider:       defaultProvider,
		DefaultAltModel:       defaultAltModel,
		DefaultAltProvider:    defaultAltProvider,
		DefaultMaxTokens:      defaultMaxTokens,
		DefaultThink:          defaultThink,
		WorkspaceID:           workspaceID,
		ContenoxDir:           contenoxDir,
		KnownPolicies:         embeddedPolicyNames(),
		HITLDefaultPolicyName: profile.hitlPolicy,
		UpdateBanner:          updateBanner,
	})

	conn := libacp.NewAgentSideConnection(acpStdio{}, func(c *libacp.AgentSideConnection) libacp.Agent {
		agent := transportFactory(c)
		transport = agent.(*acpsvc.Transport)
		return agent
	})

	runErr := conn.Run(ctx)
	if transport != nil {
		_ = transport.Close(context.Background())
	}
	if runErr != nil && runErr != io.EOF && runErr != context.Canceled {
		return fmt.Errorf("acp run: %w", runErr)
	}
	return nil
}

func acpPolicySource() hitlservice.PolicySource {
	home, err := os.UserHomeDir()
	if err != nil {
		return hitlservice.NewFSPolicySource()
	}
	return hitlservice.NewFSPolicySource(filepath.Join(home, ".contenox"))
}

// acpUpdateBanner checks for a newer contenox version and returns a one-line
// banner to surface in the ACP session, or "" when no update is available or
// the user has opted out via `contenox config set update-check false`.
// The check is non-blocking: it waits at most 500 ms so a cache hit is instant
// and a slow network call is silently skipped (will appear next session from cache).
func acpUpdateBanner(ctx context.Context, db libdb.DBManager, contenoxDir string) string {
	if acpsvc.ReadConfigValue(ctx, db, "update-check") == "false" {
		return ""
	}

	type result struct {
		tag       string
		available bool
	}
	ch := make(chan result, 1)
	go func() {
		tag, avail, err := updatecheck.IsAvailable(ctx, CLIVersion(), contenoxDir)
		if err != nil {
			ch <- result{}
			return
		}
		ch <- result{tag, avail}
	}()

	select {
	case r := <-ch:
		if !r.available {
			return ""
		}
		return fmt.Sprintf("contenox %s is available (current: %s) — run `contenox update` to upgrade.", r.tag, CLIVersion())
	case <-time.After(500 * time.Millisecond):
		return ""
	}
}
