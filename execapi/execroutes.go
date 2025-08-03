package execapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	serverops "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/execservice"
	"github.com/contenox/runtime/taskengine"
)

func AddExecRoutes(mux *http.ServeMux, promptService execservice.ExecService, taskService execservice.TasksEnvService) {
	f := &taskManager{
		promptService: promptService,
		taskService:   taskService,
	}
	mux.HandleFunc("POST /execute", f.execute)
	mux.HandleFunc("POST /tasks", f.tasks)
	mux.HandleFunc("GET /supported", f.supported)
}

type taskManager struct {
	promptService execservice.ExecService
	taskService   execservice.TasksEnvService
}

func (tm *taskManager) execute(w http.ResponseWriter, r *http.Request) {
	req, err := serverops.Decode[execservice.TaskRequest](r)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}

	resp, err := tm.promptService.Execute(r.Context(), &req)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}
	_ = serverops.Encode(w, r, http.StatusOK, resp)
}

type taskExec struct {
	Input     any                         `json:"input"`
	InputType string                      `json:"inputType"`
	Chain     *taskengine.ChainDefinition `json:"chain"`
}

func (tm *taskManager) tasks(w http.ResponseWriter, r *http.Request) {
	req, err := serverops.Decode[taskExec](r)
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

	resp, capturedStateUnits, err := tm.taskService.Execute(r.Context(), req.Chain, convertedInput, inputType)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ExecuteOperation)
		return
	}
	response := map[string]any{
		"response": resp,
		"state":    capturedStateUnits,
	}
	_ = serverops.Encode(w, r, http.StatusOK, response)
}

func (tm *taskManager) supported(w http.ResponseWriter, r *http.Request) {
	resp, err := tm.taskService.Supports(r.Context())
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, resp)
}
