package compatapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/internal/compatapi"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/taskengine"
)

func TestChatCompletions_Streaming(t *testing.T) {
	agent := &stubAgent{reply: "pong"}
	chains := &stubChains{}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: chains,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","messages":[{"role":"user","content":"ping"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	var dataLines []string
	scanner := bufio.NewScanner(bytes.NewReader(rr.Body.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}

	// Expect: role-init chunk, content chunk, stop+usage chunk, [DONE]
	if len(dataLines) != 4 {
		t.Fatalf("expected 4 data lines, got %d: %v", len(dataLines), dataLines)
	}
	if dataLines[3] != "[DONE]" {
		t.Errorf("expected last data line [DONE], got %q", dataLines[3])
	}

	// Content chunk should contain the reply.
	var contentChunk struct {
		Choices []struct {
			Delta struct{ Content string } `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(dataLines[1]), &contentChunk); err != nil {
		t.Fatalf("unmarshal content chunk: %v", err)
	}
	if contentChunk.Choices[0].Delta.Content != "pong" {
		t.Errorf("expected reply 'pong', got %q", contentChunk.Choices[0].Delta.Content)
	}
}

func TestChatCompletions_NonStreaming(t *testing.T) {
	agent := &stubAgent{reply: "pong"}
	chains := &stubChains{}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: chains,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","messages":[{"role":"user","content":"ping"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}
	if resp.Choices[0].Message.Content != "pong" {
		t.Errorf("expected reply 'pong', got %q", resp.Choices[0].Message.Content)
	}
}

func TestChatCompletions_NamedChain(t *testing.T) {
	agent := &stubAgent{reply: "ok"}
	chains := &stubChains{}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: chains,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "default-chain",
			Model:    "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/my-chain/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if chains.lastRef != "my-chain" {
		t.Errorf("expected chain 'my-chain', got %q", chains.lastRef)
	}
}

func TestChatCompletions_OverridesSessionAndUnknownModelFallsBack(t *testing.T) {
	temp := float32(0.9)
	agent := &stubAgent{reply: "pong"}
	chains := &stubChains{chain: &taskengine.TaskChainDefinition{
		ID: "test-chain",
		Tasks: []taskengine.TaskDefinition{{
			ID: "llm",
			ExecuteConfig: &taskengine.LLMExecutionConfig{
				Model:             "{{var:model}}",
				Temperature:       &temp,
				MaxTokensTemplate: "{{var:max_tokens|256}}",
			},
		}},
	}}
	deps := compatapi.CompatDeps{
		Agent:        agent,
		Chains:       chains,
		StateService: &stubStateService{states: observedState("contenox-known")},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "contenox-default",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"ide-only-model","messages":[{"role":"user","content":"ping"}],"temperature":0.25,"max_completion_tokens":64}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Session-ID", "session-123")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-Session-ID") != "session-123" {
		t.Fatalf("expected session header echo, got %q", rr.Header().Get("X-Session-ID"))
	}
	if agent.lastReq.SessionID != "session-123" {
		t.Fatalf("expected PromptRequest session, got %q", agent.lastReq.SessionID)
	}
	if got := agent.lastReq.TemplateVars["model"]; got != "contenox-default" {
		t.Fatalf("expected unknown client model to fall back to default, got %q", got)
	}
	if got := agent.lastReq.TemplateVars["max_tokens"]; got != "64" {
		t.Fatalf("expected request max_tokens template var 64, got %q", got)
	}
	cfg := agent.lastReq.Chain.Tasks[0].ExecuteConfig
	if cfg == chains.chain.Tasks[0].ExecuteConfig {
		t.Fatal("expected chain execute_config to be cloned before patching")
	}
	if cfg.MaxTokens != nil {
		t.Fatalf("expected max tokens to remain macro-owned before MacroEnv, got %#v", cfg.MaxTokens)
	}
	if cfg.MaxTokensTemplate != "{{var:max_tokens|256}}" {
		t.Fatalf("expected max tokens macro to remain on chain, got %q", cfg.MaxTokensTemplate)
	}
	if cfg.Temperature == nil || *cfg.Temperature < 0.249 || *cfg.Temperature > 0.251 {
		t.Fatalf("expected patched temperature 0.25, got %#v", cfg.Temperature)
	}
	if chains.chain.Tasks[0].ExecuteConfig.MaxTokens != nil {
		t.Fatal("original chain max tokens was mutated")
	}

	var resp struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Model != "contenox-default" {
		t.Fatalf("expected response model default fallback, got %q", resp.Model)
	}
}

func TestChatCompletions_KnownObservedModelPassesThrough(t *testing.T) {
	agent := &stubAgent{reply: "pong"}
	deps := compatapi.CompatDeps{
		Agent:        agent,
		Chains:       &stubChains{chain: modelTemplateChain("test-chain")},
		StateService: &stubStateService{states: observedState("known-model")},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "contenox-default",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"known-model","messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := agent.lastReq.TemplateVars["model"]; got != "known-model" {
		t.Fatalf("expected known observed model to pass through, got %q", got)
	}
}

func TestChatCompletions_UsesCurrentAltDefaults(t *testing.T) {
	agent := &stubAgent{reply: "pong"}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: &stubChains{chain: altModelTemplateChain("test-chain")},
		StateService: &stubStateService{config: stateservice.CLIConfigSnapshot{
			DefaultModel:       "current-model",
			DefaultProvider:    "current-provider",
			DefaultAltModel:    "current-alt-model",
			DefaultAltProvider: "current-alt-provider",
		}},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef:    "test-chain",
			Model:       "stale-model",
			Provider:    "stale-provider",
			AltModel:    "stale-alt-model",
			AltProvider: "stale-alt-provider",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := agent.lastReq.TemplateVars["model"]; got != "current-model" {
		t.Fatalf("expected current model, got %q", got)
	}
	if got := agent.lastReq.TemplateVars["provider"]; got != "current-provider" {
		t.Fatalf("expected current provider, got %q", got)
	}
	if got := agent.lastReq.TemplateVars["alt_model"]; got != "current-alt-model" {
		t.Fatalf("expected current alt model, got %q", got)
	}
	if got := agent.lastReq.TemplateVars["alt_provider"]; got != "current-alt-provider" {
		t.Fatalf("expected current alt provider, got %q", got)
	}
}

func TestChatCompletions_MissingMessagesIsBadRequest(t *testing.T) {
	deps := compatapi.CompatDeps{
		Agent:  &stubAgent{reply: "pong"},
		Chains: &stubChains{},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(`{"model":"default"}`))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRootRoutes_TokenProtectsMutatingCompatAndListsDefaultModels(t *testing.T) {
	deps := compatapi.CompatDeps{
		Agent:        &stubAgent{reply: "pong"},
		Chains:       &stubChains{chain: modelTemplateChain("test-chain")},
		StateService: &stubStateService{states: observedState("observed-model")},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "contenox-default",
		},
		Token: "secret-token",
	}

	mux := http.NewServeMux()
	compatapi.AddRootRoutes(mux, deps)

	body := `{"model":"default","messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated root POST to be 401, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected authenticated root POST to be 200, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected unauthenticated model list to be 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var models struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&models); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	ids := map[string]bool{}
	for _, model := range models.Data {
		ids[model.ID] = true
		if model.Created == 0 {
			t.Fatalf("expected non-zero created timestamp for %q", model.ID)
		}
	}
	for _, id := range []string{"default", "contenox-default", "observed-model"} {
		if !ids[id] {
			t.Fatalf("expected model list to include %q; got %#v", id, ids)
		}
	}
}

func TestRootRoutes_GETChatCompletionsIsMethodNotAllowed(t *testing.T) {
	mux := http.NewServeMux()
	compatapi.AddRootRoutes(mux, compatapi.CompatDeps{
		Agent:  &stubAgent{reply: "pong"},
		Chains: &stubChains{},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "test-chain",
			Model:    "test-model",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// stubAgent is a minimal agentservice.Agent for testing.
type stubAgent struct {
	reply   string
	lastReq agentservice.PromptRequest
}

func (s *stubAgent) Capabilities(_ context.Context) (*agentservice.AgentCapabilities, error) {
	return &agentservice.AgentCapabilities{}, nil
}
func (s *stubAgent) SessionNew(_ context.Context, _ string) (string, error) { return "sess", nil }
func (s *stubAgent) SessionList(_ context.Context) ([]*agentservice.SessionInfo, error) {
	return nil, nil
}
func (s *stubAgent) SessionLoad(_ context.Context, _ string) (string, []taskengine.Message, error) {
	return "", nil, nil
}
func (s *stubAgent) SessionResume(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubAgent) SessionDelete(_ context.Context, _ string) error           { return nil }
func (s *stubAgent) SessionEnsureDefault(_ context.Context) (string, error)    { return "sess", nil }
func (s *stubAgent) Prompt(_ context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	s.lastReq = req
	hist := taskengine.ChatHistory{
		Messages: []taskengine.Message{
			{Role: "assistant", Content: s.reply},
		},
	}
	return &agentservice.PromptResponse{
		Output:     hist,
		OutputType: taskengine.DataTypeChatHistory,
		StopReason: agentservice.StopEndTurn,
	}, nil
}

// stubChains is a minimal taskchainservice.Service for testing.
type stubChains struct {
	lastRef string
	chain   *taskengine.TaskChainDefinition
}

func (s *stubChains) Get(_ context.Context, ref string) (*taskengine.TaskChainDefinition, error) {
	s.lastRef = ref
	if s.chain != nil {
		return s.chain, nil
	}
	return &taskengine.TaskChainDefinition{ID: ref}, nil
}
func (s *stubChains) List(_ context.Context) ([]string, error) { return nil, nil }
func (s *stubChains) CreateAtPath(_ context.Context, _ string, _ *taskengine.TaskChainDefinition) error {
	return nil
}
func (s *stubChains) UpdateAtPath(_ context.Context, _ string, _ *taskengine.TaskChainDefinition) error {
	return nil
}
func (s *stubChains) DeleteByPath(_ context.Context, _ string) error { return nil }

type stubStateService struct {
	states []statetype.BackendRuntimeState
	config stateservice.CLIConfigSnapshot
}

func (s *stubStateService) Get(_ context.Context) ([]statetype.BackendRuntimeState, error) {
	return s.states, nil
}
func (s *stubStateService) CLIConfig(_ context.Context) (stateservice.CLIConfigSnapshot, error) {
	return s.config, nil
}
func (s *stubStateService) SetupStatus(_ context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}
func (s *stubStateService) Refresh(_ context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}
func (s *stubStateService) SetCLIConfig(_ context.Context, _ stateservice.CLIConfigPatch) (stateservice.CLIConfigSnapshot, error) {
	return stateservice.CLIConfigSnapshot{}, nil
}

func observedState(models ...string) []statetype.BackendRuntimeState {
	pulled := make([]statetype.ModelPullStatus, 0, len(models))
	for _, model := range models {
		pulled = append(pulled, statetype.ModelPullStatus{Model: model, CanChat: true, CanPrompt: true})
	}
	return []statetype.BackendRuntimeState{{PulledModels: pulled}}
}

func modelTemplateChain(id string) *taskengine.TaskChainDefinition {
	return &taskengine.TaskChainDefinition{
		ID: id,
		Tasks: []taskengine.TaskDefinition{{
			ID:            "llm",
			ExecuteConfig: &taskengine.LLMExecutionConfig{Model: "{{var:model}}"},
		}},
	}
}

func altModelTemplateChain(id string) *taskengine.TaskChainDefinition {
	return &taskengine.TaskChainDefinition{
		ID: id,
		Tasks: []taskengine.TaskDefinition{{
			ID: "llm",
			ExecuteConfig: &taskengine.LLMExecutionConfig{
				Model:    "{{var:alt_model|var:model}}",
				Provider: "{{var:alt_provider|var:provider}}",
			},
		}},
	}
}

func modelAndMaxTokensTemplateChain(id string) *taskengine.TaskChainDefinition {
	return &taskengine.TaskChainDefinition{
		ID: id,
		Tasks: []taskengine.TaskDefinition{{
			ID: "llm",
			ExecuteConfig: &taskengine.LLMExecutionConfig{
				Model:             "{{var:model}}",
				MaxTokensTemplate: "{{var:max_tokens|256}}",
			},
		}},
	}
}
