package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/contenox/runtime/taskengine"
)

type ProtocolHandler interface {
	// BuildRequest creates the HTTP request body for the given protocol.
	BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error)

	// ParseResponse extracts the meaningful output from the HTTP response body.
	ParseResponse(body []byte) (any, error)

	// FetchSchema retrieves the function schema from the tool server's schema endpoint.
	// It should return (nil, nil) if schema fetching is not applicable for the protocol.
	FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error)

	// FetchTools retrieves the tools from the tool server's schema endpoint
	// and converts them into the Tool struct format.
	FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error)
}

type OpenAIProtocol struct{}

func (p *OpenAIProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	mergedArgs := make(map[string]any)
	for k, v := range bodyProperties {
		mergedArgs[k] = v
	}
	for k, v := range argsMap {
		mergedArgs[k] = v
	}

	argsJSON, err := json.Marshal(mergedArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args for openai: %w", err)
	}
	return json.Marshal(taskengine.FunctionCall{
		Name:      toolName,
		Arguments: string(argsJSON),
	})
}

func (p *OpenAIProtocol) ParseResponse(body []byte) (any, error) {
	var output any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return output, nil
}

func (p *OpenAIProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error) {
	// Standard OpenAI APIs do not have a separate schema endpoint.
	// This functionality is assumed for OpenAI-compatible servers that adopt this convention (e.g., LangServe).
	// If a server doesn't support this, the request will fail, which is handled gracefully.
	schemaURL := strings.TrimRight(endpointURL, "/") + "/schema"

	req, err := http.NewRequestWithContext(ctx, "GET", schemaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schema request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Not considered a fatal error, just means no schema is available.
		return nil, nil
	}

	var schema map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	return schema, nil
}

func (p *OpenAIProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	schema, err := p.FetchSchema(ctx, endpointURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schema for tools: %w", err)
	}
	if schema == nil {
		// No schema available, or protocol does not support it.
		return nil, nil
	}

	// The schema is expected to contain a "functions" key with a list of function definitions,
	// as shown in the example server implementation.
	rawFuncs, ok := schema["functions"]
	if !ok {
		// If the 'functions' key is missing, assume no tools are defined.
		return []taskengine.Tool{}, nil
	}

	funcsList, ok := rawFuncs.([]interface{})
	if !ok {
		return nil, fmt.Errorf("schema's 'functions' field is not a list, but %T", rawFuncs)
	}

	var tools []taskengine.Tool
	for _, item := range funcsList {
		funcMap, ok := item.(map[string]interface{})
		if !ok {
			// Skip invalid entries in the functions list.
			continue
		}

		name, ok := funcMap["name"].(string)
		if !ok || name == "" {
			// A tool function must have a name.
			continue
		}

		var description string
		if d, ok := funcMap["description"].(string); ok {
			description = d
		}

		tool := taskengine.Tool{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        name,
				Description: description,
				Parameters:  funcMap["parameters"], // Parameters can be any valid JSON schema object.
			},
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

type LangServeOpenAIProtocol struct{}

func (p *LangServeOpenAIProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	return (&OpenAIProtocol{}).BuildRequest(toolName, argsMap, bodyProperties)
}

func (p *LangServeOpenAIProtocol) ParseResponse(body []byte) (any, error) {
	var output map[string]any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if inner, ok := output["output"]; ok {
		return inner, nil
	}
	return nil, fmt.Errorf("langserve-openai response missing 'output' field in response body: %s", string(body))
}

func (p *LangServeOpenAIProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error) {
	// LangServe typically exposes a /schema endpoint.
	return (&OpenAIProtocol{}).FetchSchema(ctx, endpointURL, httpClient)
}

func (p *LangServeOpenAIProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	return (&OpenAIProtocol{}).FetchTools(ctx, endpointURL, httpClient)
}

type OllamaProtocol struct{}

func (p *OllamaProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	mergedArgs := make(map[string]any)
	for k, v := range bodyProperties {
		mergedArgs[k] = v
	}
	for k, v := range argsMap {
		mergedArgs[k] = v
	}

	return json.Marshal(taskengine.FunctionCallObject{
		Name:      toolName,
		Arguments: mergedArgs,
	})
}

func (p *OllamaProtocol) ParseResponse(body []byte) (any, error) {
	var output map[string]any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if msg, ok := output["message"].(map[string]any); ok {
		if content, ok := msg["content"]; ok {
			return content, nil
		}
	}
	return nil, fmt.Errorf("ollama response missing 'message.content' field in response body: %s", string(body))
}

func (p *OllamaProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error) {
	// The standard Ollama API does not provide a separate endpoint for tool schemas.
	// Therefore, schema fetching is not supported for this protocol.
	return nil, nil
}

func (p *OllamaProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	// The standard Ollama API does not provide a separate endpoint for tool schemas.
	return nil, nil
}

type LangServeDirectProtocol struct{}

func (p *LangServeDirectProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	mergedArgs := make(map[string]any)
	for k, v := range bodyProperties {
		mergedArgs[k] = v
	}
	for k, v := range argsMap {
		mergedArgs[k] = v
	}

	return json.Marshal(mergedArgs)
}

func (p *LangServeDirectProtocol) ParseResponse(body []byte) (any, error) {
	var output any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return output, nil
}

func (p *LangServeDirectProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error) {
	return (&OpenAIProtocol{}).FetchSchema(ctx, endpointURL, httpClient)
}

func (p *LangServeDirectProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	return (&OpenAIProtocol{}).FetchTools(ctx, endpointURL, httpClient)
}

type OpenAIObjectProtocol struct{}

func (p *OpenAIObjectProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	return (&OllamaProtocol{}).BuildRequest(toolName, argsMap, bodyProperties)
}

func (p *OpenAIObjectProtocol) ParseResponse(body []byte) (any, error) {
	return (&OpenAIProtocol{}).ParseResponse(body)
}

func (p *OpenAIObjectProtocol) FetchSchema(ctx context.Context, endpointURL string, httpClient *http.Client) (map[string]interface{}, error) {
	return (&OpenAIProtocol{}).FetchSchema(ctx, endpointURL, httpClient)
}

func (p *OpenAIObjectProtocol) FetchTools(ctx context.Context, endpointURL string, httpClient *http.Client) ([]taskengine.Tool, error) {
	return (&OpenAIProtocol{}).FetchTools(ctx, endpointURL, httpClient)
}
