package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/contenox/activitytracker"
	libbus "github.com/contenox/bus"
	libdb "github.com/contenox/dbexec"
	libkv "github.com/contenox/kvstore"
	libroutine "github.com/contenox/routine"
	"github.com/contenox/runtime/hooks"
	"github.com/contenox/runtime/llmrepo"
	"github.com/contenox/runtime/ollamatokenizer"
	"github.com/contenox/runtime/runtimestate"
	"github.com/contenox/runtime/serverapi"
	"github.com/contenox/runtime/store"
	"github.com/contenox/runtime/taskengine"
)

var (
	cliSetCoreVersion string
	cliSetTenantID    string
	CoreVersion       = "CORE-UNSET-dev"
	TenantID          = "96ed1c59-ffc1-4545-b3c3-191079c68d79"
)

func initDatabase(ctx context.Context, cfg *serverapi.Config) (libdb.DBManager, error) {
	dbURL := cfg.DatabaseURL
	var err error
	if dbURL == "" {
		err = fmt.Errorf("DATABASE_URL is required")
		return nil, fmt.Errorf("failed to create store: %w", err)
	}
	var dbInstance libdb.DBManager
	err = libroutine.NewRoutine(10, time.Minute).ExecuteWithRetry(ctx, time.Second, 3, func(ctx context.Context) error {
		dbInstance, err = libdb.NewPostgresDBManager(ctx, dbURL, store.Schema)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	return dbInstance, nil
}

func initPubSub(ctx context.Context, cfg *serverapi.Config) (libbus.Messenger, error) {
	ps, err := libbus.NewPubSub(ctx, &libbus.Config{
		NATSURL:      cfg.NATSURL,
		NATSPassword: cfg.NATSPassword,
		NATSUser:     cfg.NATSUser,
	})
	if err != nil {
		return nil, err
	}
	return ps, nil
}

func main() {
	if cliSetCoreVersion == "" {
		log.Fatalf("corrupted build! cliSetCoreVersion was not injected")
	}
	if cliSetTenantID == "" {
		log.Fatalf("corrupted build! cliSetTenantID was not injected")
	}
	TenantID = cliSetTenantID
	CoreVersion = cliSetCoreVersion
	config := &serverapi.Config{}
	if err := serverapi.LoadConfig(config); err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	ctx := context.TODO()
	cleanups := []func() error{func() error {
		fmt.Println("cleaning up")
		return nil
	}}
	defer func() {
		for _, cleanup := range cleanups {
			err := cleanup()
			if err != nil {
				log.Printf("cleanup failed: %v", err)
			}
		}
	}()
	fmt.Print("initialize the database")
	dbInstance, err := initDatabase(ctx, config)
	if err != nil {
		log.Fatalf("initializing database failed: %v", err)
	}
	defer dbInstance.Close()

	ps, err := initPubSub(ctx, config)
	if err != nil {
		log.Fatalf("initializing PubSub failed: %v", err)
	}
	if err != nil {
		log.Fatalf("initializing OpenSearch failed: %v", err)
	}
	state, err := runtimestate.New(ctx, dbInstance, ps, runtimestate.WithPools())
	if err != nil {
		log.Fatalf("initializing runtime state failed: %v", err)
	}
	embedder, err := llmrepo.NewEmbedder(ctx, &llmrepo.Config{
		DatabaseURL: config.DatabaseURL,
		EmbedModel:  config.EmbedModel,
		TasksModel:  config.TasksModel,
		TenantID:    TenantID,
	}, dbInstance, state)
	if err != nil {
		log.Fatalf("initializing embedding pool failed: %v", err)
	}
	tokenizerSvc, cleanup, err := ollamatokenizer.NewHTTPClient(ctx, ollamatokenizer.ConfigHTTP{
		BaseURL: config.TokenizerServiceURL,
	})
	if err != nil {
		cleanup()
		log.Fatalf("initializing tokenizer service failed: %v", err)
	}

	execRepo, err := llmrepo.NewExecRepo(ctx, &llmrepo.Config{
		DatabaseURL: config.DatabaseURL,
		EmbedModel:  config.EmbedModel,
		TasksModel:  config.TasksModel,
		TenantID:    TenantID,
	}, dbInstance, state, tokenizerSvc)
	if err != nil {
		log.Fatalf("initializing promptexec failed: %v", err)
	}

	cleanups = append(cleanups, cleanup)
	if err != nil {
		log.Fatalf("initializing vector store failed: %v", err)
	}
	kvManager, err := libkv.NewManager(libkv.Config{
		Addr:     config.KVHost,
		Password: config.KVPassword,
	}, time.Hour*24)
	if err != nil {
		log.Fatalf("initializing kv manager failed: %v", err)
	}
	kvExec, err := kvManager.Executor(ctx)
	if err != nil {
		log.Fatalf("initializing kv manager 1 failed: %v", err)
	}
	err = kvExec.SetWithTTL(ctx, "test", []byte("test"), time.Second)
	if err != nil {
		log.Fatalf("initializing kv manager 2 failed: %v", err)
	}

	tracker := taskengine.NewKVActivityTracker(kvManager)
	stdOuttracker := activitytracker.NewLogActivityTracker(slog.Default())
	serveropsChainedTracker := activitytracker.ChainedTracker{
		tracker,
		stdOuttracker,
	}

	// Combine all hooks into one registry
	hooks := hooks.NewSimpleProvider(map[string]taskengine.HookRepo{})
	exec, err := taskengine.NewExec(ctx, execRepo, hooks, serveropsChainedTracker)
	if err != nil {
		log.Fatalf("initializing task engine engine failed: %v", err)
	}
	environmentExec, err := taskengine.NewEnv(ctx, serveropsChainedTracker, taskengine.NewAlertSink(kvManager), exec, taskengine.NewSimpleInspector(kvManager))
	if err != nil {
		log.Fatalf("initializing task engine failed: %v", err)
	}
	cleanups = append(cleanups, cleanup)
	apiHandler, cleanup, err := serverapi.New(ctx, config, dbInstance, ps, embedder, execRepo, environmentExec, state, hooks, kvManager)
	cleanups = append(cleanups, cleanup)
	if err != nil {
		log.Fatalf("initializing API handler failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", apiHandler)

	port := config.Port
	log.Printf("starting server on :%s", port)
	if err := http.ListenAndServe(config.Addr+":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
