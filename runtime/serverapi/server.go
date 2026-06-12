package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/approvalapi"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/hitlpolicyapi"
	"github.com/contenox/runtime/runtime/internal/internalchatapi"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/internal/mcpserverapi"
	"github.com/contenox/runtime/runtime/internal/modelregistryapi"
	"github.com/contenox/runtime/runtime/internal/openapidocs"
	"github.com/contenox/runtime/runtime/internal/providerapi"
	"github.com/contenox/runtime/runtime/internal/setupapi"
	"github.com/contenox/runtime/runtime/internal/taskchainapi"
	"github.com/contenox/runtime/runtime/internal/taskeventsapi"
	"github.com/contenox/runtime/runtime/internal/taskexecapi"
	"github.com/contenox/runtime/runtime/internal/terminalapi"
	"github.com/contenox/runtime/runtime/internal/toolsapi"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/mcpserverservice"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelregistryservice"
	"github.com/contenox/runtime/runtime/providerservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/terminalservice"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
	"github.com/contenox/runtime/runtime/version"
)

type Config struct {
	DatabaseURL         string `json:"database_url"`
	Port                string `json:"port"`
	Addr                string `json:"addr"`
	OllamaCompat        string `json:"ollama_compat"`
	NATSURL             string `json:"nats_url"`
	NATSUser            string `json:"nats_user"`
	NATSPassword        string `json:"nats_password"`
	TokenizerServiceURL string `json:"tokenizer_service_url"`
	Token               string `json:"token"`
	UIBaseURL           string `json:"ui_base_url"`
	ValkeyAddr          string `json:"valkey_addr"`
	ValkeyPassword      string `json:"valkey_password"`
	TerminalEnabled     string `json:"terminal_enabled"`
	TerminalAllowedRoot string `json:"terminal_allowed_root"`
	TerminalShell       string `json:"terminal_shell"`
	TerminalIdleTimeout string `json:"terminal_idle_timeout"`
}

type Dependencies struct {
	DB                   libdb.DBManager
	PubSub               libbus.Messenger
	State                *runtimestate.State
	ToolsProviderService toolsproviderservice.Service
	Auth                 middleware.AuthZReader
	Agent                agentservice.Agent
	ChatManager          *chatservice.Manager
	Chains               taskchainservice.Service
	HITLService          hitlservice.Service
	TerminalService      terminalservice.Service
	TerminalEnabled      bool
	WorkspaceID          string
	ProjectRoot          string
	ContenoxDir          string
	DefaultChainRef      string
	DefaultModel         string
	DefaultProvider      string
	AltDefaultModel      string
	AltDefaultProvider   string
	DefaultMaxTokens     string
	DefaultThink         string
}

func New(ctx context.Context, mux *http.ServeMux, nodeInstanceID, tenancy string, config *Config, deps ...Dependencies) (func() error, error) {
	if mux == nil {
		return nil, fmt.Errorf("serverapi: mux is nil")
	}
	if config == nil {
		config = &Config{}
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		_ = apiframework.Error(w, r, apiframework.ErrNotFound, apiframework.ListOperation)
	})
	AddHealthRoutes(mux)
	AddVersionRoutes(mux, version.Get(), nodeInstanceID, tenancy)
	openapidocs.Register(mux)
	if len(deps) > 0 {
		if err := registerProductRoutes(ctx, mux, config, deps[0]); err != nil {
			return nil, err
		}
	}
	return func() error { return nil }, nil
}

func registerProductRoutes(ctx context.Context, mux *http.ServeMux, config *Config, deps Dependencies) error {
	_ = ctx
	if deps.DB == nil || deps.State == nil {
		return nil
	}

	store := runtimetypes.New(deps.DB.WithoutTransaction())
	backendSvc := backendservice.New(deps.DB)
	stateSvc := stateservice.New(deps.State, deps.DB, deps.WorkspaceID)

	backendapi.AddStateRoutes(mux, stateSvc)
	backendapi.AddModelRoutes(mux, stateSvc, deps.DefaultModel)
	backendapi.AddBackendRoutes(mux, backendSvc, stateSvc)

	registrySvc := modelregistryservice.New(deps.DB)
	registry := modelregistry.New(registrySvc)
	modelregistryapi.AddRoutes(mux, registrySvc, registry, backendSvc, store)

	setupapi.AddSetupRoutes(mux, stateSvc, deps.Auth)
	providerSvc := providerservice.New(deps.DB, deps.WorkspaceID)
	providerapi.AddProviderRoutes(mux, providerSvc)

	if deps.ProjectRoot != "" {
		projectFiles, err := localfileservice.New(deps.ProjectRoot)
		if err != nil {
			return fmt.Errorf("project files: %w", err)
		}
		localfileapi.AddRoutes(mux, projectFiles)
	}
	chains := deps.Chains
	if deps.ContenoxDir != "" {
		chainFiles, err := localfileservice.New(deps.ContenoxDir)
		if err != nil {
			return fmt.Errorf("chain files: %w", err)
		}
		if chains == nil {
			chains = taskchainservice.NewLocal(chainFiles)
		}
		taskchainapi.AddTaskChainRoutes(mux, chains)
		hitlpolicyapi.AddRoutes(mux, chainFiles)
	}

	if deps.Agent != nil && deps.ChatManager != nil && chains != nil {
		internalchatapi.AddChatRoutes(mux, internalchatapi.ChatDeps{
			Agent:              deps.Agent,
			ChatMgr:            deps.ChatManager,
			Chains:             chains,
			DB:                 deps.DB,
			DefaultChainRef:    deps.DefaultChainRef,
			DefaultModel:       deps.DefaultModel,
			DefaultProvider:    deps.DefaultProvider,
			AltDefaultModel:    deps.AltDefaultModel,
			AltDefaultProvider: deps.AltDefaultProvider,
			DefaultMaxTokens:   deps.DefaultMaxTokens,
			DefaultThink:       deps.DefaultThink,
		}, deps.Auth)
	}
	if deps.Agent != nil {
		taskexecapi.AddRoutes(mux, deps.Agent, deps.Auth, taskexecapi.Defaults{
			Model:       deps.DefaultModel,
			Provider:    deps.DefaultProvider,
			AltModel:    deps.AltDefaultModel,
			AltProvider: deps.AltDefaultProvider,
			MaxTokens:   deps.DefaultMaxTokens,
			Think:       deps.DefaultThink,
		})
	}
	if deps.HITLService != nil {
		approvalapi.AddRoutes(mux, deps.HITLService, deps.Auth)
	}
	if deps.TerminalService != nil {
		terminalapi.AddRoutes(mux, deps.TerminalService, deps.Auth, deps.TerminalEnabled, config.Token)
	}

	if deps.ToolsProviderService != nil {
		toolsapi.AddRemoteToolsRoutes(mux, deps.ToolsProviderService)
	}

	if deps.PubSub != nil {
		taskeventsapi.AddRoutes(mux, deps.PubSub, deps.Auth)
		mcpSvc := mcpserverservice.New(deps.DB, mcpserverservice.WithUIBaseURL(config.UIBaseURL))
		mcpserverapi.AddMCPServerRoutes(mux, mcpSvc, deps.PubSub, deps.Auth)
	}
	return nil
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
	if err := json.Unmarshal(b, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal into config struct: %w", err)
	}

	return nil
}
