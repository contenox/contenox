package modelruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/ollamatokenizer"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/substrate"
	"github.com/contenox/contenox/libbus"
	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libkvstore"
	"github.com/contenox/contenox/libtracker"
)

// Config configures model discovery, routing and their backing resources.
type Config struct {
	DefaultModel         string
	DefaultProvider      string
	DefaultAudioModel    string
	DefaultAudioProvider string
	ContextLength        int
	NoDeleteModels       bool
	SkipBackendCycle     bool
	Tracing              bool
	TenantID             string
	Bus                  libbus.Messenger
	KVStore              libkvstore.KVManager
	State                *runtimestate.State
	Tracker              libtracker.ActivityTracker
}

// Runtime owns model-serving resources without constructing an agent or tools.
// Stop releases only resources created by Build, not injected resources.
type Runtime struct {
	Models     llmrepo.ModelRepo
	AudioModel llmrepo.ModelConfig
	State      *runtimestate.State
	Bus        libbus.Messenger
	KVStore    libkvstore.KVManager
	Tracker    libtracker.ActivityTracker
	Stop       func()
}

// Build initializes the model runtime shared by the harness and HTTP gateway.
func Build(ctx context.Context, db libdbexec.DBManager, cfg Config) (*Runtime, error) {
	engineCtx, engineCancel := context.WithCancel(ctx)

	bus := cfg.Bus
	ownsBus := false
	if bus == nil {
		opened, err := substrate.OpenBus(engineCtx, db.WithoutTransaction())
		if err != nil {
			engineCancel()
			return nil, err
		}
		bus, ownsBus = opened, true
	}

	closeBus := func() {
		if ownsBus {
			bus.Close()
		}
	}

	releaseKV := func() {}

	success := false
	defer func() {
		if !success {
			engineCancel()
			closeBus()
			releaseKV()
		}
	}()

	kvMgr := cfg.KVStore
	if kvMgr == nil {
		opened, release, err := substrate.OpenKV(engineCtx, db)
		if err != nil {
			return nil, err
		}
		kvMgr, releaseKV = opened, release
	}

	state := cfg.State
	if state == nil {
		stateOpts := []runtimestate.Option{
			runtimestate.WithKVStore(kvMgr),
			runtimestate.WithAutoDiscoverModels(),
		}
		if cfg.NoDeleteModels {
			stateOpts = append(stateOpts, runtimestate.WithSkipDeleteUndeclaredModels())
		}
		var err error
		state, err = runtimestate.New(engineCtx, db, bus, stateOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create runtime state: %w", err)
		}
	}
	engine := &Runtime{Stop: func() {
		engineCancel()
		closeBus()
		releaseKV()
	}, Bus: bus, State: state}

	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = runtimetypes.LocalTenantID
	}
	if cfg.DefaultModel != "" {
		config := &runtimestate.Config{
			TenantID:   tenantID,
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
		if err := runtimestate.EnsureModels(ctx, db, tenantID, specs); err != nil {
			return nil, fmt.Errorf("failed to ensure models: %w", err)
		}

	}

	tracker := cfg.Tracker
	if tracker == nil {
		if cfg.Tracing {
			tracker = libtracker.NewLogActivityTracker(nil)
		} else {
			tracker = libtracker.NoopTracker{}
		}
	}

	if !cfg.SkipBackendCycle {
		cycleReportErr, _, cycleEnd := tracker.Start(ctx, "sync", "backend_cycle")
		if err := state.RunBackendCycle(ctx); err != nil {
			cycleReportErr(err)
		}
		cycleEnd()
	}
	rt := state.Get(ctx)
	anyReachable := false
	_, reportReachable, reachableEnd := tracker.Start(ctx, "check", "backend_reachability")
	for id, bs := range rt {
		if bs.Error != "" {
			reportReachable(id, map[string]any{"url": bs.Backend.BaseURL, "error": bs.Error})
		} else {
			anyReachable = true
		}
	}
	if !anyReachable {
		reportReachable("", "no reachable backends; subsequent model operations may fail")
	}
	reachableEnd()

	tokenizer := ollamatokenizer.NewEstimateTokenizer()

	audio := resolveAudioModel(ctx, runtimetypes.New(db.WithoutTransaction()), cfg)

	repo, err := llmrepo.NewModelManager(state, tokenizer, llmrepo.ModelManagerConfig{
		DefaultPromptModel: llmrepo.ModelConfig{Name: cfg.DefaultModel, Provider: cfg.DefaultProvider},
		DefaultChatModel:   llmrepo.ModelConfig{Name: cfg.DefaultModel, Provider: cfg.DefaultProvider},
		DefaultAudioModel:  audio,
	}, tracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}
	engine.Models = repo
	engine.AudioModel = audio
	engine.Tracker = tracker
	engine.KVStore = kvMgr
	success = true
	return engine, nil

}

func resolveAudioModel(ctx context.Context, store runtimetypes.Store, cfg Config) llmrepo.ModelConfig {
	model := strings.TrimSpace(cfg.DefaultAudioModel)
	if model == "" {
		model = clikv.Read(ctx, store, "default-audio-model")
	}
	provider := strings.TrimSpace(cfg.DefaultAudioProvider)
	if provider == "" {
		provider = clikv.Read(ctx, store, "default-audio-provider")
	}
	if model == "" {
		return llmrepo.ModelConfig{}
	}
	return llmrepo.ModelConfig{Name: model, Provider: provider}
}
