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
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/hitlpolicyapi"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/internal/mcpserverapi"
	"github.com/contenox/runtime/runtime/internal/modeldapi"
	"github.com/contenox/runtime/runtime/internal/modelregistryapi"
	"github.com/contenox/runtime/runtime/internal/openapidocs"
	"github.com/contenox/runtime/runtime/internal/providerapi"
	"github.com/contenox/runtime/runtime/internal/setupapi"
	"github.com/contenox/runtime/runtime/internal/taskchainapi"
	"github.com/contenox/runtime/runtime/internal/taskeventsapi"
	"github.com/contenox/runtime/runtime/internal/taskexecapi"
	"github.com/contenox/runtime/runtime/internal/terminalapi"
	"github.com/contenox/runtime/runtime/internal/toolsapi"
	"github.com/contenox/runtime/runtime/terminalservice"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/mcpserverservice"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelregistryservice"
	"github.com/contenox/runtime/runtime/providerservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
	"github.com/contenox/runtime/runtime/version"
)

// Config holds the HTTP serving configuration for `contenox serve`.
type Config struct {
	Addr              string `json:"addr"`
	Port              string `json:"port"`
	Token             string `json:"token"`
	UIBaseURL         string `json:"ui_base_url"`
	AllowedAPIOrigins string `json:"allowed_api_origins"`
	ProxyOrigin       string `json:"proxy_origin"`
	BeamDevProxyURL      string `json:"beam_dev_proxy_url"`
	TerminalEnabled      string `json:"terminal_enabled"`
	TerminalAllowedRoot  string `json:"terminal_allowed_root"`
	TerminalShell        string `json:"terminal_shell"`
	TerminalIdleTimeout  string `json:"terminal_idle_timeout"`
	TerminalMaxSessions  string `json:"terminal_max_sessions"`
}

// Dependencies are the services the product routes are mounted on. All fields
// are optional: a route group is only registered when its dependencies are
// present, so a bare runtime still serves /health and /version.
type Dependencies struct {
	DB                   libdb.DBManager
	PubSub               libbus.Messenger
	State                *runtimestate.State
	ToolsProviderService toolsproviderservice.Service
	Auth                 middleware.AuthZReader
	Agent                agentservice.Agent
	Chains               taskchainservice.Service
	WorkspaceID          string
	ProjectRoot          string
	ContenoxDir          string
	Defaults             stateservice.RuntimeDefaults
	TerminalService      terminalservice.Service
	TerminalEnabled      bool
}

// New registers the spine routes (not-found shape, health, version) on mux and,
// when deps are supplied, the product API routes. Returns a cleanup function.
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
	backendapi.AddModelRoutes(mux, stateSvc, deps.Defaults)
	backendapi.AddBackendRoutes(mux, backendSvc, stateSvc)
	modeldapi.AddRoutes(mux, modeldapi.WithStateReader(stateSvc))

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

	if deps.Agent != nil {
		taskexecapi.AddRoutes(mux, deps.Agent, deps.Auth, stateSvc, deps.Defaults)
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

// Handler wraps a mux with the standard middleware chain: CORS, request ID,
// tracing, and local API request protection.
func Handler(mux *http.ServeMux, config *Config) http.Handler {
	if config == nil {
		config = &Config{}
	}
	cors := &middleware.CORSConfig{
		AllowedAPIOrigins: firstNonEmpty(config.AllowedAPIOrigins, middleware.DefaultAllowedAPIOrigins),
		AllowedMethods:    middleware.DefaultAllowedMethods,
		AllowedHeaders:    middleware.DefaultAllowedHeaders,
		ProxyOrigin:       config.ProxyOrigin,
	}

	var h http.Handler = mux
	h = ProtectMutatingAPIWithAllowedOrigins(config.Token, config.AllowedAPIOrigins, h)
	h = apiframework.TracingMiddleware(h)
	h = apiframework.RequestIDMiddleware(h)
	h = middleware.EnableCORS(cors, h)
	return h
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// LoadConfig populates cfg from environment variables (lowercased keys mapped
// to json tags).
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
		config[strings.ToLower(ar[0])] = ar[1]
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
