package playground

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/contenox/runtime/libroutine"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/providerservice"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/stateservice"
	"github.com/contenox/runtime/taskchainservice"
	"github.com/contenox/runtime/taskengine"
)

// Playground provides a fluent API for setting up a test environment.
// Errors are chained, and execution stops on the first failure.
type Playground struct {
	cleanUps                  []func()
	db                        libdbexec.DBManager
	bus                       libbus.Messenger
	state                     *runtimestate.State
	tokenizer                 ollamatokenizer.Tokenizer
	llmRepo                   llmrepo.ModelRepo
	hookrepo                  taskengine.HookRepo
	withPool                  bool
	routinesStarted           bool
	embeddingsModel           string
	embeddingsModelProvider   string
	embeddingsModelContextLen int
	llmPromptModel            string
	llmPromptModelProvider    string
	llmPromptModelContextLen  int
	llmChatModel              string
	llmChatModelProvider      string
	llmChatModelContextLen    int
	Error                     error
}

// A fixed tenant ID for testing purposes.
const testTenantID = "00000000-0000-0000-0000-000000000000"

// New creates a new Playground instance.
func New() *Playground {
	return &Playground{}
}

// AddCleanUp adds a cleanup function to be called by CleanUp.
func (p *Playground) AddCleanUp(cleanUp func()) {
	p.cleanUps = append(p.cleanUps, cleanUp)
}

// GetError returns the first error that occurred during the setup chain.
func (p *Playground) GetError() error {
	return p.Error
}

// CleanUp runs all registered cleanup functions.
func (p *Playground) CleanUp() {
	// Run cleanups in reverse order of addition.
	for i := len(p.cleanUps) - 1; i >= 0; i-- {
		p.cleanUps[i]()
	}
}

// StartBackgroundRoutines starts the core background processes for backend and download cycles.
func (p *Playground) StartBackgroundRoutines(ctx context.Context) *Playground {
	if p.Error != nil {
		return p
	}
	if p.state == nil {
		p.Error = errors.New("cannot start background routines: runtime state is not initialized")
		return p
	}

	pool := libroutine.GetPool()

	pool.StartLoop(
		ctx,
		&libroutine.LoopConfig{
			Key:          "backendCycle",
			Threshold:    3,
			ResetTimeout: 1 * time.Second,
			Interval:     1 * time.Second,
			Operation:    p.state.RunBackendCycle,
		},
	)

	pool.StartLoop(
		ctx,
		&libroutine.LoopConfig{
			Key:          "downloadCycle",
			Threshold:    3,
			ResetTimeout: 1 * time.Second,
			Interval:     1 * time.Second,
			Operation:    p.state.RunDownloadCycle,
		},
	)

	// Force an initial run to kick things off immediately in the test environment.
	pool.ForceUpdate("backendCycle")
	pool.ForceUpdate("downloadCycle")

	p.routinesStarted = true
	return p
}

// WithInternalOllamaEmbedder initializes the internal embedding model and pool.
func (p *Playground) WithInternalOllamaEmbedder(ctx context.Context, modelName string, contextLen int) *Playground {
	if p.Error != nil {
		return p
	}
	if p.db == nil {
		p.Error = errors.New("cannot init internal embedder: database is not configured")
		return p
	}
	p.embeddingsModel = modelName
	p.embeddingsModelProvider = "ollama"
	// Store context length
	p.embeddingsModelContextLen = contextLen
	config := &runtimestate.Config{
		EmbedModel: modelName,
		TenantID:   testTenantID,
	}

	err := runtimestate.InitEmbeder(ctx, config, p.db, contextLen, p.state)
	if err != nil {
		p.Error = fmt.Errorf("failed to initialize internal embedder: %w", err)
	}
	return p
}

func (p *Playground) WithInternalChatExecutor(ctx context.Context, modelName string, contextLen int) *Playground {
	if p.Error != nil {
		return p
	}
	if p.db == nil {
		p.Error = errors.New("cannot init internal chat executor: database is not configured")
		return p
	}

	config := &runtimestate.Config{
		ChatModel: modelName,
		TenantID:  testTenantID,
	}
	// Store context length
	p.llmChatModelContextLen = contextLen
	p.llmChatModel = modelName
	p.llmChatModelProvider = "ollama"

	err := runtimestate.InitChatExec(ctx, config, p.db, p.state, contextLen)
	if err != nil {
		p.Error = fmt.Errorf("failed to initialize internal chat executor: %w", err)
	}
	return p
}

