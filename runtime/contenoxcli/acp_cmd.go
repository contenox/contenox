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

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/acpsvc"
	"github.com/contenox/contenox/runtime/enginesvc"
	"github.com/contenox/contenox/runtime/hitlservice"
	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/runtimetypes"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/contenox/contenox/runtime/vfsservice"
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

func init() {
	acpCmd.Flags().Bool("auto", false, "Autonomous mode: disable HITL permission prompts (gated tools run unattended)")
	acpCmd.Flags().Bool("setup", false, "Run interactive setup wizard to configure provider and model, then exit.")
	acpCmd.Flags().String("workspace-id", "", "Workspace ID for new ACP sessions (default: the stable workspace from ~/.contenox/workspace.id, same as the CLI). Existing sessions are always located by their session ID regardless of workspace.")
	acpCmd.Flags().Bool("experimental-acp", false, "Accepted for compatibility with ACP clients that hardcode this launch flag (e.g. AionUi); no effect.")
	_ = acpCmd.Flags().MarkHidden("experimental-acp")
}

type acpStdio struct{}

func (acpStdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (acpStdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (acpStdio) Close() error                { return os.Stdin.Close() }

func runACP(cmd *cobra.Command, _ []string) error {
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
		slog.Warn("Failed to seed HITL policy presets", "error", err)
	}

	closeLogs, err := setupTelemetryLogging(ctx, runtimetypes.New(db.WithoutTransaction()), contenoxDir)
	if err != nil {
		slog.Warn("Failed to setup telemetry logging", "error", err)
	}
	defer closeLogs()

	defaultModel := acpsvc.ReadConfigValue(ctx, db, "default-model")
	defaultProvider := acpsvc.ReadConfigValue(ctx, db, "default-provider")
	if defaultModel == "" {
		return fmt.Errorf("default-model is not configured; run `contenox config set default-model <name>` first")
	}

	chains, err := acpsvc.LoadChainRegistry()
	if err != nil {
		return err
	}
	slog.Info("loaded ACP chain", "source", chains.Source(), "id", chains.Default().ID)

	var tracker libtracker.ActivityTracker = libtracker.NewLogActivityTracker(slog.Default())
	var transport *acpsvc.Transport

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
		cfg.VFS = acpGlobalVFS()
		cfg.HITLDefaultPolicyName = "hitl-policy-acp.json"
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

func acpGlobalVFS() vfsservice.Service {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return vfsservice.NewLocalFS(filepath.Join(home, ".contenox"))
}
