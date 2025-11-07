package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/contenox/runtime/affinitygroupservice"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/backendservice"
	"github.com/contenox/runtime/openaichatservice"
	"github.com/contenox/runtime/downloadservice"
	"github.com/contenox/runtime/embedservice"
	"github.com/contenox/runtime/eventbridgeservice"
	"github.com/contenox/runtime/eventmappingservice"
	"github.com/contenox/runtime/eventsourceservice"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/executor"
	"github.com/contenox/runtime/functionservice"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/internal/backendapi"
	"github.com/contenox/runtime/internal/chatapi"
	eventbridgeapi "github.com/contenox/runtime/internal/eventbusapi"
	"github.com/contenox/runtime/internal/eventdispatch"
	"github.com/contenox/runtime/internal/eventmappingapi"
	"github.com/contenox/runtime/internal/eventsourceapi"
	"github.com/contenox/runtime/internal/execapi"
	"github.com/contenox/runtime/internal/execsyncapi"
	"github.com/contenox/runtime/internal/functionapi"
	"github.com/contenox/runtime/internal/groupapi"
	"github.com/contenox/runtime/internal/hooksapi"
	"github.com/contenox/runtime/internal/llmrepo"
	"github.com/contenox/runtime/internal/providerapi"
	"github.com/contenox/runtime/internal/runtimestate"
	"github.com/contenox/runtime/internal/taskchainapi"
	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libroutine"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/providerservice"
	"github.com/contenox/runtime/stateservice"
	"github.com/contenox/runtime/taskchainservice"
	"github.com/contenox/runtime/taskengine"
)

func New(
	ctx context.Context,
	mux *http.ServeMux,
	nodeInstanceID string,
	tenancy string,
	config *Config,
	dbInstance libdb.DBManager,
	pubsub libbus.Messenger,
	repo llmrepo.ModelRepo,
	environmentExec taskengine.EnvExecutor,
	state *runtimestate.State,
	hookRegistry taskengine.HookProvider,
	taskService execservice.TasksEnvService,
	embedService embedservice.Service,
	execService execservice.ExecService,
	// kvManager libkv.KVManager,
) (func() error, error) {
	cleanup := func() error { return nil }
	// tracker := taskengine.NewKVActivityTracker(kvManager)
	stdOuttracker := libtracker.NewLogActivityTracker(slog.Default())
	serveropsChainedTracker := libtracker.ChainedTracker{
		// tracker,
		stdOuttracker,
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		apiframework.Error(w, r, apiframework.ErrNotFound, apiframework.ListOperation)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		// OK
	})
	version := apiframework.GetVersion()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		apiframework.Encode(w, r, http.StatusOK, apiframework.AboutServer{Version: version, NodeInstanceID: nodeInstanceID, Tenancy: tenancy})
	})
	backendService := backendservice.New(dbInstance)
	backendService = backendservice.WithActivityTracker(backendService, serveropsChainedTracker)
	stateService := stateservice.New(state)
	stateService = stateservice.WithActivityTracker(stateService, serveropsChainedTracker)
	backendapi.AddBackendRoutes(mux, backendService, stateService)
	groupservice := affinitygroupservice.New(dbInstance)
	backendapi.AddStateRoutes(mux, stateService)
	groupapi.AddgroupRoutes(mux, groupservice)
	// Get circuit breaker group instance
	group := libroutine.GetGroup()

	// Start managed loops using the group
	group.StartLoop(
		ctx,
		&libroutine.LoopConfig{
			Key:          "backendCycle",
			Threshold:    3,
			ResetTimeout: 10 * time.Second,
			Interval:     10 * time.Second,
			Operation:    state.RunBackendCycle,
		},
	)

	group.StartLoop(
		ctx,
		&libroutine.LoopConfig{
			Key:          "downloadCycle",
			Threshold:    3,
			ResetTimeout: 10 * time.Second,
			Interval:     10 * time.Second,
			Operation:    state.RunDownloadCycle,
		},
	)

	// Add this after the group loops are started in serverapi.New
	triggerCh := make(chan []byte, 10)
	err := pubsub.Publish(ctx, "trigger_cycle", []byte("trigger"))
	if err != nil {
		log.Fatalf("failed to publish trigger_cycle message: %v", err)
	}
	sub, err := pubsub.Stream(ctx, "trigger_cycle", triggerCh)
	if err != nil {
		log.Fatalf("failed to subscribe to trigger_cycle topic: %v", err)
	}
	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-triggerCh:
				if !ok {
					return
				}
				// Force immediate execution of both cycles
				group.ForceUpdate("backendCycle")
				group.ForceUpdate("downloadCycle")
			}
		}
	}()

	downloadService := downloadservice.New(dbInstance, pubsub)
	downloadService = downloadservice.WithActivityTracker(downloadService, serveropsChainedTracker)
	backendapi.AddQueueRoutes(mux, downloadService)
	modelService := modelservice.New(dbInstance, config.EmbedModel)
	modelService = modelservice.WithActivityTracker(modelService, serveropsChainedTracker)
	backendapi.AddModelRoutes(mux, modelService, downloadService)
	execService = execservice.WithActivityTracker(execService, serveropsChainedTracker)
	embedService = embedservice.WithActivityTracker(embedService, serveropsChainedTracker)
	taskChainService := taskchainservice.New(dbInstance)
	taskChainService = taskchainservice.WithActivityTracker(taskChainService, serveropsChainedTracker)
	taskchainapi.AddTaskChainRoutes(mux, taskChainService)
	execapi.AddExecRoutes(mux, execService, taskService, embedService)
	providerService := providerservice.New(dbInstance)
	providerService = providerservice.WithActivityTracker(providerService, serveropsChainedTracker)
	providerapi.AddProviderRoutes(mux, providerService)
	hookproviderService := hookproviderservice.New(dbInstance, hookRegistry)
	hookproviderService = hookproviderservice.WithActivityTracker(hookproviderService, serveropsChainedTracker)
	hooksapi.AddRemoteHookRoutes(mux, hookproviderService)
	chatService := openaichatservice.New(
		taskService,
		taskChainService,
	)
	chatService = openaichatservice.WithActivityTracker(chatService, serveropsChainedTracker)
	chatapi.AddChatRoutes(mux, chatService)
	functionService := functionservice.New(dbInstance)
	functionService = functionservice.WithActivityTracker(functionService, serveropsChainedTracker)
	executorService := executor.NewGojaExecutor(serveropsChainedTracker, functionService)
	eventbus, err := eventdispatch.New(ctx, functionService, func(ctx context.Context, err error) {
		// TODO:
	}, time.Second, executorService, serveropsChainedTracker)
	if err != nil {
		return cleanup, fmt.Errorf("failed to initialize event dispatch service: %w", err)
	}

	eventSourceService, err := eventsourceservice.NewEventSourceService(ctx, dbInstance, pubsub, eventbus)
	if err != nil {
		return cleanup, fmt.Errorf("failed to initialize event source service: %w", err)
	}

	eventSourceService = eventsourceservice.WithActivityTracker(eventSourceService, serveropsChainedTracker)
	eventsourceapi.AddEventSourceRoutes(mux, eventSourceService)

	functionapi.AddFunctionRoutes(mux, functionService)

	execsyncapi.AddExecutorRoutes(mux, executorService, eventbus)
	executorService.AddBuildInServices(eventSourceService, execService, taskChainService, taskService)
	executorService.StartSync(ctx, time.Second*3)

	eventMappingService := eventmappingservice.New(dbInstance)

	eventMappingService = eventmappingservice.WithActivityTracker(eventMappingService, serveropsChainedTracker)
	eventmappingapi.AddMappingRoutes(mux, eventMappingService)
	eventbridgeService := eventbridgeservice.New(eventMappingService, eventSourceService, time.Second*3)
	eventbridgeapi.AddEventBridgeRoutes(mux, eventbridgeService)

	return cleanup, nil
}