// WithInternalPromptExecutor initializes the internal task/prompt model and pool.
func (p *Playground) WithInternalPromptExecutor(ctx context.Context, modelName string, contextLen int) *Playground {
	if p.Error != nil {
		return p
	}
	if p.db == nil {
		p.Error = errors.New("cannot init internal prompt executor: database is not configured")
		return p
	}
	if p.tokenizer == nil {
		p.Error = errors.New("cannot init internal prompt executor: tokenizer is not configured")
		return p
	}

	config := &runtimestate.Config{
		TaskModel: modelName,
		TenantID:  testTenantID,
	}
	// Store context length
	p.llmPromptModelContextLen = contextLen
	p.llmPromptModel = modelName
	p.llmPromptModelProvider = "ollama"

	err := runtimestate.InitPromptExec(ctx, config, p.db, p.state, contextLen)
	if err != nil {
		p.Error = fmt.Errorf("failed to initialize internal prompt executor: %w", err)
	}
	return p
}

// WithPostgresTestContainer sets up a test PostgreSQL container and initializes the DB manager.
func (p *Playground) WithPostgresTestContainer(ctx context.Context) *Playground {
	if p.Error != nil {
		return p
	}
	connStr, _, cleanup, err := libdbexec.SetupLocalInstance(ctx, "test", "test", "test")
	if err != nil {
		p.Error = fmt.Errorf("failed to setup postgres test container: %w", err)
		return p
	}
	p.AddCleanUp(cleanup)

	dbManager, err := libdbexec.NewPostgresDBManager(ctx, connStr, runtimetypes.Schema)
	if err != nil {
		p.Error = fmt.Errorf("failed to create postgres db manager: %w", err)
		return p
	}
	p.db = dbManager
	return p
}

// WithNats sets up a test NATS server.
func (p *Playground) WithNats(ctx context.Context) *Playground {
	if p.Error != nil {
		return p
	}
	ps, cleanup, err := libbus.NewTestPubSub()
	if err != nil {
		p.Error = fmt.Errorf("failed to setup nats test server: %w", err)
		return p
	}
	p.AddCleanUp(cleanup)
	p.bus = ps
	return p
}

// WithDefaultEmbeddingsModel sets the default embeddings model and provider.
func (p *Playground) WithDefaultEmbeddingsModel(model string, provider string, contextLength int) *Playground {
	if p.Error != nil {
		return p
	}
	p.embeddingsModel = model
	p.embeddingsModelProvider = provider
	p.embeddingsModelContextLen = contextLength
	return p
}

// WithDefaultPromptModel sets the default prompt model and provider.
func (p *Playground) WithDefaultPromptModel(model string, provider string, contextLength int) *Playground {
	if p.Error != nil {
		return p
	}
	p.llmPromptModel = model
	p.llmPromptModelProvider = provider
	p.llmPromptModelContextLen = contextLength
	return p
}

// WithDefaultChatModel sets the default chat model and provider.
func (p *Playground) WithDefaultChatModel(model string, provider string, contextLength int) *Playground {
	if p.Error != nil {
		return p
	}
	p.llmChatModel = model
	p.llmChatModelProvider = provider
	p.llmChatModelContextLen = contextLength
	return p
}

// WithRuntimeState initializes the runtime state.
func (p *Playground) WithRuntimeState(ctx context.Context, withPools bool) *Playground {
	if p.Error != nil {
		return p
	}
	if p.db == nil {
		p.Error = errors.New("cannot initialize runtime state: database is not configured")
		return p
	}
	if p.bus == nil {
		p.Error = errors.New("cannot initialize runtime state: message bus is not configured")
		return p
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
		p.Error = fmt.Errorf("failed to initialize runtime state: %w", err)
		return p
	}
	p.state = state
	return p
}

// WithMockHookRegistry sets up a mock hook registry.
func (p *Playground) WithMockHookRegistry() *Playground {
	if p.Error != nil {
		return p
	}
	if p.state == nil {
		p.Error = errors.New("cannot initialize mock hook registry: runtime state is not configured")
		return p
	}
	p.hookrepo = hooks.NewMockHookRegistry()
	return p
}

// WithMockTokenizer sets up a mock tokenizer.
func (p *Playground) WithMockTokenizer() *Playground {
	if p.Error != nil {
		return p
	}
	if p.state == nil {
		p.Error = errors.New("cannot initialize mock tokenizer: runtime state is not configured")
		return p
	}
	p.tokenizer = ollamatokenizer.MockTokenizer{}
	return p
}

