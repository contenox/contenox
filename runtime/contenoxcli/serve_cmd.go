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
	"github.com/contenox/runtime/libacp"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	internaltools "github.com/contenox/runtime/runtime/internal/tools"
	internalweb "github.com/contenox/runtime/runtime/internal/web"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/contenox/runtime/runtime/shellsession"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/terminalservice"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
	"github.com/contenox/runtime/runtime/version"
	"github.com/contenox/runtime/runtime/vfs"
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
setup-status, providers, tools, mcp-servers, task-chains, hitl-policies, task
execution, task events, terminal sessions). Chat, its HITL approvals, and
execution-state/event replay run over the /acp WebSocket instead (see
'contenox acp'). The Beam web UI is served at /.

Terminal routes are enabled by default on local serve under /api/terminal/sessions.
Set TERMINAL_ENABLED=false to disable them. TERMINAL_ALLOWED_ROOT defaults to the
workspace root. TERMINAL_MAX_SESSIONS defaults to 8 (0 = unlimited).

The server binds to 127.0.0.1:32123 by default. Override with ADDR and PORT.
Set TOKEN to require a bearer token on mutating API requests and cross-origin
browser reads; TOKEN is mandatory when ADDR is not a loopback address. Set
BEAM_DEV_PROXY_URL to proxy Beam UI requests to a Vite dev server while keeping
/api on this server. A configured model is required — run
` + "`contenox setup`" + ` first if you have not configured one.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringArray("workspace-root", nil,
		"Directory a browser client may choose as a session workspace (repeatable). The serve directory is always the default; these extend the allowlist. Also settable via WORKSPACE_ROOTS (OS path-list separated) or as `contenox serve [dir]...` positional arguments.")
}

// buildWorkspaceFactory assembles the workspace-root allowlist. defaultRoot is
// the effective workspace root (the served project directory when one is given
// positionally, else home) and is always first, making it the Factory default.
// It already IS the first positional serve arg (resolved) when one was given, so
// only positional args BEYOND the first extend the allowlist here — home is
// never injected as an extra root when a project is served. --workspace-root
// flags and the WORKSPACE_ROOTS env (OS path-list separated) also extend it.
// Duplicates are collapsed by the Factory.
func buildWorkspaceFactory(cmd *cobra.Command, args []string, config *serverapi.Config, defaultRoot string) (*vfs.Factory, error) {
	roots := []string{defaultRoot}
	if len(args) > 1 {
		roots = append(roots, args[1:]...)
	}
	if flags, _ := cmd.Flags().GetStringArray("workspace-root"); len(flags) > 0 {
		roots = append(roots, flags...)
	}
	if config != nil {
		for _, r := range filepath.SplitList(config.WorkspaceRoots) {
			if strings.TrimSpace(r) != "" {
				roots = append(roots, r)
			}
		}
	}
	return vfs.NewFactory(roots...)
}

