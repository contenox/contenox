package execapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/embedservice"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/taskengine"
)

func AddExecRoutes(mux *http.ServeMux, promptService execservice.ExecService, taskService execservice.TasksEnvService, embedService embedservice.Service) {
	f := &taskManager{
		promptService: promptService,
		taskService:   taskService,
		embedService:  embedService,
	}
	mux.HandleFunc("POST /execute", f.execute)
	mux.HandleFunc("POST /tasks", f.tasks)
	mux.HandleFunc("GET /supported", f.supported)
	mux.HandleFunc("POST /embed", f.embed)
	mux.HandleFunc("GET /defaultmodel", f.defaultModel)
}

type taskManager struct {
	promptService execservice.ExecService
	taskService   execservice.TasksEnvService
	embedService  embedservice.Service
}

// Runs the prompt through the default LLM.
// This endpoint provides basic chat completion optimized for machine-to-machine (M2M) communication.
// Requests are routed ONLY to backends that have the default model available in any shared pool.
// If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.
func (tm *taskManager) execute(w http.ResponseWriter, r *http.Request) {
	req, err := serverops.Decode[execservice.TaskRequest](r) // @request execservice.TaskRequest
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}

	resp, err := tm.promptService.Execute(r.Context(), &req)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}
	_ = serverops.Encode(w, r, http.StatusOK, resp) // @response execservice.TaskResponse
}

type taskExec struct {
	Input     any                         `json:"input"`
	InputType string                      `json:"inputType"`
	Chain     *taskengine.ChainDefinition `json:"chain" oapiinclude:"taskengine.ChainDefinition"`
}

type taskResponse struct {
	Output     any                            `json:"output"`
	OutputType string                         `json:"outputType"`
	State      []taskengine.CapturedStateUnit `json:"state" oapiinclude:"taskengine.CapturedStateUnit"`
}