// WithLLMRepo initializes the LLM repository.
func (p *Playground) WithLLMRepo() *Playground {
	if p.Error != nil {
		return p
	}
	if p.state == nil {
		p.Error = errors.New("cannot initialize llm repo: runtime state is not configured")
		return p
	}
	if p.tokenizer == nil {
		p.Error = errors.New("cannot initialize llm repo: tokenizer is not configured")
		return p
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
		p.Error = fmt.Errorf("failed to create llm repo model manager: %w", err)
		return p
	}
	return p
}

// WithOllamaBackend sets up an Ollama test instance and registers it as a backend.
func (p *Playground) WithOllamaBackend(ctx context.Context, name, tag string, assignEmbeddingModel, assignTasksModel bool) *Playground {
	if p.Error != nil {
		return p
	}
	uri, _, cleanup, err := modelrepo.SetupOllamaLocalInstance(ctx, tag)
	if err != nil {
		p.Error = fmt.Errorf("failed to setup ollama local instance: %w", err)
		return p
	}
	p.AddCleanUp(cleanup)

	backends, err := p.GetBackendService()
	if err != nil {
		p.Error = fmt.Errorf("failed to get backend service for ollama setup: %w", err)
		return p
	}

	backend := &runtimetypes.Backend{
		Name:    name,
		BaseURL: uri,
		Type:    "ollama",
	}
	if err := backends.Create(ctx, backend); err != nil {
		p.Error = fmt.Errorf("failed to create ollama backend '%s': %w", name, err)
		return p
	}

	if !p.withPool {
		return p
	}

	pool, err := p.GetPoolService()
	if err != nil {
		p.Error = fmt.Errorf("failed to get pool service for ollama setup: %w", err)
		return p
	}

	if assignEmbeddingModel {
		if err := pool.AssignBackend(ctx, runtimestate.EmbedPoolID, backend.ID); err != nil {
			p.Error = fmt.Errorf("failed to assign ollama backend to embed pool: %w", err)
			return p
		}
	}
	if assignTasksModel {
		if err := pool.AssignBackend(ctx, runtimestate.TasksPoolID, backend.ID); err != nil {
			p.Error = fmt.Errorf("failed to assign ollama backend to tasks pool: %w", err)
			return p
		}
	}
	return p
}

// WithPostgresReal connects to a real PostgreSQL instance using the provided connection string.
func (p *Playground) WithPostgresReal(ctx context.Context, connStr string) *Playground {
	if p.Error != nil {
		return p
	}
	dbManager, err := libdbexec.NewPostgresDBManager(ctx, connStr, runtimetypes.Schema)
	if err != nil {
		p.Error = fmt.Errorf("failed to create postgres db manager: %w", err)
		return p
	}
	p.db = dbManager
	// No cleanup needed for real resources - user manages lifecycle
	return p
}

// WithNatsReal sets up a connection to a real NATS server.
func (p *Playground) WithNatsReal(ctx context.Context, natsURL, natsUser, natsPassword string) *Playground {
	if p.Error != nil {
		return p
	}
	ps, err := libbus.NewPubSub(ctx, &libbus.Config{
		NATSURL:      natsURL,
		NATSUser:     natsUser,
		NATSPassword: natsPassword,
	})
	if err != nil {
		p.Error = fmt.Errorf("failed to setup nats server: %w", err)
		return p
	}
	p.bus = ps
	// No cleanup needed for real resources - user manages lifecycle
	return p
}

// WithTokenizerService sets up the tokenizer service from a real service URL
func (p *Playground) WithTokenizerService(ctx context.Context, tokenizerURL string) *Playground {
	if p.Error != nil {
		return p
	}

	tokenizerSvc, cleanup, err := ollamatokenizer.NewHTTPClient(ctx, ollamatokenizer.ConfigHTTP{
		BaseURL: tokenizerURL,
	})
	if err != nil {
		p.Error = fmt.Errorf("failed to setup tokenizer service: %w", err)
		return p
	}
	wrappedCleanup := func() {
		_ = cleanup()
	}
	p.tokenizer = tokenizerSvc
	p.AddCleanUp(wrappedCleanup)
	return p
}

// GetBackendService returns a new backend service instance.
func (p *Playground) GetBackendService() (backendservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get backend service: database is not initialized")
	}
	return backendservice.New(p.db), nil
}

