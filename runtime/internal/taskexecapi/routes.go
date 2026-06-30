package taskexecapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

type Defaults = stateservice.RuntimeDefaults

func AddRoutes(mux *http.ServeMux, agent agentservice.Agent, auth middleware.AuthZReader, stateService stateservice.Service, defaults Defaults) {
	h := &handler{agent: agent, auth: auth, stateService: stateService, defaults: defaults}
	mux.HandleFunc("POST /tasks", h.execute)
}

type handler struct {
	agent        agentservice.Agent
	auth         middleware.AuthZReader
	stateService stateservice.Service
	defaults     Defaults
}

type executeTaskRequest struct {
	Input        any                            `json:"input"`
	InputType    string                         `json:"inputType"`
	Chain        taskengine.TaskChainDefinition `json:"chain" openapi_include_type:"taskengine.TaskChainDefinition"`
	TemplateVars map[string]string              `json:"templateVars,omitempty"`
}

type executeTaskResponse struct {
	RequestID  string                         `json:"requestId,omitempty"`
	Output     any                            `json:"output"`
	OutputType string                         `json:"outputType"`
	State      []taskengine.CapturedStateUnit `json:"state" openapi_include_type:"taskengine.CapturedStateUnit"`
	StopReason agentservice.StopReason        `json:"stopReason,omitempty"`
}

func (h *handler) authorize(ctx context.Context) error {
	if h.auth == nil {
		return nil
	}
	_, err := h.auth.GetIdentity(ctx)
	return err
}

func (h *handler) execute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(ctx); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.agent == nil {
		_ = apiframework.Error(w, r, fmt.Errorf("task agent is not configured"), apiframework.ServerOperation)
		return
	}

	req, err := apiframework.Decode[executeTaskRequest](r) // @request taskexecapi.executeTaskRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	if len(req.Chain.Tasks) == 0 {
		_ = apiframework.Error(w, r, apiframework.BadRequest("chain must contain at least one task"), apiframework.CreateOperation)
		return
	}

	inputType := taskengine.DataTypeAny
	if strings.TrimSpace(req.InputType) != "" {
		inputType, err = taskengine.DataTypeFromString(req.InputType)
		if err != nil {
			_ = apiframework.Error(w, r, apiframework.BadRequest(err.Error()), apiframework.CreateOperation)
			return
		}
	}

	resp, err := h.agent.Prompt(ctx, agentservice.PromptRequest{
		InputValue:   req.Input,
		InputType:    inputType,
		Chain:        &req.Chain,
		TemplateVars: h.templateVars(ctx, req.TemplateVars),
	})
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	outputType := resp.OutputType.String()
	out := executeTaskResponse{
		RequestID:  requestID(ctx),
		Output:     resp.Output,
		OutputType: outputType,
		State:      resp.Steps,
		StopReason: resp.StopReason,
	}
	_ = apiframework.Encode(w, r, http.StatusOK, out) // @response taskexecapi.executeTaskResponse
}

func (h *handler) templateVars(ctx context.Context, raw map[string]string) map[string]string {
	defaults := stateservice.ResolveRuntimeDefaults(ctx, h.stateService, h.defaults)
	vars := make(map[string]string, len(raw)+5)
	for k, v := range raw {
		vars[k] = v
	}
	for k, v := range defaults.TemplateVars() {
		vars[k] = v
	}
	return vars
}

func requestID(ctx context.Context) string {
	reqID, _ := ctx.Value(libtracker.ContextKeyRequestID).(string)
	return reqID
}
