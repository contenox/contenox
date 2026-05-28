package contenoxcli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/contenox/agent/libacp"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/acpsvc"
	"github.com/contenox/agent/runtime/enginesvc"
	"github.com/contenox/agent/runtime/hitlservice"
	"github.com/contenox/agent/runtime/localtools"
	"github.com/contenox/agent/runtime/runtimetypes"
	"github.com/contenox/agent/runtime/taskengine"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run as an Agent Client Protocol agent over stdio.",
	Long: `Speak Agent Client Protocol over stdio so editors like Zed can drive contenox as an agent.

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
		c.Flags().Bool("auto", false, "Autonomous mode: disable HITL permission prompts (gated tools run unattended)")
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
		return runSetup(cmd.OutOrStdout(), cmd.ErrOrStderr())
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

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

	contenoxDir, _ := ResolveContenoxDir(cmd)
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

	var tracker libtracker.ActivityTracker = libtracker.NewLogActivityTracker(slog.Default())
	var transport *acpsvc.Transport

	defaultModel := acpsvc.ReadConfigValue(ctx, db, "default-model")
	defaultProvider := acpsvc.ReadConfigValue(ctx, db, "default-provider")
	if defaultModel == "" {
		return fmt.Errorf("default-model is not configured; run `contenox config set default-model <name>` first")
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

	cfg := enginesvc.Config{
		DefaultModel:    defaultModel,
		DefaultProvider: defaultProvider,
		LocalTools:      tools,
		Tracker:         tracker,
		WorkspaceID:     workspaceID,
	}
	if enableHITL {
		cfg.EnableHITL = true
		cfg.AskApproval = askApproval
		cfg.HITLPolicySource = acpPolicySource()
		cfg.HITLDefaultPolicyName = profile.hitlPolicy
	}

	engine, err := enginesvc.Build(ctx, db, cfg)
	if err != nil {
		return fmt.Errorf("build engine: %w", err)
	}
	defer engine.Stop()

	transportFactory := acpsvc.New(acpsvc.Deps{
		Engine:          engine,
		DB:              db,
		ChainRegistry:   chains,
		DefaultModel:    defaultModel,
		DefaultProvider: defaultProvider,
		WorkspaceID:     workspaceID,
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
