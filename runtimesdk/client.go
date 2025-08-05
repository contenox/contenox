package runtimesdk

import (
	"net/http"

	"github.com/contenox/runtime/backendservice"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/hookproviderservice"
	"github.com/contenox/runtime/modelservice"
	"github.com/contenox/runtime/poolservice"
	"github.com/contenox/runtime/providerservice"
)

// Client is the main SDK client that provides access to all services
type Client struct {
	BackendService  backendservice.Service
	ModelService    modelservice.Service
	PoolService     poolservice.Service
	HookService     hookproviderservice.Service
	ExecService     execservice.ExecService
	EnvService      execservice.TasksEnvService
	ProviderService providerservice.Service
}

// Config holds configuration for the SDK client
type Config struct {
	BaseURL string
	Token   string
}

// NewClient creates a new SDK client with the provided configuration
func NewClient(config Config, http *http.Client) (*Client, error) {
	return &Client{
		BackendService:  NewHTTPBackendService(config.BaseURL, config.Token, http),
		ModelService:    NewHTTPModelService(config.BaseURL, config.Token, http),
		PoolService:     NewHTTPPoolService(config.BaseURL, config.Token, http),
		HookService:     NewHTTPRemoteHookService(config.BaseURL, config.Token, http),
		ExecService:     NewHTTPExecService(config.BaseURL, config.Token, http),
		EnvService:      NewHTTPTasksEnvService(config.BaseURL, config.Token, http),
		ProviderService: NewHTTPProviderService(config.BaseURL, config.Token, http),
	}, nil
}
