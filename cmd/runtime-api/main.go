package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/contenox/runtime/internal/hooks"
	"github.com/contenox/runtime/internal/llmrepo"
	"github.com/contenox/runtime/internal/ollamatokenizer"
	"github.com/contenox/runtime/internal/runtimestate"
	"github.com/contenox/runtime/internal/serverapi"
	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	libroutine "github.com/contenox/runtime/libroutine"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/taskengine"
	"github.com/google/uuid"
)

var (
	cliSetTenancy  string
	Tenancy        = "96ed1c59-ffc1-4545-b3c3-191079c68d79"
	nodeInstanceID = "NODE-Instance-UNSET-dev"
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
		dbInstance, err = libdb.NewPostgresDBManager(ctx, dbURL, runtimetypes.Schema)
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
	if cliSetTenancy == "" {
		log.Fatalf("corrupted build! cliSetTenantID was not injected")
	}

	nodeInstanceID = uuid.NewString()[0:8]
	Tenancy = cliSetTenancy
	config := &serverapi.Config{}
	if err := serverapi.LoadConfig(config); err != nil {
		log.Fatalf("%s: failed to load configuration: %v", nodeInstanceID, err)
	}
	ctx := context.TODO()
	cleanups := []func() error{func() error {
		fmt.Printf("%s cleaning up", nodeInstanceID)
		return nil
	}}
	defer func() {
		for _, cleanup := range cleanups {
			err := cleanup()
			if err != nil {
				log.Printf("%s cleanup failed: %v", nodeInstanceID, err)
			}
		}
	}()
	fmt.Print("initialize the database")
	dbInstance, err := initDatabase(ctx, config)
	if err != nil {
		log.Fatalf("%s initializing database failed: %v", nodeInstanceID, err)
	}
	defer dbInstance.Close()

	ps, err := initPubSub(ctx, config)
	if err != nil {
		log.Fatalf("%s initializing PubSub failed: %v", nodeInstanceID, err)
	}
	if err != nil {
		log.Fatalf("%s initializing OpenSearch failed: %v", nodeInstanceID, err)
	}
	state, err := runtimestate.New(ctx, dbInstance, ps, runtimestate.WithPools())
	// state, err := runtimestate.New(ctx, dbInstance, ps)
	if err != nil {
		log.Fatalf("%s initializing runtime state failed: %v", nodeInstanceID, err)
	}
	cl, err := strconv.Atoi(config.EmbedModelContextLength)
	if err != nil {
		log.Fatalf("%s parsing embed model context length failed: %v", nodeInstanceID, err)
	}
	err = runtimestate.InitEmbeder(ctx, &runtimestate.Config{
		DatabaseURL: config.DatabaseURL,
		EmbedModel:  config.EmbedModel,
		TaskModel:   config.TaskModel,
		TenantID:    Tenancy,
	}, dbInstance, cl, state)
	if err != nil {
		log.Fatalf("%s initializing embedding pool failed: %v", nodeInstanceID, err)
	}
	tokenizerSvc, cleanup, err := ollamatokenizer.NewHTTPClient(ctx, ollamatokenizer.ConfigHTTP{
		BaseURL: config.TokenizerServiceURL,
	})
	if err != nil {
		cleanup()
		log.Fatalf("%s initializing tokenizer service failed: %v", nodeInstanceID, err)
	}
	tcl, err := strconv.Atoi(config.TaskModelContextLength)
	if err != nil {
		log.Fatalf("%s parsing task model context length failed: %v", nodeInstanceID, err)
	}
	err = runtimestate.InitPromptExec(ctx, &runtimestate.Config{
		DatabaseURL: config.DatabaseURL,
		TaskModel:   config.TaskModel,
		EmbedModel:  config.EmbedModel,
		TenantID:    Tenancy,
	}, dbInstance, state, tcl)
	if err != nil {
		log.Fatalf("%s initializing promptexec failed: %v", nodeInstanceID, err)
	}
	tcl, err = strconv.Atoi(config.ChatModelContextLength)
	if err != nil {
		log.Fatalf("%s parsing chat model context length failed: %v", nodeInstanceID, err)
	}
	err = runtimestate.InitChatExec(ctx, &runtimestate.Config{
		DatabaseURL: config.DatabaseURL,
		ChatModel:   config.ChatModel,
		TenantID:    Tenancy,
	}, dbInstance, state, tcl)
	if err != nil {
		log.Fatalf("%s initializing task model failed: %v", nodeInstanceID, err)
	}
	cleanups = append(cleanups, cleanup)
	if err != nil {
		log.Fatalf("%s initializing vector store failed: %v", nodeInstanceID, err)
	}

	// tracker := taskengine.NewKVActivityTracker(kvManager)
	stdOuttracker := libtracker.NewLogActivityTracker(slog.Default())
	serveropsChainedTracker := libtracker.ChainedTracker{
		// tracker,
		stdOuttracker,
	}
	repo, err := llmrepo.NewModelManager(state, tokenizerSvc, llmrepo.ModelManagerConfig{
		DefaultPromptModel: llmrepo.ModelConfig{
			Name:     config.TaskModel,
			Provider: config.TaskProvider,
		},
		DefaultEmbeddingModel: llmrepo.ModelConfig{
			Name:     config.EmbedModel,
			Provider: config.EmbedProvider,
		},
		DefaultChatModel: llmrepo.ModelConfig{
			Name:     config.ChatModel,
			Provider: config.ChatProvider,
		},
	})
	if err != nil {
		log.Fatalf("%s initializing llm repo failed: %v", nodeInstanceID, err)
	}
	// Create persistent hook repo
	hookRepo := hooks.NewPersistentRepo(map[string]taskengine.HookRepo{}, dbInstance, http.DefaultClient)
	exec, err := taskengine.NewExec(ctx, repo, hookRepo, serveropsChainedTracker)
	if err != nil {
		log.Fatalf("%s initializing task engine engine failed: %v", nodeInstanceID, err)
	}
	environmentExec, err := taskengine.NewEnv(ctx, serveropsChainedTracker, exec, taskengine.NewSimpleInspector())
	if err != nil {
		log.Fatalf("%s initializing task engine failed: %v", nodeInstanceID, err)
	}
	cleanups = append(cleanups, cleanup)

	apiHandler, cleanup, err := serverapi.New(ctx, nodeInstanceID, Tenancy, config, dbInstance, ps, repo, environmentExec, state, hookRepo)
	cleanups = append(cleanups, cleanup)
	if err != nil {
		log.Fatalf("%s initializing API handler failed: %v", nodeInstanceID, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", apiHandler)
	port := config.Port
	log.Printf("%s %s starting server on :%s", Tenancy, nodeInstanceID, port)
	if err := http.ListenAndServe(config.Addr+":"+port, mux); err != nil {
		log.Fatalf("%s server failed: %v", nodeInstanceID, err)
	}
}
