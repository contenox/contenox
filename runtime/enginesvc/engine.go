package enginesvc

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	libbus "github.com/contenox/agent/libbus"
	"github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/libkvstore"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/execservice"
	"github.com/contenox/agent/runtime/hitlservice"
	"github.com/contenox/agent/runtime/llmrepo"
	"github.com/contenox/agent/runtime/ollamatokenizer"
	"github.com/contenox/agent/runtime/runtimestate"
	"github.com/contenox/agent/runtime/internal/tools"
	"github.com/contenox/agent/runtime/localtools"
	"github.com/contenox/agent/runtime/mcpworker"
	"github.com/contenox/agent/runtime/runtimetypes"
	"github.com/contenox/agent/runtime/stateservice"
	"github.com/contenox/agent/runtime/taskengine"
)

const LocalTenantID = "00000000-0000-0000-0000-000000000001"

func Build(ctx context.Context, db libdbexec.DBManager, cfg Config) (*Engine, error) {
	engineCtx, engineCancel := context.WithCancel(ctx)

	bus := cfg.Bus
	ownsBus := false
	if bus == nil {
		bus = libbus.NewSQLite(db.WithoutTransaction())
		ownsBus = true
	}

	closeBus := func() {
		if ownsBus {
			bus.Close()
		}
	}

	success := false
	defer func() {
		if !success {
			engineCancel()
			closeBus()
		}
	}()

	engine := &Engine{Stop: func() {
		engineCancel()
		closeBus()
	}, Bus: bus}

	stateOpts := []runtimestate.Option{
		runtimestate.WithAutoDiscoverModels(),
	}
	if cfg.NoDeleteModels {
		stateOpts = append(stateOpts, runtimestate.WithSkipDeleteUndeclaredModels())
	}
	kvMgr := cfg.KVStore
	if kvMgr == nil {
		kvMgr = libkvstore.NewSQLiteManager(db)
	}
	stateOpts = append(stateOpts, runtimestate.WithKVStore(kvMgr), runtimestate.WithAutoDiscoverModels())
	state, err := runtimestate.New(engineCtx, db, bus, stateOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime state: %w", err)
	}

	config := &runtimestate.Config{
		TenantID:   LocalTenantID,
		EmbedModel: cfg.DefaultModel,
		TaskModel:  cfg.DefaultModel,
		ChatModel:  cfg.DefaultModel,
	}
	if err := runtimestate.InitEmbeder(ctx, config, db, cfg.ContextLength, state); err != nil {
		return nil, fmt.Errorf("failed to init embedder: %w", err)
	}
	if err := runtimestate.InitPromptExec(ctx, config, db, state, cfg.ContextLength); err != nil {
		return nil, fmt.Errorf("failed to init prompt executor: %w", err)
	}
	if err := runtimestate.InitChatExec(ctx, config, db, state, cfg.ContextLength); err != nil {
		return nil, fmt.Errorf("failed to init chat executor: %w", err)
	}

	specs := []runtimestate.ExtraModelSpec{
		{
			Name:          cfg.DefaultModel,
			ContextLength: cfg.ContextLength,
			CanChat:       true,
			CanPrompt:     true,
			CanEmbed:      false,
		},
	}
	if err := runtimestate.EnsureModels(ctx, db, LocalTenantID, specs); err != nil {
		return nil, fmt.Errorf("failed to ensure models: %w", err)
	}

	if !cfg.SkipBackendCycle {
		if cfg.Tracing {
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
			if cfg.Tracing {
				slog.Warn("Backend unreachable", "id", id, "url", bs.Backend.BaseURL, "error", bs.Error)
			}
		} else {
			anyReachable = true
		}
	}
	if !anyReachable && cfg.Tracing {
		slog.Warn("No reachable backends – subsequent model operations may fail")
	}

	ss := stateservice.New(state, db, cfg.WorkspaceID)
	res, err := ss.SetupStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup status failed: %w", err)
	}
	engine.SetupCheck = res

	tokenizer := ollamatokenizer.NewEstimateTokenizer()
	tracker := cfg.Tracker
	if tracker == nil {
		if cfg.Tracing {
			tracker = libtracker.NewLogActivityTracker(slog.Default())
		} else {
			tracker = libtracker.NoopTracker{}
		}
	}

	repo, err := llmrepo.NewModelManager(state, tokenizer, llmrepo.ModelManagerConfig{
		DefaultPromptModel:    llmrepo.ModelConfig{Name: cfg.DefaultModel, Provider: cfg.DefaultProvider},
		DefaultEmbeddingModel: llmrepo.ModelConfig{Name: cfg.DefaultModel, Provider: cfg.DefaultProvider},
		DefaultChatModel:      llmrepo.ModelConfig{Name: cfg.DefaultModel, Provider: cfg.DefaultProvider},
	}, tracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}

	mgr, localToolNames, toolsRepo, err := buildTools(engineCtx, cfg, db, tracker, bus)
	if err != nil {
		return nil, err
	}

	eventSink := cfg.TaskEventSink
	if eventSink == nil {
		eventSink = taskengine.NewBusTaskEventSink(bus)
	}
	execCtx := taskengine.WithTaskEventSink(engineCtx, eventSink)

	exec, err := taskengine.NewExec(execCtx, repo, toolsRepo, tracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create task executor: %w", err)
	}
	var inspector taskengine.Inspector = taskengine.NewSimpleInspector()
	inspector = taskengine.NewKVInspector(inspector, kvMgr, tracker)
	inspector = taskengine.NewBusInspector(inspector, bus, tracker)
	for _, wrap := range cfg.ExtraInspectors {
		inspector = wrap(inspector)
	}
	envExec, err := taskengine.NewEnv(execCtx, tracker, exec, inspector, toolsRepo)
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
	engine.TaskEventSink = eventSink
	engine.MCPManager = mgr
	engine.LocalTools = localToolNames

	oldStop := engine.Stop
	engine.Stop = func() {
		mgr.StopAll()
		oldStop()
	}
	success = true
	return engine, nil
}

func buildTools(engineCtx context.Context, cfg Config, db libdbexec.DBManager, tracker libtracker.ActivityTracker, bus libbus.Messenger) (*mcpworker.Manager, []string, taskengine.ToolsRepo, error) {
	store := runtimetypes.New(db.WithoutTransaction())
	mgr, err := mcpworker.New(engineCtx, store, bus, tracker)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create mcp worker manager: %w", err)
	}
	if err := mgr.WatchEvents(engineCtx); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start mcp event watcher: %w", err)
	}

	localToolNames := make([]string, 0, len(cfg.LocalTools))
	for name := range cfg.LocalTools {
		localToolNames = append(localToolNames, name)
	}
	toolsRepo := tools.NewPersistentRepo(cfg.LocalTools, db, http.DefaultClient, bus)

	if cfg.EnableHITL {
		if cfg.AskApproval == nil {
			return nil, nil, nil, fmt.Errorf("enginesvc: EnableHITL is true but AskApproval callback is nil")
		}
		hitlSvc := cfg.HITLService
		if hitlSvc == nil {
			hitlVFS := cfg.VFS
			if cfg.FallbackVFS != nil {
				hitlVFS = newLayered(cfg.VFS, cfg.FallbackVFS)
			}
			hitlSvc = hitlservice.NewWithDefaultPolicy(hitlVFS, store, tracker, cfg.HITLDefaultPolicyName)
		}
		toolsRepo = localtools.NewHITLWrapper(toolsRepo, cfg.AskApproval, hitlSvc, tracker)
	}

	return mgr, localToolNames, toolsRepo, nil
}