type Config struct {
	DatabaseURL             string `json:"database_url"`
	Port                    string `json:"port"`
	Addr                    string `json:"addr"`
	NATSURL                 string `json:"nats_url"`
	NATSUser                string `json:"nats_user"`
	NATSPassword            string `json:"nats_password"`
	TokenizerServiceURL     string `json:"tokenizer_service_url"`
	EmbedModel              string `json:"embed_model"`
	EmbedProvider           string `json:"embed_provider"`
	EmbedModelContextLength string `json:"embed_model_context_length"`
	TaskModel               string `json:"task_model"`
	TaskProvider            string `json:"task_provider"`
	TaskModelContextLength  string `json:"task_model_context_length"`
	ChatModel               string `json:"chat_model"`
	ChatProvider            string `json:"chat_provider"`
	ChatModelContextLength  string `json:"chat_model_context_length"`
	VectorStoreURL          string `json:"vector_store_url"`
	Token                   string `json:"token"`
	UIBaseURL               string `json:"ui_base_url"`
}

func LoadConfig[T any](cfg *T) error {
	if cfg == nil {
		return fmt.Errorf("config pointer is nil")
	}
	config := map[string]string{}
	for _, kvPair := range os.Environ() {
		ar := strings.SplitN(kvPair, "=", 2)
		if len(ar) < 2 {
			continue
		}
		key := strings.ToLower(ar[0])
		value := ar[1]
		config[key] = value
	}

	b, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal env vars: %w", err)
	}
	err = json.Unmarshal(b, cfg)
	if err != nil {
		return fmt.Errorf("failed to unmarshal into config struct: %w", err)
	}

	return nil
}
