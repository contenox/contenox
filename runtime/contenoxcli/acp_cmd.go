package contenoxcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/updatecheck"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/missiontools"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/presence"
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
		c.Flags().Bool("setup-web", false, "Serve the Beam setup wizard in the browser, exit once configured.")
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
	// forwardMissions wires the standalone `/mission` forwarder (fire a mission at
	// a running serve over REST). Enabled for the editor profile (acp — the Zed
	// journey); DISABLED for acpx: that profile is the hardened surface for an
	// untrusted driver (OpenClaw), which must not be handed a lever to dispatch
	// fleet units at the operator's serve.
	forwardMissions bool
}

var acpProfileACP = acpProfile{
	hitlPolicy:      "hitl-policy-acp.json",
	chainFile:       "default-acp-chain.json",
	chainEnv:        "CONTENOX_ACP_CHAIN_PATH",
	seedChain:       seedACPChainIfMissing,
	forwardMissions: true,
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

	if setupWeb, _ := cmd.Flags().GetBool("setup-web"); setupWeb {
		return runSetupWeb(ctx, cmd.OutOrStdout(), true)
	}
	// Deferred before the engine is built so it runs after engine teardown
	// (LIFO): drain registered model-backend shutdown hooks (e.g. native
	// in-process inference sessions) deterministically on exit. No-op when no
	// hook is registered.
	defer func() { _ = modelrepo.Shutdown() }()

	autoMode, _ := cmd.Flags().GetBool("auto")
	enableHITL := !autoMode

	workspaceFlag, _ := cmd.Flags().GetString("workspace-id")

	// Create tracker early for full startup telemetry (using text to stderr for ACP/CLI).
	// This instruments the entire launch so we can see phase timings and errors on freezes.
	var tracker libtracker.ActivityTracker = libtracker.NewTextActivityTracker(os.Stderr)

	reportErr, reportChange, endStartup := tracker.Start(ctx, "startup", "acp")
	defer endStartup()
	reportChange("phase", "flags_parsed")

	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		reportErr(err)
		return err
	}
	reportChange("phase", "resolve_db_path")
	dbCtx := libtracker.WithNewRequestID(ctx)
	db, err := OpenDBAt(dbCtx, dbPath)
	if err != nil {
		reportErr(err)
		return fmt.Errorf("open database %q: %w", dbPath, err)
	}
	defer db.Close()
	reportChange("phase", "open_db")

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
		reportErr(err)
		return fmt.Errorf("resolve contenox dir: %w", err)
	}
	reportChange("phase", "resolve_contenox_dir")
	workspaceID := workspaceFlag
	if workspaceID == "" {
		workspaceID = ResolveWorkspaceID(contenoxDir)
	}
	if err := writeEmbeddedHITLPolicies(contenoxDir, false); err != nil {
		reportErr(err)
		return fmt.Errorf("seed HITL policy presets: %w", err)
	}
	reportChange("phase", "seed_hitl")
	if profile.seedChain != nil {
		if err := profile.seedChain(contenoxDir); err != nil {
			reportErr(err)
			return fmt.Errorf("seed ACP chain preset: %w", err)
		}
	}
	reportChange("phase", "seed_chain")

	closeLogs, err := setupTelemetryLogging(ctx, runtimetypes.New(db.WithoutTransaction()), contenoxDir)
	if err != nil {
		reportErr(err)
		return fmt.Errorf("setup telemetry logging: %w", err)
	}
	defer closeLogs()
	reportChange("phase", "setup_telemetry")

	var transport *acpsvc.Transport

	// Environment-based setup: when nothing is configured yet but the launch
	// environment names a provider/model (an editor config or the ACP env_var
	// auth flow relaunching us), persist that configuration exactly as the
	// interactive wizard would. A failure is not fatal — we fall through to
	// setup-only mode, whose auth methods explain what is missing.
	if acpsvc.ReadConfigValue(ctx, db, "default-model") == "" &&
		(os.Getenv(envDefaultProvider) != "" || os.Getenv(envDefaultModel) != "") {
		if err := completeEnvSetup(ctx, db); err != nil {
			fmt.Fprintf(os.Stderr, "contenox acp: environment-based setup incomplete: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "contenox acp: configured provider/model from environment.")
		}
	}

	// Config reads are environment-first: a CONTENOX_DEFAULT_* variable
	// overrides the stored value for this process without persisting.
	defaultModel := configValueWithEnv(ctx, db, "default-model", envDefaultModel)
	defaultProvider := configValueWithEnv(ctx, db, "default-provider", envDefaultProvider)
	defaultAltModel := configValueWithEnv(ctx, db, "default-alt-model", envDefaultAltModel)
	defaultAltProvider := configValueWithEnv(ctx, db, "default-alt-provider", envDefaultAltProvider)
	defaultMaxTokens, err := normalizeMaxTokensConfig(configValueWithEnv(ctx, db, "default-max-tokens", envDefaultMaxTokens))
	if err != nil {
		return err
	}
	defaultThink := reasoning.Default
	if configuredThink := configValueWithEnv(ctx, db, "default-think", envDefaultThink); configuredThink != "" {
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
		// Mission tools: the per-mission report/ask-for-attention channel a
		// dispatched unit holds while running unattended. Registered here because
		// a fleet unit IS a `contenox acp` subprocess, and the mission tools are
		// its OWN local providers writing to the shared mission store (this db,
		// under the same $HOME/.contenox the dispatcher reads). The grant is still
		// per-mission: the provider exposes nothing and executes nothing unless the
		// session was constructed with a mission id (session/new `_meta`), so an
		// ordinary editor session over `contenox acp` never sees them. The nil
		// asker means mission_ask_attention records a durable blocker report rather
		// than a durable permission ask — wiring the ask to the operator inbox from
		// a viewer-less unit is fleet-consolidation.md's M5, a separate slice.
		//
		// The mission service MUST carry an event publisher: report routing runs in
		// the DISPATCHER's process (serve's reportrouter), subscribed to the SQLite
		// bus over this same shared $HOME/.contenox/local.db, and it routes purely
		// off ReportAddedEvent. A publisher-less service here would store a unit's
		// report durably but never publish, so the supervision edge would silently
		// go nowhere — the exact cross-process seam the composed round-trip e2e
		// (acpsvc/e2e_mission_roundtrip_test.go) exists to keep closed.
		missiontools.ToolsProviderName: missiontools.New(
			missionservice.New(db, missionservice.WithEventPublisher(libbus.NewSQLite(db.WithoutTransaction()))),
			nil,
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

	// Fleet presence store (shared-SQLite over the same $HOME/.contenox/local.db
	// serve reads): serve REGISTERS its reachable address here, and a forwarding
	// `/mission` session DISCOVERS it here. One store, used by the mission
	// forwarder just below and the presence reporter further down.
	presenceStore := presence.NewStore(libkvstore.NewSQLiteManager(db))

	// Mission forwarding: in a standalone `contenox acp` session (what Zed spawns)
	// `/mission` fires at a running serve over its REST API — discovered lazily
	// (CONTENOX_SERVER_URL → serve's presence row → the loopback default) and
	// health-probed, so the command is advertised per session exactly when a serve
	// is reachable. The pair is the narrow MissionDispatcher/MissionAgentResolver
	// acpsvc needs; MissionForwarded marks it REMOTE so acpsvc's advertisement,
	// fired-mission confirmation, and teaching error stay forwarding-honest (see
	// runtime/acpsvc/mission.go). Gated to the editor profile — acpx (untrusted
	// driver) is deliberately never handed a lever to dispatch at the operator's serve.
	var (
		missionFleet   acpsvc.MissionDispatcher
		missionAgents  acpsvc.MissionAgentResolver
		missionForward *acpsvc.MissionForwardConfig
	)
	if profile.forwardMissions {
		fwd := newMissionForwarder(presenceStore)
		missionFleet, missionAgents, missionForward = fwd, fwd, fwd.forwardConfig()
	}

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
		// Standalone `/mission` forwarding to a running serve (nil on the acpx
		// profile, and nil-safe throughout acpsvc when not wired).
		Fleet:            missionFleet,
		Agents:           missionAgents,
		MissionForwarded: missionForward,
		EnvSetup: &acpsvc.EnvSetupSpec{
			Vars: acpEnvSetupVars(),
			Complete: func(cctx context.Context) error {
				return completeEnvSetup(cctx, db)
			},
		},
	})

	// Fleet presence: make THIS editor-spawned process visible on the fleet board.
	// It self-registers into the shared-SQLite presence store (the same $HOME/
	// .contenox/local.db serve reads), heartbeating on a modest interval and on
	// session events. Entirely best-effort — a presence write never blocks or
	// fails serving the editor (see runtime/presence). The decorator around the
	// transport feeds it the client name (from initialize) and the open-session
	// count without acpsvc needing to know presence exists.
	acpCwd, _ := os.Getwd()
	presenceReporter := presence.StartReporter(ctx, presenceStore, presence.Record{
		Kind: presence.KindACP,
		Cwd:  acpCwd,
	})
	defer presenceReporter.Stop()

	conn := libacp.NewAgentSideConnection(acpStdio{}, func(c *libacp.AgentSideConnection) libacp.Agent {
		agent := transportFactory(c)
		transport = agent.(*acpsvc.Transport)
		return newPresenceAgent(agent, presenceReporter)
	})

	runErr := conn.Run(ctx)
	if transport != nil {
		_ = transport.Close(context.Background())
	}
	if runErr != nil && !errors.Is(runErr, io.EOF) && !errors.Is(runErr, context.Canceled) {
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
