package playground

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/runtime/backendservice"
	"github.com/contenox/runtime/chatservice"
	"github.com/contenox/runtime/downloadservice"
	"github.com/contenox/runtime/embedservice"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/internal/hooks"
	"github.com/contenox/runtime/internal/llmrepo"
	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/contenox/runtime/internal/ollamatokenizer"
	"github.com/contenox/runtime/internal/runtimestate"
	"github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/providerservice"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/stateservice"
	"github.com/contenox/runtime/taskchainservice"
	"github.com/contenox/runtime/taskengine"
)

type Playground struct {
	cleanUps                []func()
	db                      libdbexec.DBManager
	bus                     libbus.Messenger
	state                   *runtimestate.State
	tokenizer               ollamatokenizer.Tokenizer
	llmRepo                 llmrepo.ModelRepo
	hookrepo                taskengine.HookRepo
	withPool                bool
	embeddingsModel         string
	embeddingsModelProvider string
	llmPromptModel          string
	llmPromptModelProvider  string
	llmChatModel            string
	llmChatModelProvider    string
	Error                   error
}

func NewPlayground() *Playground {
	return &Playground{}
}

func (p *Playground) AddCleanUp(cleanUp func()) {
	p.cleanUps = append(p.cleanUps, cleanUp)
}

func (p *Playground) GetError() error {
	return p.Error
}

func (p *Playground) WithPostgres(ctx context.Context) {
	connStr, _, cleanup, err := libdbexec.SetupLocalInstance(ctx, "test", "test", "test")
	if err != nil {
		p.Error = err
		return
	}
	dbManager, err := libdbexec.NewPostgresDBManager(ctx, connStr, runtimetypes.Schema)
	if err != nil {
		p.Error = err
		return
	}
	p.db = dbManager
	p.AddCleanUp(cleanup)
}

func (p *Playground) WithNats(ctx context.Context) {
	ps, cleanup, err := libbus.NewTestPubSub()
	if err != nil {
		p.Error = err
		return
	}
	p.AddCleanUp(cleanup)
	p.bus = ps
}

func (p *Playground) WithDefaultEmbeddingsModel(model string, provider string) {
	p.embeddingsModel = model
	p.embeddingsModelProvider = provider
}

func (p *Playground) WithDefaultPromptModel(model string, provider string) {
	p.llmPromptModel = model
	p.llmPromptModelProvider = provider
}

func (p *Playground) WithDefaultChatModel(model string, provider string) {
	p.llmChatModel = model
	p.llmChatModelProvider = provider
}

func (p *Playground) WithRuntimeState(ctx context.Context, withPools bool) {
	if p.Error != nil {
		return
	}
	if p.db == nil {
		return
	}
	if p.bus == nil {
		return
	}
	var state *runtimestate.State
	var err error
	p.withPool = withPools
	if withPools {
		state, err = runtimestate.New(ctx, p.db, p.bus, runtimestate.WithPools())
	} else {
		state, err = runtimestate.New(ctx, p.db, p.bus)
	}
	if err != nil {
		p.Error = err
		return
	}
	p.state = state
}

func (p *Playground) WithMockHookRegistry() {
	if p.Error != nil {
		return
	}
	if p.state == nil {
		return
	}
	p.hookrepo = hooks.NewMockHookRegistry()
}

func (p *Playground) WithMockTokenizer() {
	if p.Error != nil {
		return
	}
	if p.state == nil {
		return
	}
	p.tokenizer = ollamatokenizer.MockTokenizer{}
}

func (p *Playground) WithLLMRepo() {
	if p.Error != nil {
		return
	}
	if p.state == nil {
		return
	}
	if p.tokenizer == nil {
		return
	}
	if p.embeddingsModelProvider == "" {
		return
	}
	if p.llmChatModel == "" {
		return
	}
	if p.llmChatModelProvider == "" {
		return
	}
	if p.llmPromptModel == "" {
		return
	}
	if p.llmPromptModelProvider == "" {
		return
	}
	var err error
	p.llmRepo, err = llmrepo.NewModelManager(p.state, p.tokenizer, llmrepo.ModelManagerConfig{
		DefaultEmbeddingModel: llmrepo.ModelConfig{
			Name:     p.embeddingsModel,
			Provider: p.embeddingsModelProvider,
		},
		DefaultPromptModel: llmrepo.ModelConfig{
			Name:     p.llmPromptModel,
			Provider: p.llmPromptModelProvider,
		},
		DefaultChatModel: llmrepo.ModelConfig{
			Name:     p.llmChatModel,
			Provider: p.llmChatModelProvider,
		},
	})
	if err != nil {
		p.Error = err
		return
	}
}

