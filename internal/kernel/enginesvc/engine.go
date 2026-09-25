package enginesvc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/kernel/tools"
	"github.com/contenox/contenox/internal/models/modelruntime"
	"github.com/contenox/contenox/internal/services/execservice"
	"github.com/contenox/contenox/internal/services/hitlservice"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/contenox/contenox/internal/services/mcpworker"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/contenox/contenox/internal/services/setupcheck"
	"github.com/contenox/contenox/internal/services/stateservice"
	"github.com/contenox/contenox/internal/services/toolguidance"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
)

// LocalTenantID re-exports runtimetypes.LocalTenantID for backwards compatibility.
const LocalTenantID = runtimetypes.LocalTenantID

func Build(ctx context.Context, db libdbexec.DBManager, cfg Config) (*Engine, error) {
	engineCtx, engineCancel := context.WithCancel(ctx)
	models, err := modelruntime.Build(engineCtx, db, modelruntime.Config{
		DefaultModel: cfg.DefaultModel, DefaultProvider: cfg.DefaultProvider,
		DefaultAudioModel: cfg.DefaultAudioModel, DefaultAudioProvider: cfg.DefaultAudioProvider,
		ContextLength: cfg.ContextLength, NoDeleteModels: cfg.NoDeleteModels,
		SkipBackendCycle: cfg.SkipBackendCycle, Tracing: cfg.Tracing, TenantID: cfg.TenantID,
		Bus: cfg.Bus, KVStore: cfg.KVStore, State: cfg.State, Tracker: cfg.Tracker,
	})
	if err != nil {
		engineCancel()
		return nil, err
	}
	engine := &Engine{Models: models.Models, AudioModel: models.AudioModel,
		State: models.State, Bus: models.Bus, Tracker: models.Tracker,
		Stop: func() { engineCancel(); models.Stop() },
	}
	success := false
	defer func() {
		if !success {
			engine.Stop()
		}
	}()
	state, bus, tracker, kvMgr := models.State, models.Bus, models.Tracker, models.KVStore
	repo := models.Models
	if cfg.WrapModels != nil {
		repo, err = cfg.WrapModels(repo, state)
		if err != nil {
			return nil, err
		}
		engine.Models = repo
	}
	ss := stateservice.New(state, db, cfg.WorkspaceID)
	setupStatus := func(ctx context.Context) (setupcheck.Result, error) {
		r, err := ss.SetupStatus(ctx)
		if err != nil {
			return setupcheck.Result{}, err
		}
		return setupcheck.OverlayEffectiveDefaults(r, cfg.ReadinessDefaultModel, cfg.ReadinessDefaultProvider), nil
	}
	res, err := setupStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup status failed: %w", err)
	}
	engine.SetupCheck = res
	engine.SetupStatus = setupStatus

	eventSink := cfg.TaskEventSink
	if eventSink == nil {
		eventSink = taskengine.NewBusTaskEventSink(bus)
	}
	cfg.TaskEventSink = eventSink

	mgr, localToolNames, toolsRepo, err := buildTools(engineCtx, cfg, db, tracker, bus)
	if err != nil {
		return nil, err
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
	engine.Tools = toolsRepo

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
	toolsRepo := tools.NewPersistentRepo(cfg.LocalTools, db, http.DefaultClient, bus, tracker)

	if cfg.EnableHITL {
		if cfg.AskApproval == nil {
			return nil, nil, nil, fmt.Errorf("enginesvc: EnableHITL is true but AskApproval callback is nil")
		}
		hitlSvc := cfg.HITLService
		if hitlSvc == nil {
			hitlTenant := cfg.TenantID
			if hitlTenant == "" {
				hitlTenant = runtimetypes.LocalTenantID
			}
			hitlSvc = hitlservice.NewWithDefaultPolicy(cfg.HITLPolicySource, hitlTenant, store, tracker, cfg.HITLDefaultPolicyName)
		}
		// Runs unconditionally, injected hitlSvc included: the evaluator must
		// read this engine's workspace-scoped policy row, not the global one.
		hitlservice.SetWorkspaceID(hitlSvc, cfg.WorkspaceID)
		// Mission tools are exempted from the HITL gate by construction: they
		// are the attention channel itself, and gating them would deadlock an
		// unattended unit asking for its own report.
		raw := toolsRepo
		toolsRepo = hitlExemptProviders(
			localtools.NewHITLWrapper(toolsRepo, cfg.AskApproval, hitlSvc, tracker, cfg.TaskEventSink),
			raw,
			missiontools.ToolsProviderName,
		)
	}

	// Position matters: after the HITL wrap, before the guidance wrap, so a
	// nested tool caller meets the same gate the model meets.
	if cfg.OnToolsRepoReady != nil {
		cfg.OnToolsRepoReady(toolsRepo)
	}

	// Sits outside the HITL wrapper so it counts only model-level calls, not
	// the gate's internal diff reads.
	toolsRepo = toolguidance.WrapFromEnv(toolsRepo)

	return mgr, localToolNames, toolsRepo, nil
}

type hitlExemptRepo struct {
	taskengine.ToolsRepo
	raw    taskengine.ToolsRepo
	exempt map[string]bool
}

func hitlExemptProviders(gated, raw taskengine.ToolsRepo, providers ...string) taskengine.ToolsRepo {
	exempt := make(map[string]bool, len(providers))
	for _, p := range providers {
		exempt[p] = true
	}
	return &hitlExemptRepo{ToolsRepo: gated, raw: raw, exempt: exempt}
}

func (h *hitlExemptRepo) Exec(ctx context.Context, startingTime time.Time, input any, debug bool, args *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if args != nil && h.exempt[args.Name] {
		return h.raw.Exec(ctx, startingTime, input, debug, args)
	}
	return h.ToolsRepo.Exec(ctx, startingTime, input, debug, args)
}
