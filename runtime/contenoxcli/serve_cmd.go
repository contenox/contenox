package contenoxcli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/internal/compatapi"
	internaltools "github.com/contenox/runtime/runtime/internal/tools"
	internalweb "github.com/contenox/runtime/runtime/internal/web"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/terminalservice"
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
	Short: "Start the Contenox HTTP server.",
	Long: `Start the Contenox HTTP API server.

This command exposes the foundation routes and the revived OSS API under /api:
  GET /health
  GET /version
  GET /api/state
  GET /api/models
  GET /api/model-registry
  GET /api/tools/local
  GET /api/mcp-servers
  GET /api/setup-status
  GET /api/openapi.json

The server binds to 127.0.0.1:32123 by default. Override with ADDR and PORT.
Terminal routes are enabled by default on local serve under /api/terminal/sessions.
Set TERMINAL_ENABLED=false to disable them. TERMINAL_ALLOWED_ROOT defaults to the
workspace root. The local_shell tool is enabled by default for serve; pass
--shell=false to disable it.`,
	RunE: runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, stop := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := &serverapi.Config{}
	if err := serverapi.LoadConfig(config); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ollamaCompat := configBool(config.OllamaCompat)
	if flag := cmd.Flags().Lookup("ollama-compat"); flag != nil {
		flagValue, _ := cmd.Flags().GetBool("ollama-compat")
		if flag.Changed || flagValue {
			ollamaCompat = flagValue
		}
	}

	addr := config.Addr
	if addr == "" {
		addr = defaultServeAddr
	}
	port := config.Port
	if port == "" {
		port = defaultServePort
	}
	if err := serverapi.ValidateLocalServeSecurity(addr, config.Token); err != nil {
		return err
	}

	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return err
	}
	db, err := OpenDBAt(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve .contenox dir: %w", err)
	}
	workspaceID := ResolveWorkspaceID(contenoxDir)
	workspaceRoot := filepath.Dir(contenoxDir)
	nodeID := uuid.NewString()[:8]

	tracker := libtracker.NoopTracker{}
	store := runtimetypes.New(db.WithoutTransaction())
	cleanupTelemetry, err := setupTelemetryLogging(ctx, store, contenoxDir)
	if err != nil {
		return err
	}
	defer cleanupTelemetry()

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()

	kvMgr := libkvstore.NewSQLiteManager(db)
	localTools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(tracker),
		"local_fs": localtools.NewLocalFSTools(workspaceRoot, db),
	}
	serveChatCfg, err := resolveServeChatConfig(ctx, cmd, store, workspaceID, contenoxDir)
	if err != nil {
		return err
	}
	if serveChatCfg.EnableLocalExec {
		execOpts := []localtools.LocalExecOption{}
		if serveChatCfg.LocalExecAllowedDir != "" {
			execOpts = append(execOpts, localtools.WithLocalExecAllowedDir(serveChatCfg.LocalExecAllowedDir))
		}
		localTools["local_shell"] = localtools.NewLocalExecTools(execOpts...)
	}
	toolsRepo := internaltools.NewPersistentRepo(localTools, db, http.DefaultClient, bus, tracker)
	toolsProviderSvc := toolsproviderservice.New(db, toolsRepo, tracker)

	taskEventSink := taskengine.NewBusTaskEventSink(bus)
	hitlSvc := hitlservice.NewWithDefaultPolicy(
		hitlPolicySource(contenoxDir),
		runtimetypes.LocalTenantID,
		store,
		tracker,
		"",
	)
	engine, err := enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:       serveChatCfg.DefaultModel,
		DefaultProvider:    serveChatCfg.DefaultProvider,
		AltDefaultModel:    serveChatCfg.AltDefaultModel,
		AltDefaultProvider: serveChatCfg.AltDefaultProvider,
		ContextLength:      serveChatCfg.ContextLength,
		NoDeleteModels:     serveChatCfg.NoDeleteModels,
		LocalTools:         localTools,
		EnableHITL:         true,
		AskApproval: func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			return hitlSvc.RequestApproval(ctx, req, taskEventSink)
		},
		HITLService:      hitlSvc,
		Bus:              bus,
		KVStore:          kvMgr,
		Tracker:          tracker,
		Tracing:          serveChatCfg.Tracing,
		TaskEventSink:    taskEventSink,
		WorkspaceID:      workspaceID,
		HITLPolicySource: hitlPolicySource(contenoxDir),
	})
	if err != nil {
		return fmt.Errorf("build chat engine: %w", err)
	}
	if engine.State == nil {
		return fmt.Errorf("build chat engine: runtime state is not configured")
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

	rootMux := http.NewServeMux()
	apiMux := http.NewServeMux()
	serverapi.AddHealthRoutes(rootMux)
	serverapi.AddVersionRoutes(rootMux, version.Get(), nodeID, "local")
	cleanupAPI, err := serverapi.New(ctx, apiMux, nodeID, "local", config, serverapi.Dependencies{
		DB:                   db,
		PubSub:               bus,
		State:                engine.State,
		ToolsProviderService: toolsProviderSvc,
		Agent:                agent,
		ChatManager:          chatMgr,
		Chains:               chains,
		HITLService:          hitlSvc,
		TerminalService:      terminalSvc,
		TerminalEnabled:      terminalCfg.Enabled,
		WorkspaceID:          workspaceID,
		ProjectRoot:          workspaceRoot,
		ContenoxDir:          contenoxDir,
		DefaultChainRef:      serveChatCfg.DefaultChainRef,
		DefaultModel:         serveChatCfg.DefaultModel,
		DefaultProvider:      serveChatCfg.DefaultProvider,
		AltDefaultModel:      serveChatCfg.AltDefaultModel,
		AltDefaultProvider:   serveChatCfg.AltDefaultProvider,
		DefaultMaxTokens:     serveChatCfg.DefaultMaxTokens,
		DefaultThink:         serveChatCfg.DefaultThink,
	})
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}
	defer func() { _ = cleanupAPI() }()

	compatStateSvc := stateservice.New(engine.State, db, workspaceID)
	compatDeps := compatapi.CompatDeps{
		Agent:              agent,
		Chains:             chains,
		StateService:       compatStateSvc,
		DefaultChainRef:    serveChatCfg.DefaultCompatChainRef,
		DefaultFIMChainRef: serveChatCfg.DefaultFIMChainRef,
		DefaultModel:       serveChatCfg.DefaultModel,
		DefaultProvider:    serveChatCfg.DefaultProvider,
		DefaultMaxTokens:   serveChatCfg.DefaultMaxTokens,
		Token:              config.Token,
		// Auth is nil: /api/openai POST routes go through ProtectMutatingAPI on apiMux.
		// Token protects root-level mutating compat routes such as /v1/chat/completions.
	}
	compatapi.AddOpenAIRoutes(apiMux, compatDeps)
	compatapi.AddRootRoutes(rootMux, compatDeps)
	if ollamaCompat {
		compatapi.AddOllamaRoutes(rootMux, compatDeps)
	}

	rootMux.Handle("/api/", http.StripPrefix("/api", serverapi.ProtectMutatingAPI(config.Token, apiMux)))
	rootMux.Handle("/", internalweb.SPAHandler())

	ln, err := listenWithOllamaFallback(addr, port, config.Port, ollamaCompat)
	if err != nil {
		return err
	}
	listenAddr := ln.Addr().String()

	handler := middleware.EnableCORS(&middleware.CORSConfig{
		AllowedAPIOrigins: middleware.DefaultAllowedAPIOrigins,
		AllowedMethods:    middleware.DefaultAllowedMethods,
		AllowedHeaders:    middleware.DefaultAllowedHeaders,
	}, apiframework.RequestIDMiddleware(rootMux))

	httpSrv := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
			return
		}
		errCh <- nil
	}()

	fmt.Fprintf(cmd.OutOrStdout(), "Contenox HTTP ready: http://%s\n", listenAddr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return err
	}
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
	cfg, err := terminalservice.ParseEnv(terminalEnabled, allowedRoot, config.TerminalShell, config.TerminalIdleTimeout)
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

