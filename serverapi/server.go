package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/contenox/activitytracker"
	libbus "github.com/contenox/bus"
	libdb "github.com/contenox/dbexec"
	libkv "github.com/contenox/kvstore"
	"github.com/contenox/routine"
	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/backendapi"
	"github.com/contenox/runtime/backendservice"
	"github.com/contenox/runtime/downloadservice"
	"github.com/contenox/runtime/embedservice"
	"github.com/contenox/runtime/execapi"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/hooksapi"
	"github.com/contenox/runtime/llmrepo"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/poolapi"
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/providerapi"
	"github.com/contenox/runtime/providerservice"
	"github.com/contenox/runtime/runtimestate"
	"github.com/contenox/runtime/stateservice"
	"github.com/contenox/runtime/taskengine"
)

func New(
	ctx context.Context,
	nodeInstanceID string,
	tenancy string,
	config *Config,
	dbInstance libdb.DBManager,
	pubsub libbus.Messenger,
	repo llmrepo.ModelRepo,
	environmentExec taskengine.EnvExecutor,
	state *runtimestate.State,
	hookRegistry taskengine.HookRegistry,
	kvManager libkv.KVManager,
) (http.Handler, func() error, error) {
	cleanup := func() error { return nil }
	mux := http.NewServeMux()
	var handler http.Handler = mux
	tracker := taskengine.NewKVActivityTracker(kvManager)
	stdOuttracker := activitytracker.NewLogActivityTracker(slog.Default())
	serveropsChainedTracker := activitytracker.ChainedTracker{
		tracker,
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
	poolservice := poolservice.New(dbInstance)
	backendapi.AddStateRoutes(mux, stateService)
	poolapi.AddPoolRoutes(mux, poolservice)
	// Get circuit breaker pool instance
	pool := routine.GetPool()

	// Start managed loops using the pool
	pool.StartLoop(
		ctx,
		&routine.LoopConfig{
			Key:          "backendCycle",
			Threshold:    3,
			ResetTimeout: 10 * time.Second,
			Interval:     10 * time.Second,
			Operation:    state.RunBackendCycle,
		},
	)

	pool.StartLoop(
		ctx,
		&routine.LoopConfig{
			Key:          "downloadCycle",
			Threshold:    3,
			ResetTimeout: 10 * time.Second,
			Interval:     10 * time.Second,
			Operation:    state.RunDownloadCycle,
		},
	)

	downloadService := downloadservice.New(dbInstance, pubsub)
	downloadService = downloadservice.WithActivityTracker(downloadService, serveropsChainedTracker)
	backendapi.AddQueueRoutes(mux, downloadService)
	modelService := modelservice.New(dbInstance, config.EmbedModel)
	modelService = modelservice.WithActivityTracker(modelService, serveropsChainedTracker)
	backendapi.AddModelRoutes(mux, modelService, downloadService)
	execService := execservice.NewExec(ctx, repo, dbInstance)
	execService = execservice.WithActivityTracker(execService, serveropsChainedTracker)
	taskService := execservice.NewTasksEnv(ctx, environmentExec, dbInstance, hookRegistry)
	embedService := embedservice.New(repo, config.EmbedModel, config.EmbedProvider)
	embedService = embedservice.WithActivityTracker(embedService, serveropsChainedTracker)
	execapi.AddExecRoutes(mux, execService, taskService, embedService)
	providerService := providerservice.New(dbInstance)
	providerService = providerservice.WithActivityTracker(providerService, serveropsChainedTracker)
	providerapi.AddProviderRoutes(mux, providerService)
	hookproviderService := hookproviderservice.New(dbInstance)
	hookproviderService = hookproviderservice.WithActivityTracker(hookproviderService, serveropsChainedTracker)
	hooksapi.AddRemoteHookRoutes(mux, hookproviderService)
	handler = apiframework.RequestIDMiddleware(handler)
	handler = apiframework.TracingMiddleware(handler)
	if config.Token != "" {
		handler = apiframework.TokenMiddleware(handler)
		handler = apiframework.EnforceToken(config.Token, handler)
	}

	return handler, cleanup, nil
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
	VectorStoreURL          string `json:"vector_store_url"`
	KVBackend               string `json:"kv_backend"`
	KVHost                  string `json:"kv_host"`
	KVPassword              string `json:"kv_password"`
	Token                   string `json:"token"`
}

func LoadConfig[T any](cfg *T) error {
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
