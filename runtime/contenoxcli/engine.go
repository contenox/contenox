package contenoxcli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	libbus "github.com/contenox/contenox/libbus"
	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libkvstore"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/execservice"
	"github.com/contenox/contenox/runtime/hitlservice"
	"github.com/contenox/contenox/runtime/internal/llmrepo"
	"github.com/contenox/contenox/runtime/internal/ollamatokenizer"
	"github.com/contenox/contenox/runtime/internal/runtimestate"
	"github.com/contenox/contenox/runtime/internal/setupcheck"
	"github.com/contenox/contenox/runtime/internal/tools"
	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/mcpworker"
	"github.com/contenox/contenox/runtime/runtimetypes"
	"github.com/contenox/contenox/runtime/stateservice"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/contenox/contenox/runtime/vfsservice"
)

type Engine struct {
	TaskService execservice.TasksEnvService
	Tracker     libtracker.ActivityTracker
	Stop        func()
	Bus         libbus.Messenger
	MCPManager  *mcpworker.Manager
	// LocalTools lists the names of all registered local tools handlers.
	LocalTools []string
	// SetupCheck is the last SetupStatus evaluation after RunBackendCycle (for resolver-failure hints).
	SetupCheck setupcheck.Result
}

// BuildEngine scaffolds the complex dependency graph needed to run task chains.
func BuildEngine(ctx context.Context, db libdbexec.DBManager, opts chatOpts, vfs vfsservice.Service) (*Engine, error) {
	// Derive a cancellable context owned by this engine instance.
	// Cancelling it unblocks all goroutines (WatchEvents, bus streams, etc.)
	// before bus.Close() is called, preventing the process from hanging.
	engineCtx, engineCancel := context.WithCancel(ctx)

	// SQLite-backed bus (same architecture as runtime-API, just without NATS)
	bus := libbus.NewSQLite(db.WithoutTransaction())

	// Armed defer: if we return early on error, cancel the engine context and
	// close the bus so no goroutines are leaked.
	success := false
	defer func() {
		if !success {
			engineCancel()
			bus.Close()
		}
	}()

	engine := &Engine{Stop: func() {
		engineCancel() // signal all goroutines to stop
		bus.Close()
	}, Bus: bus}

	// Runtime state — always enable auto-discover for the CLI so users don't
	// need to run 'model add' before using Ollama, OpenAI or vLLM models.
	// The fleet-management runtime-api (Dockerfile) does NOT use this option.
	stateOpts := []runtimestate.Option{
		runtimestate.WithAutoDiscoverModels(),
	}
	if opts.EffectiveNoDeleteModels {
		stateOpts = append(stateOpts, runtimestate.WithSkipDeleteUndeclaredModels())
	}
	// Wire the SQLite-backed KV store so the provider model-list cache (Gemini/OpenAI)
	// survives across CLI invocations.
	kvMgr := libkvstore.NewSQLiteManager(db)
	stateOpts = append(stateOpts, runtimestate.WithKVStore(kvMgr), runtimestate.WithAutoDiscoverModels())
	state, err := runtimestate.New(engineCtx, db, bus, stateOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime state: %w", err)
	}

	// 4. Initialize embed/task/chat groups
	config := &runtimestate.Config{
		TenantID:   localTenantID,
		EmbedModel: opts.EffectiveDefaultModel,
		TaskModel:  opts.EffectiveDefaultModel,
		ChatModel:  opts.EffectiveDefaultModel,
	}
	if err := runtimestate.InitEmbeder(ctx, config, db, opts.EffectiveContext, state); err != nil {
		return nil, fmt.Errorf("failed to init embedder: %w", err)
	}
	if err := runtimestate.InitPromptExec(ctx, config, db, state, opts.EffectiveContext); err != nil {
		return nil, fmt.Errorf("failed to init prompt executor: %w", err)
	}
	if err := runtimestate.InitChatExec(ctx, config, db, state, opts.EffectiveContext); err != nil {
		return nil, fmt.Errorf("failed to init chat executor: %w", err)
	}

	// 4b. Keep an internal row for the effective default model so bootstrap groups
	// and local overrides keep working, even though OSS no longer exposes model CRUD.
	specs := []runtimestate.ExtraModelSpec{
		{
			Name:          opts.EffectiveDefaultModel,
			ContextLength: opts.EffectiveContext, // 0 = unknown, resolver won't filter on context
			CanChat:       true,
			CanPrompt:     true,
			CanEmbed:      false,
		},
	}
	if len(specs) > 0 {
		if err := runtimestate.EnsureModels(ctx, db, localTenantID, specs); err != nil {
			return nil, fmt.Errorf("failed to ensure models: %w", err)
		}
	}

	// 5. Backends are already in SQLite from `contenox backend add`; just run the sync cycle.
	// 6. Run backend cycle
	if !opts.EffectiveSkipBackendCycle {
		if opts.EffectiveTracing {
			slog.Info("Running backend cycle to sync models...")
		}
		if err := state.RunBackendCycle(ctx); err != nil {
			slog.Warn("Backend cycle encountered errors", "error", err)
		}
	}
	rt := state.Get(ctx)
	anyReachable := false
	for id, bs := range rt {
		if bs.Error != "" {
			if opts.EffectiveTracing {
				slog.Warn("Backend unreachable", "id", id, "url", bs.Backend.BaseURL, "error", bs.Error)
			}
		} else {
			anyReachable = true
		}
	}
	if !anyReachable && opts.EffectiveTracing {
		slog.Warn("No reachable backends – subsequent model operations may fail")
	}

	ss := stateservice.New(state, db, ResolveWorkspaceID(opts.ContenoxDir))
	res, err := ss.SetupStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup status failed: %w", err)
	}
	engine.SetupCheck = res

	// 7. Tokenizer and model manager
	tokenizer := ollamatokenizer.NewEstimateTokenizer()
	var tracker libtracker.ActivityTracker
	if opts.EffectiveTracing {
		tracker = libtracker.NewLogActivityTracker(slog.Default())
	} else {
		tracker = libtracker.NoopTracker{}
	}
	repo, err := llmrepo.NewModelManager(state, tokenizer, llmrepo.ModelManagerConfig{
		DefaultPromptModel:    llmrepo.ModelConfig{Name: opts.EffectiveDefaultModel, Provider: opts.EffectiveDefaultProvider},
		DefaultEmbeddingModel: llmrepo.ModelConfig{Name: opts.EffectiveDefaultModel, Provider: opts.EffectiveDefaultProvider},
		DefaultChatModel:      llmrepo.ModelConfig{Name: opts.EffectiveDefaultModel, Provider: opts.EffectiveDefaultProvider},
	}, tracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}
	mgr, localToolNames, toolsRepo, err := buildTools(engineCtx, opts, db, ResolveWorkspaceID(opts.ContenoxDir), tracker, bus, vfs)
	if err != nil {
		return nil, err
	}

	exec, err := taskengine.NewExec(engineCtx, repo, toolsRepo, tracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create task executor: %w", err)
	}
	var inspector taskengine.Inspector = taskengine.NewSimpleInspector()
	inspector = taskengine.NewKVInspector(inspector, kvMgr, tracker)
	inspector = taskengine.NewBusInspector(inspector, bus, tracker)
	envExec, err := taskengine.NewEnv(engineCtx, tracker, exec, inspector, toolsRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create environment executor: %w", err)
	}
	envExec, err = taskengine.NewMacroEnv(envExec, toolsRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create macro environment: %w", err)
	}
	taskService := execservice.NewTasksEnv(engineCtx, envExec, toolsRepo)

	engine.TaskService = taskService
	engine.Tracker = tracker
	engine.MCPManager = mgr
	engine.LocalTools = localToolNames

	oldStop := engine.Stop
	engine.Stop = func() {
		mgr.StopAll() // terminates all stdio MCP child processes
		oldStop()
	}
	success = true
	return engine, nil
}