func (p *Playground) WithOllamaBackend(ctx context.Context, name, tag string, assignEmbeddingModel, assignTasksModel bool) {
	uri, _, cleanup, err := modelrepo.SetupOllamaLocalInstance(ctx, tag)
	if err != nil {
		p.Error = err
		return
	}
	p.AddCleanUp(cleanup)
	backends, err := p.GetBackendService()
	if err != nil {
		p.Error = err
		return
	}
	backend := &runtimetypes.Backend{
		Name:    name,
		BaseURL: uri,
		Type:    "ollama",
	}
	err = backends.Create(ctx, backend)
	if err != nil {
		p.Error = err
		return
	}
	if !p.withPool {
		return
	}
	pool, err := p.GetPoolService()
	if err != nil {
		p.Error = err
		return
	}
	if assignEmbeddingModel {
		err = pool.AssignBackend(ctx, runtimestate.EmbedPoolID, backend.ID)
		if err != nil {
			p.Error = err
			return
		}
	}
	if assignTasksModel {
		err = pool.AssignBackend(ctx, runtimestate.TasksPoolID, backend.ID)
		if err != nil {
			p.Error = err
			return
		}
	}
}

func (p *Playground) GetBackendService() (backendservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := backendservice.New(p.db)
	return service, nil
}

func (p *Playground) WithDownloadService() (downloadservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := downloadservice.New(p.db, p.bus)
	return service, nil
}

func (p *Playground) GetModelService() (modelservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	if p.embeddingsModel == "" {
		return nil, errors.New("embeddings model is not initialized")
	}
	service := modelservice.New(p.db, p.embeddingsModel)
	return service, nil
}

func (p *Playground) GetPoolService() (poolservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := poolservice.New(p.db)
	return service, nil
}

func (p *Playground) GetProviderService() (providerservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := providerservice.New(p.db)
	return service, nil
}

func (p *Playground) GetEmbedService() (embedservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	if p.embeddingsModel == "" {
		return nil, errors.New("embeddings model is not initialized")
	}
	if p.embeddingsModelProvider == "" {
		return nil, errors.New("embeddings model provider is not initialized")
	}
	service := embedservice.New(p.llmRepo, p.embeddingsModel, p.embeddingsModelProvider)
	return service, nil
}

func (p *Playground) GetStateService() (stateservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.state == nil {
		return nil, errors.New("state is not initialized")
	}
	service := stateservice.New(p.state)
	return service, nil
}

func (p *Playground) GetTaskChainService() (taskchainservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := taskchainservice.New(p.db)
	return service, nil
}

func (p *Playground) GetExecService(ctx context.Context) (execservice.ExecService, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.llmRepo == nil {
		return nil, errors.New("llmRepo is not initialized")
	}
	service := execservice.NewExec(ctx, p.llmRepo)
	return service, nil
}

func (p *Playground) GetTasksEnvService(ctx context.Context) (execservice.TasksEnvService, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.llmRepo == nil {
		return nil, errors.New("llmRepo is not initialized")
	}
	if p.hookrepo == nil {
		return nil, errors.New("hookrepo is not initialized")
	}
	exec, err := taskengine.NewExec(ctx, p.llmRepo, p.hookrepo, &libtracker.LogActivityTracker{})
	if err != nil {
		return nil, err
	}
	env, err := taskengine.NewEnv(ctx, &libtracker.LogActivityTracker{}, exec, &taskengine.NoopInspector{})
	if err != nil {
		return nil, err
	}
	service := execservice.NewTasksEnv(ctx, env, p.hookrepo)
	return service, nil
}

func (p *Playground) GetChatService(ctx context.Context) (chatservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	envExec, err := p.GetTasksEnvService(ctx)
	if err != nil {
		return nil, err
	}
	taskChainService, err := p.GetTaskChainService()
	if err != nil {
		return nil, err
	}
	service := chatservice.New(envExec, taskChainService)

	return service, nil
}

func (p *Playground) GetHookProviderService() (hookproviderservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("database is not initialized")
	}
	service := hookproviderservice.New(p.db)
	return service, nil
}

func (p *Playground) WaitUntilModelIsReady(ctx context.Context, backendName, modelName string) error {
	if p.Error != nil {
		return p.Error
	}

	stateService, err := p.GetStateService()
	if err != nil {
		return fmt.Errorf("could not get state service: %w", err)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for model '%s' on backend '%s': %w", modelName, backendName, ctx.Err())

		case <-ticker.C:
			allStates, err := stateService.Get(ctx)
			if err != nil {
				continue
			}

			for _, backendState := range allStates {
				if backendState.Name == backendName {
					for _, pulledModel := range backendState.PulledModels {
						if pulledModel.Model == modelName {
							// Success! The model is ready.
							return nil
						}
					}
					break
				}
			}
		}
	}
}