type serveChatConfig struct {
	DefaultChainRef       string
	DefaultCompatChainRef string
	DefaultFIMChainRef    string
	DefaultModel          string
	DefaultProvider       string
	AltDefaultModel       string
	AltDefaultProvider    string
	DefaultMaxTokens      string
	DefaultThink          string
	ContextLength         int
	NoDeleteModels        bool
	EnableLocalExec       bool
	LocalExecAllowedDir   string
	Tracing               bool
}

func resolveServeChatConfig(ctx context.Context, cmd *cobra.Command, store runtimetypes.Store, workspaceID, contenoxDir string) (serveChatConfig, error) {
	flags := cmd.Root().Flags()

	kvModel, _ := getConfigKV(ctx, store, "default-model")
	kvProvider, _ := getConfigKV(ctx, store, "default-provider")
	kvAltModel, _ := getConfigKV(ctx, store, "default-alt-model")
	kvAltProvider, _ := getConfigKV(ctx, store, "default-alt-provider")
	kvCompatChain, _ := getConfigKV(ctx, store, "default-compat-chain")
	kvFIMChain, _ := getConfigKV(ctx, store, "default-fim-chain")
	maxTokens, err := resolveEffectiveMaxTokens(ctx, store, flags)
	if err != nil {
		return serveChatConfig{}, err
	}
	think, err := resolveEffectiveThink(ctx, store, flags)
	if err != nil {
		return serveChatConfig{}, err
	}

	model, _ := flags.GetString("model")
	if !flags.Changed("model") && (model == "" || model == defaultModel) {
		if kvModel != "" {
			model = kvModel
		} else {
			model = defaultModel
		}
	}

	provider := kvProvider
	if flags.Changed("provider") {
		provider, _ = flags.GetString("provider")
	}

	altModel := kvAltModel
	if flags.Changed("alt-model") {
		altModel, _ = flags.GetString("alt-model")
	}

	altProvider := kvAltProvider
	if flags.Changed("alt-provider") {
		altProvider, _ = flags.GetString("alt-provider")
	}

	chainRef, _ := flags.GetString("chain")
	if chainRef == "" && !flags.Changed("chain") {
		if kv, _ := clikv.ReadConfig(ctx, store, workspaceID, "default-chain"); kv != "" {
			chainRef = kv
		}
	}
	if chainRef == "" && !flags.Changed("chain") {
		if _, err := os.Stat(filepath.Join(contenoxDir, "default-chain.json")); err == nil {
			chainRef = "default-chain.json"
		}
	}
	chainRef = normalizeServeChainRef(contenoxDir, chainRef)

	contextLength, _ := flags.GetInt("context")
	noDeleteModels, _ := flags.GetBool("no-delete-models")
	enableLocalExec, _ := flags.GetBool("shell")
	if !flags.Changed("shell") {
		enableLocalExec = true
	}
	localExecAllowedDir, _ := flags.GetString("local-exec-allowed-dir")
	tracing, _ := flags.GetBool("trace")

	compatChainRef := kvCompatChain
	if compatChainRef == "" {
		compatChainRef = "chain-openai-compat.json"
	}
	fimChainRef := kvFIMChain
	if fimChainRef == "" {
		fimChainRef = "chain-fim-compat.json"
	}

	return serveChatConfig{
		DefaultChainRef:       chainRef,
		DefaultCompatChainRef: compatChainRef,
		DefaultFIMChainRef:    fimChainRef,
		DefaultModel:          model,
		DefaultProvider:       provider,
		AltDefaultModel:       altModel,
		AltDefaultProvider:    altProvider,
		DefaultMaxTokens:      maxTokens,
		DefaultThink:          think,
		ContextLength:         contextLength,
		NoDeleteModels:        noDeleteModels,
		EnableLocalExec:       enableLocalExec,
		LocalExecAllowedDir:   localExecAllowedDir,
		Tracing:               tracing,
	}, nil
}

