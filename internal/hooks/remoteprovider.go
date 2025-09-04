package hooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/taskengine"
)

type PersistentRepo struct {
	localHooks       map[string]taskengine.HookRepo
	dbInstance       libdb.DBManager
	httpClient       *http.Client
	protocolHandlers map[runtimetypes.HookProtocolType]ProtocolHandler
}

func NewPersistentRepo(
	localHooks map[string]taskengine.HookRepo,
	dbInstance libdb.DBManager,
	httpClient *http.Client,
) taskengine.HookRepo {
	handlers := make(map[runtimetypes.HookProtocolType]ProtocolHandler)
	handlers[runtimetypes.ProtocolOpenAI] = &OpenAIProtocol{}
	handlers[runtimetypes.ProtocolOllama] = &OllamaProtocol{}
	handlers[runtimetypes.ProtocolLangServeOpenAI] = &LangServeOpenAIProtocol{}
	handlers[runtimetypes.ProtocolLangServeDirect] = &LangServeDirectProtocol{}
	handlers[runtimetypes.ProtocolOpenAIObject] = &OpenAIObjectProtocol{}

	return &PersistentRepo{
		localHooks:       localHooks,
		dbInstance:       dbInstance,
		httpClient:       httpClient,
		protocolHandlers: handlers,
	}
}

// Exec remains the same.
func (p *PersistentRepo) Exec(
	ctx context.Context,
	startingTime time.Time,
	input any,
	args *taskengine.HookCall,
) (any, error) {
	// Check local hooks first
	if hook, ok := p.localHooks[args.Name]; ok {
		return hook.Exec(ctx, startingTime, input, args)
	}

	// Check remote
	storeInstance := runtimetypes.New(p.dbInstance.WithoutTransaction())
	remoteHook, err := storeInstance.GetRemoteHookByName(ctx, args.Name)
	if err != nil {
		return nil, fmt.Errorf("unknown hook: %s", args.Name)
	}

	if remoteHook.Name != args.Name {
		return nil, fmt.Errorf("internal consistency error: hook name mismatch: requested '%s', but found '%s'", args.Name, remoteHook.Name)
	}

	return p.execRemoteHook(ctx, remoteHook, startingTime, input, args)
}

// execRemoteHook is now refactored to use the protocol handlers.
func (p *PersistentRepo) execRemoteHook(
	ctx context.Context,
	hook *runtimetypes.RemoteHook,
	startingTime time.Time,
	input any,
	args *taskengine.HookCall,
) (any, error) {
	// Find the correct protocol handler from the map.
	handler, ok := p.protocolHandlers[hook.ProtocolType]
	if !ok {
		return nil, fmt.Errorf("unsupported protocol: %s", hook.ProtocolType)
	}

	argumentsMap := make(map[string]any)
	argumentsMap["input"] = input
	for key, val := range args.Args {
		argumentsMap[key] = val
	}

	jsonBody, err := handler.BuildRequest(hook.Name, argumentsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body for protocol %s: %w", hook.ProtocolType, err)
	}

	// Validate endpoint URL
	u, parseErr := url.Parse(hook.EndpointURL)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid endpoint URL format: %w", parseErr)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("endpoint URL must be absolute (include http:// or https://): %s", hook.EndpointURL)
	}
	if hook.TimeoutMs <= 0 {
		return nil, fmt.Errorf("timeout must be positive: %d", hook.TimeoutMs)
	}

	// Set up request with timeout
	timeout := time.Duration(hook.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(
		ctx,
		hook.Method,
		hook.EndpointURL,
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range hook.Headers {
		httpReq.Header.Set(key, value)
	}

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("remote hook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle non-successful status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySample := string(body)
		if len(bodySample) > 200 {
			bodySample = bodySample[:200] + "..."
		}
		return nil, fmt.Errorf("remote hook '%s' failed with status %d: %s", hook.Name, resp.StatusCode, bodySample)
	}

	// 2. Parse response using the specific protocol handler.
	output, err := handler.ParseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response for protocol %s: %w", hook.ProtocolType, err)
	}

	return output, nil
}

// Supports remains the same.
func (p *PersistentRepo) Supports(ctx context.Context) ([]string, error) {
	// Start with local hooks
	localSupported := make([]string, 0, len(p.localHooks))
	for k := range p.localHooks {
		localSupported = append(localSupported, k)
	}

	// Fetch all remote hooks by paginating through the store
	storeInstance := runtimetypes.New(p.dbInstance.WithoutTransaction())
	var remoteHooks []*runtimetypes.RemoteHook
	var lastCursor *time.Time
	limit := 100 // A reasonable page size

	for {
		page, err := storeInstance.ListRemoteHooks(ctx, lastCursor, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to list remote hooks: %w", err)
		}

		remoteHooks = append(remoteHooks, page...)

		// If the page size is less than the limit, we've reached the end
		if len(page) < limit {
			break
		}

		// Update the cursor for the next iteration
		lastCursor = &page[len(page)-1].CreatedAt
	}

	for _, hook := range remoteHooks {
		localSupported = append(localSupported, hook.Name)
	}

	return localSupported, nil
}
