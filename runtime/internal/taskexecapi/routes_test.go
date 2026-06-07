package taskexecapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

type mockAgent struct {
	req agentservice.PromptRequest
}

func (m *mockAgent) Capabilities(context.Context) (*agentservice.AgentCapabilities, error) {
	return nil, nil
}

func (m *mockAgent) SessionNew(context.Context, string) (string, error) {
	return "", nil
}

func (m *mockAgent) SessionList(context.Context) ([]*agentservice.SessionInfo, error) {
	return nil, nil
}

func (m *mockAgent) SessionLoad(context.Context, string) (string, []taskengine.Message, error) {
	return "", nil, nil
}

func (m *mockAgent) SessionEnsureDefault(context.Context) (string, error) {
	return "", nil
}

func (m *mockAgent) Prompt(_ context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	m.req = req
	return &agentservice.PromptResponse{
		Output:     "ok",
		OutputType: taskengine.DataTypeString,
		Steps: []taskengine.CapturedStateUnit{
			{TaskID: "one", TaskHandler: string(taskengine.HandleNoop)},
		},
		StopReason: agentservice.StopEndTurn,
	}, nil
}

func TestUnit_ExecuteTask_PostsChainToAgent(t *testing.T) {
	agent := &mockAgent{}
	mux := http.NewServeMux()
	AddRoutes(mux, agent, nil, Defaults{Model: "llama3", Provider: "ollama", Think: "off"})
	handler := apiframework.RequestIDMiddleware(mux)

	body := `{
		"input": "hello",
		"inputType": "string",
		"templateVars": {"custom": "value"},
		"chain": {
			"id": "test-chain",
			"tasks": [{
				"id": "one",
				"handler": "noop",
				"transition": {"branches": [{"operator": "default", "goto": "end"}]}
			}]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if agent.req.InputValue != "hello" {
		t.Fatalf("input = %#v, want hello", agent.req.InputValue)
	}
	if agent.req.InputType != taskengine.DataTypeString {
		t.Fatalf("input type = %v, want string", agent.req.InputType)
	}
	if agent.req.Chain == nil || agent.req.Chain.ID != "test-chain" {
		t.Fatalf("chain = %#v", agent.req.Chain)
	}
	if agent.req.TemplateVars["custom"] != "value" ||
		agent.req.TemplateVars["model"] != "llama3" ||
		agent.req.TemplateVars["provider"] != "ollama" ||
		agent.req.TemplateVars["think"] != "off" {
		t.Fatalf("template vars = %#v", agent.req.TemplateVars)
	}

	var got executeTaskResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RequestID != "req-test" || got.Output != "ok" || got.OutputType != "string" || len(got.State) != 1 {
		t.Fatalf("response = %#v", got)
	}
}

func TestUnit_ExecuteTask_RejectsEmptyChain(t *testing.T) {
	agent := &mockAgent{}
	mux := http.NewServeMux()
	AddRoutes(mux, agent, nil, Defaults{})

	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"chain":{"id":"empty"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if agent.req.Chain != nil {
		t.Fatalf("agent should not be called, got %#v", agent.req)
	}
}