func buildTools(engineCtx context.Context, opts chatOpts, db libdbexec.DBManager, workspaceID string, tracker libtracker.ActivityTracker, bus libbus.Messenger, vfs vfsservice.Service) (*mcpworker.Manager, []string, taskengine.ToolsRepo, error) {
	// LocalTools maps are separated for security and context boundaries:
	// - localTools: Available directly to the native LLM context (e.g. executing tasks).
	// - jsTools: Exposed specifically to the JS sandbox environment (macro executions).
	//   Includes sandbox-safe tools and those needed by scripts (e.g. ssh, webtools),
	//   preventing JS from accessing privileged host tools unless explicitly allowed.
	localTools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(),
		"local_fs": localtools.NewLocalFSTools(opts.EffectiveLocalExecAllowedDir),
	}
	jsTools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(),
	}
	if sshTools, err := localtools.NewSSHTools(); err != nil {
		slog.Debug("SSH tools not registered", "error", err)
	} else {
		jsTools["ssh"] = sshTools
	}
	if opts.EffectiveEnableLocalExec {
		toolsOpts := []localtools.LocalExecOption{}
		if opts.EffectiveLocalExecAllowedDir != "" {
			toolsOpts = append(toolsOpts, localtools.WithLocalExecAllowedDir(opts.EffectiveLocalExecAllowedDir))
		}
		localExecTools := localtools.NewLocalExecTools(toolsOpts...)
		jsTools["local_shell"] = localExecTools
		localTools["local_shell"] = localExecTools
	}

	// Start mcpworker.Manager — loads MCP servers from SQLite and serves them
	// via the SQLite bus. This is the same code path as the runtime-API (which uses NATS).
	store := runtimetypes.New(db.WithoutTransaction())
	mgr, err := mcpworker.New(engineCtx, store, bus, tracker)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create mcp worker manager: %w", err)
	}
	if err := mgr.WatchEvents(engineCtx); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start mcp event watcher: %w", err)
	}

	var localToolNames []string
	for name := range localTools {
		localToolNames = append(localToolNames, name)
	}
	toolsRepo := tools.NewPersistentRepo(localTools, db, http.DefaultClient, bus)

	if opts.EffectiveHITL {
		hitlVFS := newLayeredHITLVFS(vfs)
		hitlSvc := hitlservice.New(hitlVFS, store, tracker)
		toolsRepo = localtools.NewHITLWrapper(toolsRepo, NewCLIAskApproval(os.Stderr), hitlSvc, tracker)
	} else if opts.EffectiveEnableLocalExec && opts.EffectiveLocalExecAllowedDir == "" {
		// local_shell is wired without HITL and without a static allowed-dir.
		// Chain JSON tools_policies (allowlist) is the only remaining gate; if
		// the active chain doesn't define one, every shell command runs unchecked.
		slog.Warn("local_shell is enabled with no HITL and no allowed-dir; chain-level tools_policies is the only safety gate — confirm your chain JSON sets local_shell._allowed_commands or _allowed_dir")
	}

	return mgr, localToolNames, toolsRepo, nil
}