func runServe(cmd *cobra.Command, args []string) error {
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
	// The effective workspace root is the cwd for every chat session (native and
	// external), the local-exec allowed dir, the terminal allowed root, the
	// /files ProjectRoot, and the Factory default. When a project directory is
	// served positionally it BECOMES that root, bounding all of the above to the
	// project. Only the argument-less `contenox serve` falls back to the parent
	// of ~/.contenox (home), preserving today's behavior.
	workspaceRoot := filepath.Dir(contenoxDir)
	if len(args) > 0 {
		servedDir, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve serve directory %q: %w", args[0], err)
		}
		workspaceRoot = servedDir
	}
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

	// The workspace-root allowlist bounds what a browser client may choose as a
	// session's workspace. The serve directory is the default root, so behavior
	// is unchanged when nothing else is configured.
	workspaceFactory, err := buildWorkspaceFactory(cmd, args, config, workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve workspace roots: %w", err)
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
		// local_fs roots at the session's chosen workspace (its cwd), falling back
		// to the default workspace root for sessions without one. The fixed root is
		// intentionally empty so the cwd resolver, not a static dir, drives
		// containment — mirroring the stdio ACP path, but resolving the cwd from
		// the DB because serve shares one tool across many per-connection
		// transports (there is no single Transport to close over).
		"local_fs": localtools.NewLocalFSToolsWith(
			"", db, nil, "local_fs",
			acpsvc.NewServeCwdResolver(db, workspaceFactory.Default()),
		),
	}
	if opts.EffectiveEnableLocalExec {
		execOpts := []localtools.LocalExecOption{}
		if opts.EffectiveLocalExecAllowedDir != "" {
			execOpts = append(execOpts, localtools.WithLocalExecAllowedDir(opts.EffectiveLocalExecAllowedDir))
		}
		localTools["local_shell"] = localtools.NewLocalExecTools(execOpts...)
	}

	// The shell-session surface (persistent per-chat PTY behind the terminal
	// panel, the `!` passthrough, and the shell_session_run/read agent tools)
	// shares the same enablement gate as local_shell. Each shell is rooted at its
	// session's workspace via the same cwd resolver local_fs uses. When disabled,
	// the tools are absent and the terminal extension methods report
	// method-not-found — the feature is absent, not broken.
	var shellSessions shellsession.Manager
	if opts.EffectiveEnableLocalExec {
		shellSessions = shellsession.NewManager(shellsession.Config{
			CwdResolver: acpsvc.NewServeCwdResolver(db, workspaceFactory.Default()),
			DefaultRoot: workspaceFactory.Default(),
		})
		defer shellSessions.Shutdown()
		localTools[shellsession.ToolsProviderName] = shellsession.NewTools(shellSessions)
	}

	toolsRepo := internaltools.NewPersistentRepo(localTools, db, http.DefaultClient, bus, tracker)
	toolsProviderSvc := toolsproviderservice.New(db, toolsRepo, tracker)

	// Journal events durably per request (console scrollback evidence: diffs,
	// approvals, tool calls) in addition to the live bus stream served by SSE.
	taskEventSink := taskengine.NewKVJournalTaskEventSink(
		taskengine.NewBusTaskEventSink(bus), kvMgr, tracker)
	hitlSource := hitlPolicySource(contenoxDir)
	hitlSvc := hitlservice.NewWithDefaultPolicy(hitlSource, runtimetypes.LocalTenantID, store, tracker, "")

	// serve runs many ACP WebSocket connections (each its own acpsvc.Transport)
	// behind this SINGLE shared engine, so the engine's one AskApproval callback
	// cannot close over a single transport the way the stdio ACP path does. The
	// router is a stable shared object created BEFORE the engine (no late-binding
	// gymnastics): the per-connection transports built later (acpsvc.New below)
	// register their live sessions into it, and the AskApproval closure consults
	// it per request. Nil out of the box until a transport binds a session.
	permissionRouter := acpsvc.NewPermissionRouter()

	engine, err := enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:       opts.EffectiveDefaultModel,
		DefaultProvider:    opts.EffectiveDefaultProvider,
		AltDefaultModel:    opts.EffectiveAltDefaultModel,
		AltDefaultProvider: opts.EffectiveAltDefaultProvider,
		ContextLength:      opts.EffectiveContext,
		NoDeleteModels:     opts.EffectiveNoDeleteModels,
		LocalTools:         localTools,
		EnableHITL:         true,
		// Dispatch HITL approval per request: when the contenox session (from ctx)
		// is bound to a live beam ACP WS transport, bridge to that connection's
		// session/request_permission flow (beam's PermissionGate answers it).
		// Otherwise fall back to the approval-API path for headless/API callers,
		// which have no live ACP connection to prompt.
		AskApproval: func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			if allowed, err := permissionRouter.AskApproval(ctx, req); !errors.Is(err, acpsvc.ErrNoBoundSession) {
				return allowed, err
			}
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
	terminalCfg, err := resolveTerminalConfig(config, workspaceRoot)
	if err != nil {
		return err
	}
	terminalSvc, err := terminalservice.New(terminalCfg, db, nodeID, workspaceID)
	if err != nil {
		return fmt.Errorf("create terminal service: %w", err)
	}
	if terminalCfg.Enabled {
		terminalSvc = terminalservice.WithActivityTracker(terminalSvc, tracker)
		defer func() { _ = terminalSvc.CloseAll(context.Background()) }()
		stopTerminalReaper := startTerminalReaper(ctx, terminalSvc, terminalReapInterval(terminalCfg.IdleTimeout))
		defer stopTerminalReaper()
	}

	// /acp serves the same acpsvc agent `contenox acp` speaks over stdio, over
	// a WebSocket instead — see acp_ws.go. It reuses the engine/db/workspace
	// already built above; only the ACP chain registry is looked up fresh
	// (it lives outside enginesvc.Config). A missing chain file must not take
	// serve down: log and skip registering /acp rather than failing startup.
	var acpAgentFactory libacp.AgentFactory
	if acpChains, err := acpsvc.LoadChainRegistry(); err != nil {
		slog.Warn("contenox serve: /acp transport disabled: ACP chain registry unavailable", "error", err)
	} else {
		acpAgentFactory = acpsvc.New(acpsvc.Deps{
			Engine:             engine,
			DB:                 db,
			ChainRegistry:      acpChains,
			DefaultModel:       opts.EffectiveDefaultModel,
			DefaultProvider:    opts.EffectiveDefaultProvider,
			DefaultAltModel:    opts.EffectiveAltDefaultModel,
			DefaultAltProvider: opts.EffectiveAltDefaultProvider,
			DefaultMaxTokens:   opts.EffectiveMaxTokens,
			DefaultThink:       opts.EffectiveThink,
			WorkspaceID:        workspaceID,
			ContenoxDir:        contenoxDir,
			WorkspaceRoots:     workspaceFactory,
			ShellSessions:      shellSessions,
			// Share the same router the engine's AskApproval consults, so each WS
			// connection's transport registers its live sessions and gated tool
			// calls route back to the client that raised them.
			PermissionRouter: permissionRouter,
			KnownPolicies:    embeddedPolicyNames(),
			// serve passes no fallbackPolicy to hitlservice.NewWithDefaultPolicy
			// above ("" = the service's own built-in default), so there is no
			// named policy to report here either; HITLDefaultPolicyName is
			// display-only (the ACP /policy command).
			HITLDefaultPolicyName: "",
		})
	}

	apiMux := http.NewServeMux()
	cleanupAPI, err := serverapi.New(ctx, apiMux, nodeID, "local", config, serverapi.Dependencies{
		DB:                   db,
		PubSub:               bus,
		State:                engine.State,
		ToolsProviderService: toolsProviderSvc,
		Agent:                agent,
		Chains:               chains,
		TerminalService:      terminalSvc,
		TerminalEnabled:      terminalCfg.Enabled,
		WorkspaceID:          workspaceID,
		ProjectRoot:          workspaceRoot,
		ContenoxDir:          contenoxDir,
		WorkspaceRoots:       workspaceFactory,
		// Feed the /files `agent` view filter the same HITL policy source serve
		// uses so its verdicts match the live agent's gates. Fallback policy is ""
		// (the service's built-in default), mirroring hitlSvc above.
		HITLPolicySource:      hitlSource,
		HITLDefaultPolicyName: "",
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
	// When a TOKEN is configured, EVERY /api/* request (all methods, incl. GET)
	// requires a valid credential — a session-cookie JWT or the raw token as a
	// bearer — closing the same-origin-read hole. Without a token (loopback dev),
	// browser-originated mutations must be same-origin or explicitly allowed.
	// StripPrefix lets route packages register clean paths (/state, /models, ...).
	rootMux.Handle("/api/", http.StripPrefix("/api", serverapi.ProtectAPI(config.Token, config.AllowedAPIOrigins, apiMux)))
	// Beam remote-access login: /ui/login issues an HttpOnly session cookie for
	// the configured TOKEN, /ui/logout clears it, /ui/auth-status reports whether
	// login is required and the caller is authenticated. Registered directly on
	// the root mux (not under the /api protection wrapper) so /ui/login is
	// reachable before the browser holds the cookie it issues. stdio ACP is
	// unaffected — these routes exist only on the serve HTTP surface.
	serverapi.AddUIAuthRoutes(rootMux, config.Token)
	if acpAgentFactory != nil {
		rootMux.Handle("/acp", acpWebSocketHandler(acpAgentFactory, config.Token))
	}
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
		AllowedAPIOrigins: firstNonEmptyStr(config.AllowedAPIOrigins, middleware.DefaultAllowedAPIOrigins),
		AllowedMethods:    middleware.DefaultAllowedMethods,
		AllowedHeaders:    middleware.DefaultAllowedHeaders,
		ProxyOrigin:       config.ProxyOrigin,
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

func resolveTerminalConfig(config *serverapi.Config, workspaceRoot string) (terminalservice.Config, error) {
	if config == nil {
		config = &serverapi.Config{}
	}
	terminalEnabled := strings.TrimSpace(config.TerminalEnabled)
	if terminalEnabled == "" {
		terminalEnabled = "true"
	}
	allowedRoot := strings.TrimSpace(config.TerminalAllowedRoot)
	if terminalservice.IsEnabled(terminalEnabled) && allowedRoot == "" {
		allowedRoot = workspaceRoot
	}
	cfg, err := terminalservice.ParseEnv(
		terminalEnabled,
		allowedRoot,
		config.TerminalShell,
		config.TerminalIdleTimeout,
		config.TerminalMaxSessions,
	)
	if err != nil {
		return terminalservice.Config{}, err
	}
	return cfg, nil
}

func terminalReapInterval(idleTimeout time.Duration) time.Duration {
	if idleTimeout <= 0 {
		return 0
	}
	interval := time.Minute
	if half := idleTimeout / 2; half > 0 && half < interval {
		interval = half
	}
	if interval < time.Second {
		return time.Second
	}
	return interval
}

func startTerminalReaper(ctx context.Context, svc terminalservice.Service, interval time.Duration) func() {
	if svc == nil || interval <= 0 {
		return func() {}
	}
	reaperCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-reaperCtx.Done():
				return
			case <-ticker.C:
				_ = svc.ReapIdle(reaperCtx)
			}
		}
	}()
	return cancel
}
