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
	"github.com/contenox/runtime/runtime/taskengine"
)

func TestFIMCompletions_Streaming(t *testing.T) {
	deps := compatapi.CompatDeps{
		Agent:              &stubAgent{reply: "name string"},
		Chains:             &stubChains{},
		DefaultFIMChainRef: "fim-chain",
		DefaultModel:       "test-model",
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","prompt":"func hello(","suffix":") {}","max_tokens":64,"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/fim/completions", strings.NewReader(body))
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

	// Expect: content chunk, stop+usage chunk, [DONE]
	if len(dataLines) != 3 {
		t.Fatalf("expected 3 data lines, got %d: %v", len(dataLines), dataLines)
	}
	if dataLines[2] != "[DONE]" {
		t.Errorf("expected last line [DONE], got %q", dataLines[2])
	}

	var contentChunk struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(dataLines[0]), &contentChunk); err != nil {
		t.Fatalf("unmarshal content chunk: %v", err)
	}
	if contentChunk.Choices[0].Text != "name string" {
		t.Errorf("expected 'name string', got %q", contentChunk.Choices[0].Text)
	}
}

func TestFIMCompletions_NonStreaming(t *testing.T) {
	deps := compatapi.CompatDeps{
		Agent:              &stubAgent{reply: "name string"},
		Chains:             &stubChains{},
		DefaultFIMChainRef: "fim-chain",
		DefaultModel:       "test-model",
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","prompt":"func hello(","suffix":") {}","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/fim/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Object  string `json:"object"`
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "text_completion" {
		t.Errorf("expected object 'text_completion', got %q", resp.Object)
	}
	if resp.Choices[0].Text != "name string" {
		t.Errorf("expected 'name string', got %q", resp.Choices[0].Text)
	}
}

func TestFIMCompletions_LegacyAlias(t *testing.T) {
	deps := compatapi.CompatDeps{
		Agent:              &stubAgent{reply: "result"},
		Chains:             &stubChains{},
		DefaultFIMChainRef: "fim-chain",
		DefaultModel:       "test-model",
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"prompt":"hello ","suffix":"world","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from legacy alias, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFIMCompletions_SentinelFormat(t *testing.T) {
	capture := &sentinelCapture{}
	deps := compatapi.CompatDeps{
		Agent:              capture,
		Chains:             &stubChains{},
		DefaultFIMChainRef: "fim-chain",
		DefaultModel:       "test-model",
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"prompt":"PREFIX","suffix":"SUFFIX","stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/fim/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	want := "<fim_prefix>PREFIX<fim_suffix>SUFFIX<fim_middle>"
	if capture.lastContent != want {
		t.Errorf("sentinel mismatch:\n want %q\n  got %q", want, capture.lastContent)
	}
}

func TestFIMCompletions_OverridesSessionAndStopStrings(t *testing.T) {
	agent := &stubAgent{reply: "name string STOP tail"}
	deps := compatapi.CompatDeps{
		Agent:              agent,
		Chains:             &stubChains{chain: modelAndMaxTokensTemplateChain("fim-chain")},
		DefaultFIMChainRef: "fim-chain",
		DefaultModel:       "test-model",
	}

	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, deps)

	body := `{"model":"default","prompt":"func hello(","suffix":") {}","temperature":0.1,"max_tokens":32,"stop":[" STOP"],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/fim/completions", strings.NewReader(body))
	req.Header.Set("X-Session-ID", "fim-session")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if agent.lastReq.SessionID != "fim-session" {
		t.Fatalf("expected PromptRequest session, got %q", agent.lastReq.SessionID)
	}
	if got := agent.lastReq.TemplateVars["max_tokens"]; got != "32" {
		t.Fatalf("expected request max_tokens template var 32, got %q", got)
	}
	cfg := agent.lastReq.Chain.Tasks[0].ExecuteConfig
	if cfg.MaxTokens != nil {
		t.Fatalf("expected max tokens to remain macro-owned before MacroEnv, got %#v", cfg.MaxTokens)
	}
	if cfg.MaxTokensTemplate != "{{var:max_tokens|256}}" {
		t.Fatalf("expected max tokens macro to remain on chain, got %q", cfg.MaxTokensTemplate)
	}
	if cfg.Temperature == nil || *cfg.Temperature < 0.099 || *cfg.Temperature > 0.101 {
		t.Fatalf("expected patched temperature 0.1, got %#v", cfg.Temperature)
	}

	var resp struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := resp.Choices[0].Text; got != "name string" {
		t.Fatalf("expected stop string to trim completion, got %q", got)
	}
}

// sentinelCapture captures the first user message content from each Prompt call.
type sentinelCapture struct {
	lastContent string
}

func (s *sentinelCapture) Capabilities(_ context.Context) (*agentservice.AgentCapabilities, error) {
	return &agentservice.AgentCapabilities{}, nil
}
func (s *sentinelCapture) SessionNew(_ context.Context, _ string) (string, error) { return "", nil }
func (s *sentinelCapture) SessionList(_ context.Context) ([]*agentservice.SessionInfo, error) {
	return nil, nil
}
func (s *sentinelCapture) SessionLoad(_ context.Context, _ string) (string, []taskengine.Message, error) {
	return "", nil, nil
}
func (s *sentinelCapture) SessionEnsureDefault(_ context.Context) (string, error) { return "", nil }
func (s *sentinelCapture) Prompt(_ context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	if hist, ok := req.InputValue.(taskengine.ChatHistory); ok && len(hist.Messages) > 0 {
		s.lastContent = hist.Messages[0].Content
	}
	hist := taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "assistant", Content: ""}}}
	return &agentservice.PromptResponse{Output: hist, OutputType: taskengine.DataTypeChatHistory}, nil
}