const ollamaPort = "11434"

// listenWithOllamaFallback binds the server. In Ollama compatibility mode, when
// the user has not set PORT explicitly (configPort == ""), it first tries the
// Ollama default port 11434 so that tools expecting Ollama at that address work
// without configuration.
// If 11434 is taken it falls back to the resolved port (default 32123).
func listenWithOllamaFallback(addr, resolvedPort, configPort string, ollamaCompat bool) (net.Listener, error) {
	userSetPort := strings.TrimSpace(configPort) != ""
	if ollamaCompat && !userSetPort && resolvedPort != ollamaPort {
		if ln, err := net.Listen("tcp", net.JoinHostPort(addr, ollamaPort)); err == nil {
			return ln, nil
		}
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(addr, resolvedPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s:%s: %w; set PORT or ADDR to override", addr, resolvedPort, err)
	}
	return ln, nil
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on", "enabled":
		return true
	default:
		return false
	}
}

func normalizeServeChainRef(contenoxDir, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	clean := filepath.Clean(ref)
	if filepath.IsAbs(clean) {
		if rel, err := filepath.Rel(contenoxDir, clean); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return clean
	}
	slash := filepath.ToSlash(clean)
	if strings.HasPrefix(slash, ".contenox/") {
		return strings.TrimPrefix(slash, ".contenox/")
	}
	return slash
}
