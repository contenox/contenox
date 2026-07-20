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
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/agentregistryapi"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/fleetapi"
	"github.com/contenox/runtime/runtime/internal/hitlpolicyapi"
	"github.com/contenox/runtime/runtime/internal/localfileapi"
	"github.com/contenox/runtime/runtime/internal/mcpserverapi"
	"github.com/contenox/runtime/runtime/internal/missionapi"
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
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/mcpserverservice"
	"github.com/contenox/runtime/runtime/missionservice"
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
	"github.com/contenox/runtime/runtime/vfs"
)

// Config holds the HTTP serving configuration for `contenox serve`.
type Config struct {
	Addr                string `json:"addr"`
	Port                string `json:"port"`
	Token               string `json:"token"`
	UIBaseURL           string `json:"ui_base_url"`
	AllowedAPIOrigins   string `json:"allowed_api_origins"`
	ProxyOrigin         string `json:"proxy_origin"`
	BeamDevProxyURL     string `json:"beam_dev_proxy_url"`
	TerminalEnabled     string `json:"terminal_enabled"`
	TerminalAllowedRoot string `json:"terminal_allowed_root"`
	TerminalShell       string `json:"terminal_shell"`
	TerminalIdleTimeout string `json:"terminal_idle_timeout"`
	TerminalMaxSessions string `json:"terminal_max_sessions"`
	// WorkspaceRoots is the operator's allowlist of directories a browser client
	// may choose as a session workspace, separated by the OS path-list separator
	// (":" on POSIX). The serve directory is always the default root; these
	// extend the allowlist. Also settable via `--workspace-root` flags and the
	// `contenox serve [dir]` positional arguments.
	WorkspaceRoots string `json:"workspace_roots"`
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
	// Instances is serve's live agent-instance manager; the /fleet routes surface
	// its config+runtime join (declared agents annotated with running instances).
	Instances agentinstance.Manager
	// Missions is the durable mission registry; the /missions routes surface it.
	// The other half of the manifest — one-line intents bound to fleet work.
	Missions missionservice.Service
	// Tracker records after-the-fact fleet facts — currently the outcome of a
	// dispatch's detached first prompt (POST /fleet/dispatch). Optional: nil
	// degrades to a Noop, matching the passive-telemetry gate the rest of the
	// fleet rides.
	Tracker     libtracker.ActivityTracker
	WorkspaceID string
	ProjectRoot string
	ContenoxDir string
	// WorkspaceRoots is the workspace-root allowlist. When set, the /files browse
	// API resolves each request against a client-supplied `root` (validated
	// through the allowlist) instead of the single fixed ProjectRoot.
	WorkspaceRoots  *vfs.Factory
	Defaults        stateservice.RuntimeDefaults
	TerminalService terminalservice.Service
	TerminalEnabled bool
	// HITLPolicySource and HITLDefaultPolicyName feed the /files `agent` view
	// filter: verdicts are computed by the same HITL policy engine the live agent
	// uses. When HITLPolicySource is nil the filter is unavailable (the raw tree
	// still serves). HITLDefaultPolicyName is the fallback policy when a request
	// omits `policy`; empty means the service's built-in default.
	HITLPolicySource      hitlservice.PolicySource
	HITLDefaultPolicyName string
}

// emptyKVReader is a KVReader whose lookups always miss, forcing a hitlservice
// to use its constructor fallback policy rather than the active-policy KV key.
// It is how the /files `agent` filter pins evaluation to an explicitly requested
// policy name.
type emptyKVReader struct{}

func (emptyKVReader) GetKV(context.Context, string, interface{}) error {
	return fmt.Errorf("serverapi: no active policy override")
}

// workspaceHITLFactory builds the PolicyEvaluatorFactory the /files `agent`
// filter uses, mirroring how serve constructs its HITL service (same
// PolicySource, tenant, and KV store) so the API and the runtime agree. An empty
// requested policy name defers to the runtime's default resolution (reads the
// active-policy KV key, then HITLDefaultPolicyName); a named policy is pinned via
// emptyKVReader so it is evaluated verbatim. Returns nil when no PolicySource or
// DB is configured, which disables the filter.
func workspaceHITLFactory(deps Dependencies) localfileapi.PolicyEvaluatorFactory {
	if deps.HITLPolicySource == nil || deps.DB == nil {
		return nil
	}
	src := deps.HITLPolicySource
	defaultPolicy := deps.HITLDefaultPolicyName
	dbm := deps.DB
	return func(policyName string) hitlservice.Service {
		store := runtimetypes.New(dbm.WithoutTransaction())
		if strings.TrimSpace(policyName) == "" {
			return hitlservice.NewWithDefaultPolicy(src, runtimetypes.LocalTenantID, store, libtracker.NoopTracker{}, defaultPolicy)
		}
		return hitlservice.NewWithDefaultPolicy(src, runtimetypes.LocalTenantID, emptyKVReader{}, libtracker.NoopTracker{}, policyName)
	}
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

	// Read-only declared-agents registry (registration stays with `contenox
	// agent`). Mounted here where deps.DB is guaranteed non-nil, not gated on
	// PubSub — listing agents needs only the store.
	agentregistryapi.AddAgentRegistryRoutes(mux, agentregistryservice.New(deps.DB))

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

	// The /files browse API is the file-explorer data source. When a workspace
	// allowlist is configured (serve), it is per-root: each request names a
	// `root` (the session's chosen workspace), validated through the allowlist,
	// so a browser can browse any allowlisted root but nothing outside it. When
	// no allowlist is configured, it stays rooted at the single fixed ProjectRoot
	// (unchanged legacy behavior).
	if deps.WorkspaceRoots != nil {
		if err := localfileapi.AddWorkspaceRoutes(mux, deps.WorkspaceRoots, workspaceHITLFactory(deps)); err != nil {
			return fmt.Errorf("workspace files: %w", err)
		}
	} else if deps.ProjectRoot != "" {
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

	// Live-fleet counterpart of the declared-agents registry above: the
	// config+runtime join lives only in serve's Manager, so the routes exist
	// only when serve passes it. Dispatch additionally uses the mission registry,
	// the workspace-root allowlist, the project root, and the tracker — all
	// optional (dispatch validates a missionIntent against a nil registry, and a
	// nil tracker degrades to a Noop).
	if deps.Instances != nil {
		fleetapi.AddRoutes(mux, deps.Instances, deps.Missions, deps.WorkspaceRoots, deps.ProjectRoot, deps.Tracker)
	}

	// Durable manifest half of the fleet: mission records. Registered only when
	// serve builds the service, mirroring the other nil-gated route groups.
	if deps.Missions != nil {
		missionapi.AddRoutes(mux, deps.Missions)
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
	h = ProtectAPI(config.Token, config.AllowedAPIOrigins, h)
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
