package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/taskengine"
)

// ProtocolHandler defines the contract for building requests and parsing responses
// for a specific remote hook protocol.
type ProtocolHandler interface {
	// BuildRequest creates the HTTP request body for the given protocol.
	BuildRequest(toolName string, argsMap map[string]any) ([]byte, error)

	// ParseResponse extracts the meaningful output from the HTTP response body.
	ParseResponse(body []byte) (any, error)
}

type OpenAIProtocol struct{}

func (p *OpenAIProtocol) BuildRequest(toolName string, argsMap map[string]any) ([]byte, error) {
	argsJSON, err := json.Marshal(argsMap)
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

func (p *LangServeOpenAIProtocol) BuildRequest(toolName string, argsMap map[string]any) ([]byte, error) {
	return (&OpenAIProtocol{}).BuildRequest(toolName, argsMap)
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

func (p *OllamaProtocol) BuildRequest(toolName string, argsMap map[string]any) ([]byte, error) {
	return json.Marshal(taskengine.FunctionCallObject{
		Name:      toolName,
		Arguments: argsMap,
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

func (p *LangServeDirectProtocol) BuildRequest(toolName string, argsMap map[string]any) ([]byte, error) {
	return json.Marshal(argsMap)
}

func (p *LangServeDirectProtocol) ParseResponse(body []byte) (any, error) {
	var output any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	return output, nil
}

type OpenAIObjectProtocol struct{}

func (p *OpenAIObjectProtocol) BuildRequest(toolName string, argsMap map[string]any) ([]byte, error) {
	return (&OllamaProtocol{}).BuildRequest(toolName, argsMap)
}

func (p *OpenAIObjectProtocol) ParseResponse(body []byte) (any, error) {
	return (&OpenAIProtocol{}).ParseResponse(body)
}
