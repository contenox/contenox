package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/taskengine"
)

type ProtocolHandler interface {
	// BuildRequest creates the HTTP request body for the given protocol.
	BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error)

	// ParseResponse extracts the meaningful output from the HTTP response body.
	ParseResponse(body []byte) (any, error)
}

type OpenAIProtocol struct{}

func (p *OpenAIProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	// Merge body properties into args
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

type OllamaProtocol struct{}

func (p *OllamaProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	// Merge body properties into args
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

type LangServeDirectProtocol struct{}

func (p *LangServeDirectProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	// Merge body properties into args
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

type OpenAIObjectProtocol struct{}

func (p *OpenAIObjectProtocol) BuildRequest(toolName string, argsMap map[string]any, bodyProperties map[string]any) ([]byte, error) {
	return (&OllamaProtocol{}).BuildRequest(toolName, argsMap, bodyProperties)
}

func (p *OpenAIObjectProtocol) ParseResponse(body []byte) (any, error) {
	return (&OpenAIProtocol{}).ParseResponse(body)
}

// OpenAIProtocol
func (p *OpenAIProtocol) GetFunctionSchema(toolName string) (map[string]interface{}, error) {
	// For OpenAI protocol, we need to know the specific function schema
	// This would typically be fetched from a registry or configuration
	return map[string]interface{}{
		"name":        toolName,
		"description": "Function executed via OpenAI protocol",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "any",
					"description": "Input data for the function",
				},
			},
			"required": []string{"input"},
		},
	}, nil
}

// LangServeOpenAIProtocol
func (p *LangServeOpenAIProtocol) GetFunctionSchema(toolName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":        toolName,
		"description": "Function executed via LangServe OpenAI protocol",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "any",
					"description": "Input data for the function",
				},
			},
			"required": []string{"input"},
		},
	}, nil
}
