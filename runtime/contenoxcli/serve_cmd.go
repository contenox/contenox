package contenoxcli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"os/signal"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	internaltools "github.com/contenox/runtime/runtime/internal/tools"
	internalweb "github.com/contenox/runtime/runtime/internal/web"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
	"github.com/contenox/runtime/runtime/version"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	defaultServeAddr = "127.0.0.1"
	defaultServePort = "32123"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Contenox HTTP server and Beam UI.",
	Long: `Start the Contenox HTTP API server.

Foundation routes:
  GET /health
  GET /version

The product API is served under /api (state, models, model-registry, backends,
setup-status, providers, tools, mcp-servers, task-chains, hitl-policies, chat,
task execution, approvals, task events). The Beam web UI is served at /.

The server binds to 127.0.0.1:32123 by default. Override with ADDR and PORT.
Set TOKEN to require a bearer token on mutating requests; TOKEN is mandatory
when ADDR is not a loopback address. Set BEAM_DEV_PROXY_URL to proxy Beam UI
requests to a Vite dev server while keeping /api on this server. A configured
model is required — run
` + "`contenox setup`" + ` first if you have not configured one.`,
	RunE: runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, stop := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Drain registered model-backend shutdown hooks on exit (no-op when none).
	defer func() { _ = modelrepo.Shutdown() }()

	config := &serverapi.Config{}
	if err := serverapi.LoadConfig(config); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.Addr == "" {
		config.Addr = defaultServeAddr
	}
	if config.Port == "" {
		config.Port = defaultServePort
	}
	config.Token = strings.TrimSpace(config.Token)
	if err := serverapi.ValidateLocalServeSecurity(config.Addr, config.Token); err != nil {
		return err
	}

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

	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve .contenox dir: %w", err)
	}
	workspaceID := ResolveWorkspaceID(contenoxDir)
	workspaceRoot := filepath.Dir(contenoxDir)
	nodeID := uuid.NewString()[:8]

	store := runtimetypes.New(db.WithoutTransaction())
	closeLogs, err := setupTelemetryLogging(ctx, store, contenoxDir)
	if err != nil {
		return fmt.Errorf("setup telemetry logging: %w", err)
	}
	defer closeLogs()

	opts, err := buildRunOpts(cmd, db, contenoxDir)
	if err != nil {
		return err
	}
	// serve offers the local_shell tool by default (the Beam chat workspace
	// expects it); it is gated by the HITL policy (EnableHITL below). Default
	// the exec allowed-dir to the workspace root so commands cannot escape it.
	opts.EffectiveEnableLocalExec = true
	if opts.EffectiveLocalExecAllowedDir == "" {
		opts.EffectiveLocalExecAllowedDir = workspaceRoot
	}

	var tracker libtracker.ActivityTracker = libtracker.NoopTracker{}
	if opts.EffectiveTracing {
		tracker = libtracker.NewLogActivityTracker(slog.Default())
	}

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	kvMgr := libkvstore.NewSQLiteManager(db)

	localTools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(tracker),
		"local_fs": localtools.NewLocalFSTools(opts.EffectiveLocalExecAllowedDir, db),
	}
	if opts.EffectiveEnableLocalExec {
		execOpts := []localtools.LocalExecOption{}
		if opts.EffectiveLocalExecAllowedDir != "" {
			execOpts = append(execOpts, localtools.WithLocalExecAllowedDir(opts.EffectiveLocalExecAllowedDir))
		}
		localTools["local_shell"] = localtools.NewLocalExecTools(execOpts...)
	}

	toolsRepo := internaltools.NewPersistentRepo(localTools, db, http.DefaultClient, bus, tracker)
	toolsProviderSvc := toolsproviderservice.New(db, toolsRepo, tracker)

	taskEventSink := taskengine.NewBusTaskEventSink(bus)
	hitlSource := hitlPolicySource(contenoxDir)
	hitlSvc := hitlservice.NewWithDefaultPolicy(hitlSource, runtimetypes.LocalTenantID, store, tracker, "")

	engine, err := enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:       opts.EffectiveDefaultModel,
		DefaultProvider:    opts.EffectiveDefaultProvider,
		AltDefaultModel:    opts.EffectiveAltDefaultModel,
		AltDefaultProvider: opts.EffectiveAltDefaultProvider,
		ContextLength:      opts.EffectiveContext,
		NoDeleteModels:     opts.EffectiveNoDeleteModels,
		LocalTools:         localTools,
		EnableHITL:         true,
		AskApproval: func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			return hitlSvc.RequestApproval(ctx, req, taskEventSink)
		},
		HITLService:      hitlSvc,
		Bus:              bus,
		KVStore:          kvMgr,
		Tracker:          tracker,
		Tracing:          opts.EffectiveTracing,
		TaskEventSink:    taskEventSink,
		WorkspaceID:      workspaceID,
		HITLPolicySource: hitlSource,
	})
	if err != nil {
		return fmt.Errorf("build engine (run `contenox setup` to configure a model): %w", err)
	}
	if engine.State == nil {
		return fmt.Errorf("build engine: runtime state is not configured")
	}
	defer engine.Stop()

	chainFiles, err := localfileservice.New(contenoxDir)
	if err != nil {
		return fmt.Errorf("chain files: %w", err)
	}
	chains := taskchainservice.NewLocal(chainFiles)
	agent := agentservice.New(agentservice.Deps{
		Engine:      engine,
		DB:          db,
		WorkspaceID: workspaceID,
	})
	chatMgr := chatservice.NewManager(workspaceID)

	apiMux := http.NewServeMux()
	cleanupAPI, err := serverapi.New(ctx, apiMux, nodeID, "local", config, serverapi.Dependencies{
		DB:                   db,
		PubSub:               bus,
		State:                engine.State,
		ToolsProviderService: toolsProviderSvc,
		Agent:                agent,
		ChatManager:          chatMgr,
		Chains:               chains,
		HITLService:          hitlSvc,
		WorkspaceID:          workspaceID,
		ProjectRoot:          workspaceRoot,
		ContenoxDir:          contenoxDir,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef:    firstNonEmptyStr(opts.EffectiveChain, "default-chain.json"),
			Model:       opts.EffectiveDefaultModel,
			Provider:    opts.EffectiveDefaultProvider,
			AltModel:    opts.EffectiveAltDefaultModel,
			AltProvider: opts.EffectiveAltDefaultProvider,
			MaxTokens:   opts.EffectiveMaxTokens,
			Think:       opts.EffectiveThink,
		},
	})
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}
	defer func() { _ = cleanupAPI() }()

	rootMux := http.NewServeMux()
	serverapi.AddHealthRoutes(rootMux)
	serverapi.AddVersionRoutes(rootMux, version.Get(), nodeID, "local")
	// Mutating /api/* requests require the bearer token (when configured); the
	// StripPrefix lets route packages register clean paths (/state, /models, ...).
	rootMux.Handle("/api/", http.StripPrefix("/api", serverapi.ProtectMutatingAPI(config.Token, apiMux)))
	uiHandler := internalweb.SPAHandler()
	if strings.TrimSpace(config.BeamDevProxyURL) != "" {
		devProxy, err := internalweb.DevProxyHandler(config.BeamDevProxyURL)
		if err != nil {
			return fmt.Errorf("beam dev proxy: %w", err)
		}
		uiHandler = devProxy
		fmt.Fprintf(cmd.OutOrStdout(), "Beam dev proxy: %s\n", config.BeamDevProxyURL)
	}
	rootMux.Handle("/", uiHandler)

	handler := middleware.EnableCORS(&middleware.CORSConfig{
		AllowedAPIOrigins: middleware.DefaultAllowedAPIOrigins,
		AllowedMethods:    middleware.DefaultAllowedMethods,
		AllowedHeaders:    middleware.DefaultAllowedHeaders,
	}, apiframework.RequestIDMiddleware(rootMux))

	srv := &http.Server{
		Addr:              net.JoinHostPort(config.Addr, config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(cmd.OutOrStdout(), "contenox serve %s ready: http://%s\n", version.Get(), srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
