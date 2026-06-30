package compatapi_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/internal/compatapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/taskengine"
)

func TestOllamaTagsListsContenoxModels(t *testing.T) {
	deps := compatapi.CompatDeps{
		StateService: &stubStateService{states: []statetype.BackendRuntimeState{{
			PulledModels: []statetype.ModelPullStatus{{
				Model:      "known-model",
				ModifiedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				Size:       123,
				Digest:     "sha256:test",
				CanChat:    true,
				CanPrompt:  true,
			}},
		}}},
		Defaults: stateservice.RuntimeDefaults{Model: "contenox-default"},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
			Size  int64  `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Models) < 3 {
		t.Fatalf("expected default alias, default model, and observed model; got %#v", resp.Models)
	}
	if resp.Models[0].Name != "default" {
		t.Fatalf("expected default alias first, got %q", resp.Models[0].Name)
	}
	seen := map[string]bool{}
	for _, model := range resp.Models {
		seen[model.Name] = true
	}
	for _, name := range []string{"default", "contenox-default", "known-model"} {
		if !seen[name] {
			t.Fatalf("expected /api/tags to include %q; got %#v", name, seen)
		}
	}
}

func TestOllamaTagsFiltersEmbeddingOnlyModels(t *testing.T) {
	deps := compatapi.CompatDeps{
		StateService: &stubStateService{states: []statetype.BackendRuntimeState{{
			PulledModels: []statetype.ModelPullStatus{
				{Model: "chat-model", CanChat: true, CanPrompt: true},
				{Model: "embedding-model", CanEmbed: true},
			},
		}}},
		Defaults: stateservice.RuntimeDefaults{Model: "chat-model"},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, model := range resp.Models {
		seen[model.Name] = true
	}
	if !seen["default"] || !seen["chat-model"] {
		t.Fatalf("expected chat models in /api/tags, got %#v", seen)
	}
	if seen["embedding-model"] {
		t.Fatalf("did not expect embedding-only model in /api/tags: %#v", seen)
	}
}

func TestOllamaTagsFiltersToDefaultProvider(t *testing.T) {
	deps := compatapi.CompatDeps{
		StateService: &stubStateService{states: []statetype.BackendRuntimeState{
			{
				Backend: runtimetypes.Backend{Type: "openai", Name: "openai"},
				PulledModels: []statetype.ModelPullStatus{
					{Model: "gpt-5", CanChat: true, CanPrompt: true},
				},
			},
			{
				Backend: runtimetypes.Backend{Type: "vertex-google", Name: "vertex-global"},
				PulledModels: []statetype.ModelPullStatus{
					{Model: "gemini-3.1-pro-preview", CanChat: true, CanPrompt: true},
				},
			},
		}},
		Defaults: stateservice.RuntimeDefaults{
			Provider: "vertex-google",
			Model:    "gemini-3.1-pro-preview",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, model := range resp.Models {
		seen[model.Name] = true
	}
	for _, name := range []string{"default", "gemini-3.1-pro-preview"} {
		if !seen[name] {
			t.Fatalf("expected /api/tags to include %q; got %#v", name, seen)
		}
	}
	if seen["gpt-5"] {
		t.Fatalf("did not expect cross-provider model in /api/tags: %#v", seen)
	}
}

func TestOllamaShowReportsCapabilitiesForZed(t *testing.T) {
	modifiedAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	deps := compatapi.CompatDeps{
		StateService: &stubStateService{states: []statetype.BackendRuntimeState{{
			PulledModels: []statetype.ModelPullStatus{{
				Model:         "zed-model",
				ModifiedAt:    modifiedAt,
				ContextLength: 8192,
				Details: statetype.ModelDetails{
					Format:            "gguf",
					Family:            "llama",
					ParameterSize:     "7B",
					QuantizationLevel: "Q4_K_M",
				},
				CanChat:   true,
				CanPrompt: true,
				CanStream: true,
				CanThink:  true,
			}},
		}}},
		Defaults: stateservice.RuntimeDefaults{Model: "zed-model"},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	req := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(`{"model":"zed-model"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Details struct {
			Family string `json:"family"`
		} `json:"details"`
		ModelInfo    map[string]any `json:"model_info"`
		Capabilities []string       `json:"capabilities"`
		Parameters   string         `json:"parameters"`
		ModifiedAt   string         `json:"modified_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Details.Family != "llama" {
		t.Fatalf("expected details family llama, got %q", resp.Details.Family)
	}
	if resp.ModelInfo["general.architecture"] != "contenox" {
		t.Fatalf("expected contenox architecture, got %#v", resp.ModelInfo)
	}
	if got, ok := resp.ModelInfo["contenox.context_length"].(float64); !ok || got != 8192 {
		t.Fatalf("expected contenox.context_length 8192, got %#v", resp.ModelInfo["contenox.context_length"])
	}
	for _, cap := range []string{"completion", "thinking"} {
		if !stringSliceContains(resp.Capabilities, cap) {
			t.Fatalf("expected capability %q in %#v", cap, resp.Capabilities)
		}
	}
	if resp.Parameters != "num_ctx 8192" {
		t.Fatalf("expected num_ctx parameter, got %q", resp.Parameters)
	}
	if resp.ModifiedAt == "" {
		t.Fatal("expected modified_at")
	}
}

func TestOllamaChatStreamsNDJSONAndUsesKnownModel(t *testing.T) {
	agent := &stubAgent{reply: "hello STOP tail"}
	deps := compatapi.CompatDeps{
		Agent:        agent,
		Chains:       &stubChains{chain: modelAndMaxTokensTemplateChain("chat-chain")},
		StateService: &stubStateService{states: observedState("known-model")},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "chat-chain",
			Model:    "contenox-default",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	body := `{"model":"known-model","messages":[{"role":"user","content":"hi"}],"options":{"temperature":0.2,"num_predict":12,"stop":[" STOP"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Fatalf("expected NDJSON content type, got %q", ct)
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(rr.Body.Bytes()))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %v", len(lines), lines)
	}
	var chunk struct {
		Model   string `json:"model"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &chunk); err != nil {
		t.Fatalf("decode first chunk: %v", err)
	}
	if chunk.Done || chunk.Model != "known-model" || chunk.Message.Content != "hello" {
		t.Fatalf("unexpected first chunk: %#v", chunk)
	}
	var done struct {
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &done); err != nil {
		t.Fatalf("decode final chunk: %v", err)
	}
	if !done.Done || done.DoneReason != "stop" {
		t.Fatalf("unexpected final chunk: %#v", done)
	}
	if got := agent.lastReq.TemplateVars["model"]; got != "known-model" {
		t.Fatalf("expected known observed model to pass through, got %q", got)
	}
	if got := agent.lastReq.TemplateVars["max_tokens"]; got != "12" {
		t.Fatalf("expected num_predict template var 12, got %q", got)
	}
	cfg := agent.lastReq.Chain.Tasks[0].ExecuteConfig
	if cfg.MaxTokens != nil {
		t.Fatalf("expected max tokens to remain macro-owned before MacroEnv, got %#v", cfg.MaxTokens)
	}
	if cfg.MaxTokensTemplate != "{{var:max_tokens|256}}" {
		t.Fatalf("expected max tokens macro to remain on chain, got %q", cfg.MaxTokensTemplate)
	}
	if cfg.Temperature == nil || *cfg.Temperature < 0.199 || *cfg.Temperature > 0.201 {
		t.Fatalf("expected patched temperature 0.2, got %#v", cfg.Temperature)
	}
}

func TestOllamaChatFallsBackWhenModelBelongsToDifferentProvider(t *testing.T) {
	agent := &stubAgent{reply: "hello"}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: &stubChains{chain: modelAndMaxTokensTemplateChain("chat-chain")},
		StateService: &stubStateService{states: []statetype.BackendRuntimeState{
			{
				Backend: runtimetypes.Backend{Type: "openai", Name: "openai"},
				PulledModels: []statetype.ModelPullStatus{
					{Model: "gpt-5", CanChat: true, CanPrompt: true},
				},
			},
			{
				Backend: runtimetypes.Backend{Type: "vertex-google", Name: "vertex-global"},
				PulledModels: []statetype.ModelPullStatus{
					{Model: "gemini-3.1-pro-preview", CanChat: true, CanPrompt: true},
				},
			},
		}},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "chat-chain",
			Provider: "vertex-google",
			Model:    "gemini-3.1-pro-preview",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	body := `{"model":"gpt-5","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := agent.lastReq.TemplateVars["model"]; got != "gemini-3.1-pro-preview" {
		t.Fatalf("expected cross-provider model to fall back to default, got %q", got)
	}
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func TestOllamaGenerateNonStreamingUsesFIMInput(t *testing.T) {
	agent := &stubAgent{reply: "completion"}
	deps := compatapi.CompatDeps{
		Agent:  agent,
		Chains: &stubChains{chain: modelTemplateChain("fim-chain")},
		Defaults: stateservice.RuntimeDefaults{
			FIMChainRef: "fim-chain",
			Model:       "test-model",
		},
	}

	mux := http.NewServeMux()
	compatapi.AddOllamaRoutes(mux, deps)

	body := `{"model":"default","prompt":"PREFIX","suffix":"SUFFIX","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Model    string `json:"model"`
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Model != "test-model" || resp.Response != "completion" || !resp.Done {
		t.Fatalf("unexpected generate response: %#v", resp)
	}
	hist, ok := agent.lastReq.InputValue.(taskengine.ChatHistory)
	if !ok || len(hist.Messages) != 1 {
		t.Fatalf("expected chat history input, got %#v", agent.lastReq.InputValue)
	}
	want := "<fim_prefix>PREFIX<fim_suffix>SUFFIX<fim_middle>"
	if hist.Messages[0].Content != want {
		t.Fatalf("unexpected FIM sentinel:\n want %q\n  got %q", want, hist.Messages[0].Content)
	}
}