// Executes dynamic task-chain workflows.
// Task-chains are state-machine workflows (DAGs) with conditional branches,
// external hooks, and captured execution state.
// Requests are routed ONLY to backends that have the requested model available in any shared pool.
// If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.
func (tm *taskManager) tasks(w http.ResponseWriter, r *http.Request) {
	req, err := serverops.Decode[taskExec](r) // @request execapi.taskExec
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}
	inputType, err := taskengine.DataTypeFromString(req.InputType)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}

	// Convert req.Input to the appropriate type based on inputType
	var convertedInput any
	switch inputType {
	case taskengine.DataTypeChatHistory:
		// Convert req.Input (map) to ChatHistory
		data, err := json.Marshal(req.Input)
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
			return
		}
		var chatHistory taskengine.ChatHistory
		if err := json.Unmarshal(data, &chatHistory); err != nil {
			_ = serverops.Error(w, r, fmt.Errorf("failed to convert to ChatHistory: %w", err), serverops.ExecuteOperation)
			return
		}
		convertedInput = chatHistory

	case taskengine.DataTypeOpenAIChat:
		// Convert req.Input (map) to OpenAIChatRequest
		data, err := json.Marshal(req.Input)
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
			return
		}
		var openAIChat taskengine.OpenAIChatRequest
		if err := json.Unmarshal(data, &openAIChat); err != nil {
			_ = serverops.Error(w, r, fmt.Errorf("failed to convert to OpenAIChatRequest: %w", err), serverops.ExecuteOperation)
			return
		}
		convertedInput = openAIChat

	case taskengine.DataTypeOpenAIChatResponse:
		// Convert req.Input (map) to OpenAIChatResponse
		data, err := json.Marshal(req.Input)
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
			return
		}
		var openAIChatResponse taskengine.OpenAIChatResponse
		if err := json.Unmarshal(data, &openAIChatResponse); err != nil {
			_ = serverops.Error(w, r, fmt.Errorf("failed to convert to OpenAIChatResponse: %w", err), serverops.ExecuteOperation)
			return
		}
		convertedInput = openAIChatResponse

	case taskengine.DataTypeSearchResults:
		// Convert req.Input to []SearchResult
		data, err := json.Marshal(req.Input)
		if err != nil {
			_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
			return
		}
		var searchResults []taskengine.SearchResult
		if err := json.Unmarshal(data, &searchResults); err != nil {
			_ = serverops.Error(w, r, fmt.Errorf("failed to convert to []SearchResult: %w", err), serverops.ExecuteOperation)
			return
		}
		convertedInput = searchResults

	case taskengine.DataTypeString:
		// Convert to string (could be direct value or string representation in map)
		switch v := req.Input.(type) {
		case string:
			convertedInput = v
		case map[string]any:
			if strVal, ok := v["value"].(string); ok {
				convertedInput = strVal
			} else {
				// Try to marshal the whole map to JSON string
				if jsonData, err := json.Marshal(req.Input); err == nil {
					convertedInput = string(jsonData)
				} else {
					convertedInput = fmt.Sprintf("%v", req.Input)
				}
			}
		default:
			convertedInput = fmt.Sprintf("%v", req.Input)
		}

	case taskengine.DataTypeBool:
		// Convert to bool
		switch v := req.Input.(type) {
		case bool:
			convertedInput = v
		case string:
			if b, err := strconv.ParseBool(v); err == nil {
				convertedInput = b
			} else {
				convertedInput = v == "true"
			}
		case float64:
			convertedInput = v != 0
		default:
			// Try to convert whatever we have to bool
			strVal := fmt.Sprintf("%v", req.Input)
			b, _ := strconv.ParseBool(strVal)
			convertedInput = b
		}

	case taskengine.DataTypeInt:
		// Convert to int
		switch v := req.Input.(type) {
		case int:
			convertedInput = v
		case float64:
			convertedInput = int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				convertedInput = i
			} else {
				convertedInput = 0
			}
		default:
			if i, err := strconv.Atoi(fmt.Sprintf("%v", req.Input)); err == nil {
				convertedInput = i
			} else {
				convertedInput = 0
			}
		}

	case taskengine.DataTypeFloat:
		// Convert to float64
		switch v := req.Input.(type) {
		case float64:
			convertedInput = v
		case int:
			convertedInput = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				convertedInput = f
			} else {
				convertedInput = 0.0
			}
		default:
			if f, err := strconv.ParseFloat(fmt.Sprintf("%v", req.Input), 64); err == nil {
				convertedInput = f
			} else {
				convertedInput = 0.0
			}
		}

	case taskengine.DataTypeJSON:
		// For JSON type, we can keep it as map or slice
	default:
		// For DataTypeAny and any other unrecognized types, use the raw input
		convertedInput = req.Input
	}

	resp, outputType, capturedStateUnits, err := tm.taskService.Execute(r.Context(), req.Chain, convertedInput, inputType)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}
	var response taskResponse
	response.Output = resp
	response.OutputType = outputType.String()
	response.State = capturedStateUnits
	_ = serverops.Encode(w, r, http.StatusOK, response) // @response execapi.taskResponse
}

// Lists available task-chain hook types.
// Returns all registered external action types that can be used in task-chain hooks.
func (tm *taskManager) supported(w http.ResponseWriter, r *http.Request) {
	resp, err := tm.taskService.Supports(r.Context())
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, resp) // @response []string
}

type EmbedRequest struct {
	Text string `json:"text" example:"Hello, world!"`
}

type EmbedResponse struct {
	Vector []float64 `json:"vector" example:"[0.1, 0.2, 0.3, ...]"`
}

// Generates vector embeddings for text.
// Uses the system's default embedding model configured at startup.
// Requests are routed ONLY to backends that have the default model available in any shared pool.
// If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.
func (tm *taskManager) embed(w http.ResponseWriter, r *http.Request) {
	req, err := serverops.Decode[EmbedRequest](r) // @request execapi.EmbedRequest
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	vector, err := tm.embedService.Embed(r.Context(), req.Text)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("embedding failed: %w", err), serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, EmbedResponse{Vector: vector}) // @response execapi.EmbedResponse
}

type DefaultModelResponse struct {
	ModelName string `json:"modelName" example:"mistral:latest"`
}

// Returns the default model configured during system initialization.
func (tm *taskManager) defaultModel(w http.ResponseWriter, r *http.Request) {
	modelName, err := tm.embedService.DefaultModelName(r.Context())
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("failed to get default model: %w", err), serverops.GetOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, DefaultModelResponse{ModelName: modelName}) // @response execapi.DefaultModelResponse
}