// GetDownloadService returns a new download service instance.
func (p *Playground) GetDownloadService() (downloadservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get download service: database is not initialized")
	}
	if p.bus == nil {
		return nil, errors.New("cannot get download service: message bus is not initialized")
	}
	return downloadservice.New(p.db, p.bus), nil
}

// GetModelService returns a new model service instance.
func (p *Playground) GetModelService() (modelservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get model service: database is not initialized")
	}
	return modelservice.New(p.db, p.embeddingsModel), nil
}

// GetPoolService returns a new pool service instance.
func (p *Playground) GetPoolService() (poolservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get pool service: database is not initialized")
	}
	return poolservice.New(p.db), nil
}

// GetProviderService returns a new provider service instance.
func (p *Playground) GetProviderService() (providerservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get provider service: database is not initialized")
	}
	return providerservice.New(p.db), nil
}

// GetEmbedService returns a new embed service instance.
func (p *Playground) GetEmbedService() (embedservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.llmRepo == nil {
		return nil, errors.New("cannot get embed service: llm repo is not initialized")
	}
	if p.embeddingsModel == "" {
		return nil, errors.New("cannot get embed service: embeddings model is not configured")
	}
	if p.embeddingsModelProvider == "" {
		return nil, errors.New("cannot get embed service: embeddings model provider is not configured")
	}
	return embedservice.New(p.llmRepo, p.embeddingsModel, p.embeddingsModelProvider), nil
}

// GetStateService returns a new state service instance.
func (p *Playground) GetStateService() (stateservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.state == nil {
		return nil, errors.New("cannot get state service: runtime state is not initialized")
	}
	return stateservice.New(p.state), nil
}

// GetTaskChainService returns a new task chain service instance.
func (p *Playground) GetTaskChainService() (taskchainservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get task chain service: database is not initialized")
	}
	return taskchainservice.New(p.db), nil
}

// GetExecService returns a new exec service instance.
func (p *Playground) GetExecService(ctx context.Context) (execservice.ExecService, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.llmRepo == nil {
		return nil, errors.New("cannot get exec service: llmRepo is not initialized")
	}
	return execservice.NewExec(ctx, p.llmRepo), nil
}

// GetTasksEnvService returns a new tasks environment service instance.
func (p *Playground) GetTasksEnvService(ctx context.Context) (execservice.TasksEnvService, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.llmRepo == nil {
		return nil, errors.New("cannot get tasks env service: llmRepo is not initialized")
	}
	if p.hookrepo == nil {
		return nil, errors.New("cannot get tasks env service: hookrepo is not initialized")
	}

	exec, err := taskengine.NewExec(ctx, p.llmRepo, p.hookrepo, libtracker.NewLogActivityTracker(slog.Default()))
	if err != nil {
		return nil, fmt.Errorf("failed to create task engine exec: %w", err)
	}

	env, err := taskengine.NewEnv(ctx, libtracker.NewLogActivityTracker(slog.Default()), exec, taskengine.NewSimpleInspector())
	if err != nil {
		return nil, fmt.Errorf("failed to create task engine env: %w", err)
	}

	return execservice.NewTasksEnv(ctx, env, p.hookrepo), nil
}

// GetChatService returns a new chat service instance.
func (p *Playground) GetChatService(ctx context.Context) (chatservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}

	envExec, err := p.GetTasksEnvService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks env service for chat service: %w", err)
	}

	taskChainService, err := p.GetTaskChainService()
	if err != nil {
		return nil, fmt.Errorf("failed to get task chain service for chat service: %w", err)
	}

	return chatservice.New(envExec, taskChainService), nil
}

// GetHookProviderService returns a new hook provider service instance.
func (p *Playground) GetHookProviderService() (hookproviderservice.Service, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.db == nil {
		return nil, errors.New("cannot get hook provider service: database is not initialized")
	}
	return hookproviderservice.New(p.db), nil
}

// WaitUntilModelIsReady blocks until the specified model is available on the specified backend.
func (p *Playground) WaitUntilModelIsReady(ctx context.Context, backendName, modelName string) error {
	if p.Error != nil {
		return p.Error
	}
	if !p.routinesStarted {
		return errors.New("WaitUntilModelIsReady called before WithBackgroundRoutines; routines are not running")
	}

	stateService, err := p.GetStateService()
	if err != nil {
		return fmt.Errorf("could not get state service to wait for model: %w", err)
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
				// Log or ignore transient errors and continue trying.
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
					// Found the backend, but not the model yet. Continue waiting.
					break
				}
			}
		}
	}
}
