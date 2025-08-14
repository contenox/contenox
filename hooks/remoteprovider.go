package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/contenox/runtime/taskengine"
)

type PersistentRepo struct {
	localHooks map[string]taskengine.HookRepo
	dbInstance libdb.DBManager
	httpClient *http.Client
}

func NewPersistentRepo(
	localHooks map[string]taskengine.HookRepo,
	dbInstance libdb.DBManager,
	httpClient *http.Client,
) taskengine.HookRepo {
	return &PersistentRepo{
		localHooks: localHooks,
		dbInstance: dbInstance,
		httpClient: httpClient,
	}
}

func (p *PersistentRepo) Exec(
	ctx context.Context,
	startingTime time.Time,
	input any,
	dataType taskengine.DataType,
	transition string,
	args *taskengine.HookCall,
) (any, taskengine.DataType, string, error) {
	// Check local hooks first
	if hook, ok := p.localHooks[args.Name]; ok {
		return hook.Exec(ctx, startingTime, input, dataType, transition, args)
	}

	// Check remote
	storeInstance := runtimetypes.New(p.dbInstance.WithoutTransaction())
	remoteHook, err := storeInstance.GetRemoteHookByName(ctx, args.Name)
	if err != nil {
		return nil, taskengine.DataTypeAny, transition,
			fmt.Errorf("unknown hook: %s", args.Name)
	}

	return p.execRemoteHook(ctx, remoteHook, startingTime, input, dataType, transition, args)
}

func (p *PersistentRepo) execRemoteHook(
	ctx context.Context,
	hook *runtimetypes.RemoteHook,
	startingTime time.Time,
	input any,
	dataType taskengine.DataType,
	transition string,
	args *taskengine.HookCall,
) (any, taskengine.DataType, string, error) {
	request := struct {
		StartingTime time.Time            `json:"startingTime"`
		Input        any                  `json:"input"`
		DataType     string               `json:"dataType"`
		Transition   string               `json:"transition"`
		Args         *taskengine.HookCall `json:"args"`
	}{
		StartingTime: startingTime,
		Input:        input,
		DataType:     dataType.String(),
		Transition:   transition,
		Args:         args,
	}

	jsonBody, err := json.Marshal(request)
	if err != nil {
		return nil, taskengine.DataTypeAny, transition,
			fmt.Errorf("failed to marshal request: %w", err)
	}

	timeout := time.Duration(hook.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(
		ctx,
		hook.Method,
		hook.EndpointURL,
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return nil, taskengine.DataTypeAny, transition,
			fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := p.httpClient
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, taskengine.DataTypeAny, transition,
			fmt.Errorf("remote hook request failed: %w", err)
	}
	defer resp.Body.Close()
	errorStatus := false
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorStatus = true
	}
	response := struct {
		Output     any    `json:"output"`
		DataType   string `json:"dataType"`
		Error      string `json:"error,omitempty"`
		Transition string `json:"transition"`
	}{}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if errorStatus {
			return nil, taskengine.DataTypeAny, fmt.Sprint(resp.StatusCode),
				fmt.Errorf("failed to read response body: %w", err)
		}
		return nil, taskengine.DataTypeAny, fmt.Sprint(resp.StatusCode),
			fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &response); err != nil {
		if errorStatus {
			return nil, taskengine.DataTypeAny, fmt.Sprint(resp.StatusCode),
				fmt.Errorf("failed to parse response: %w", err)
		}
		return nil, taskengine.DataTypeAny, fmt.Sprint(resp.StatusCode),
			fmt.Errorf("failed to parse response: %w", err)
	}

	dt, err := taskengine.DataTypeFromString(response.DataType)
	if err != nil {
		if errorStatus {
			return nil, dt, fmt.Sprint(resp.StatusCode),
				fmt.Errorf("invalid data type in response: %s", response.DataType)
		}
		return nil, taskengine.DataTypeAny, fmt.Sprint(resp.StatusCode),
			fmt.Errorf("invalid data type in response: %s", response.DataType)
	}
	if response.Error != "" {
		err = errors.New(response.Error)
	} else if errorStatus {
		err = fmt.Errorf("failed with status %d", resp.StatusCode)
	}
	return response.Output, dt, response.Transition, err
}

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
