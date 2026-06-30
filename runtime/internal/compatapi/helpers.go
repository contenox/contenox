package compatapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/taskengine"
)

// chainUsesVar reports whether any task in the chain references {{var:varName}}
// in its execute_config model, provider, think, max_tokens, prompt_template, or system_instruction.
func chainUsesVar(chain *taskengine.TaskChainDefinition, varName string) bool {
	if chain == nil {
		return false
	}
	for i := range chain.Tasks {
		t := &chain.Tasks[i]
		if containsVarMacro(t.SystemInstruction, varName) ||
			containsVarMacro(t.PromptTemplate, varName) {
			return true
		}
		if t.ExecuteConfig == nil {
			continue
		}
		if containsVarMacro(t.ExecuteConfig.Model, varName) ||
			containsVarMacro(t.ExecuteConfig.Provider, varName) ||
			containsVarMacro(t.ExecuteConfig.Think, varName) ||
			containsVarMacro(t.ExecuteConfig.MaxTokensTemplate, varName) {
			return true
		}
	}
	return false
}

func containsVarMacro(s, varName string) bool {
	return strings.Contains(s, "{{var:"+varName+"}}") ||
		strings.Contains(s, "{{var:"+varName+"|") ||
		strings.Contains(s, "|var:"+varName+"}}")
}

func runtimeDefaults(ctx context.Context, deps CompatDeps) stateservice.RuntimeDefaults {
	return stateservice.ResolveRuntimeDefaults(ctx, deps.StateService, deps.Defaults)
}

// buildTemplateVars constructs the template-var map for a compat request.
// Values are only injected when the chain actually uses the corresponding
// {{var:*}} macro, so hardcoded chain configs remain authoritative.
func buildTemplateVars(chain *taskengine.TaskChainDefinition, defaults stateservice.RuntimeDefaults, model string, maxTokens *int) map[string]string {
	defaults = defaults.Trimmed()
	vars := map[string]string{}
	if chainUsesVar(chain, "model") {
		if strings.TrimSpace(model) != "" {
			vars["model"] = strings.TrimSpace(model)
		}
	}
	if chainUsesVar(chain, "provider") && defaults.Provider != "" {
		vars["provider"] = defaults.Provider
	}
	if chainUsesVar(chain, "alt_model") && defaults.AltModel != "" {
		vars["alt_model"] = defaults.AltModel
	}
	if chainUsesVar(chain, "alt_provider") && defaults.AltProvider != "" {
		vars["alt_provider"] = defaults.AltProvider
	}
	if chainUsesVar(chain, "max_tokens") {
		if maxTokens != nil {
			vars["max_tokens"] = strconv.Itoa(*maxTokens)
		} else if defaults.MaxTokens != "" {
			vars["max_tokens"] = defaults.MaxTokens
		}
	}
	if chainUsesVar(chain, "think") && defaults.Think != "" {
		vars["think"] = defaults.Think
	}
	return vars
}

var idCounter atomic.Uint64

func newCompletionID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, idCounter.Add(1))
}

func unixNow() int64 {
	return time.Now().Unix()
}

// resolveRequestedModel keeps client model selection inside the models Contenox
// can route through the configured provider. Empty, "default", unknown IDE model
// IDs, and cross-provider model IDs resolve to the configured default model
// when one exists. Known models for the configured provider pass through.
func resolveRequestedModel(ctx context.Context, deps CompatDeps, defaults stateservice.RuntimeDefaults, requested string) string {
	r := strings.TrimSpace(requested)
	defaultModel := strings.TrimSpace(defaults.Model)
	if r == "" || r == "default" {
		return defaultModel
	}
	if defaultModel != "" && r == defaultModel {
		return r
	}

	if deps.StateService != nil {
		ok, err := modelObservedForDefaultProvider(ctx, deps, defaults, r)
		if err == nil && ok {
			return r
		}
		if defaultModel != "" {
			return defaultModel
		}
	}
	if deps.StateService != nil && defaultModel != "" {
		return defaultModel
	}
	return r
}

func modelObservedForDefaultProvider(ctx context.Context, deps CompatDeps, defaults stateservice.RuntimeDefaults, requested string) (bool, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false, nil
	}
	defaultProvider := strings.TrimSpace(defaults.Provider)
	if defaultProvider == "" || deps.StateService == nil {
		observed, err := observedModels(ctx, deps)
		if err != nil {
			return false, err
		}
		for _, model := range observed {
			if requested == model.ID || requested == model.Model {
				return true, nil
			}
		}
		return false, nil
	}
	states, err := deps.StateService.Get(ctx)
	if err != nil {
		return false, err
	}
	for _, state := range states {
		if !stateMatchesProvider(state, defaultProvider) {
			continue
		}
		for _, pulled := range state.PulledModels {
			name := strings.TrimSpace(pulled.Model)
			if name == "" {
				name = strings.TrimSpace(pulled.Name)
			}
			if requested == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func stateMatchesProvider(state statetype.BackendRuntimeState, provider string) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(state.Backend.Type), provider) ||
		strings.EqualFold(strings.TrimSpace(state.Backend.Name), provider)
}

func observedModels(ctx context.Context, deps CompatDeps) ([]backendapi.ObservedModel, error) {
	if deps.StateService == nil {
		return nil, nil
	}
	states, err := deps.StateService.Get(ctx)
	if err != nil {
		return nil, err
	}
	return backendapi.ListObservedModels(states), nil
}

func patchExecOverrides(chain *taskengine.TaskChainDefinition, temp *float64) *taskengine.TaskChainDefinition {
	if chain == nil || temp == nil {
		return chain
	}
	clone := *chain
	clone.Tasks = append([]taskengine.TaskDefinition(nil), chain.Tasks...)
	for i := range clone.Tasks {
		if clone.Tasks[i].ExecuteConfig == nil {
			continue
		}
		cfg := *clone.Tasks[i].ExecuteConfig
		if temp != nil {
			t := float32(*temp)
			cfg.Temperature = &t
		}
		clone.Tasks[i].ExecuteConfig = &cfg
		break
	}
	return &clone
}

func applyStopStrings(text string, stop []string) string {
	first := -1
	for _, s := range stop {
		if s == "" {
			continue
		}
		if idx := strings.Index(text, s); idx >= 0 && (first == -1 || idx < first) {
			first = idx
		}
	}
	if first >= 0 {
		return text[:first]
	}
	return text
}

func buildFIMHistory(prompt, suffix string) taskengine.ChatHistory {
	return taskengine.ChatHistory{
		Messages: []taskengine.Message{{
			Role:    "user",
			Content: fmt.Sprintf("<fim_prefix>%s<fim_suffix>%s<fim_middle>", prompt, suffix),
		}},
	}
}

// lastAssistantMessage returns the content of the last assistant message in the history.
func lastAssistantMessage(messages []taskengine.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" && m.Content != "" {
			return m.Content
		}
	}
	return ""
}

// writeSSE writes a single SSE data frame and flushes if possible.
func writeSSE(w http.ResponseWriter, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// writeDone writes the terminal SSE [DONE] frame.
func writeDone(w http.ResponseWriter) {
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// setSSEHeaders sets the required headers for an SSE response.
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
}

// rootModels returns a handler that serves GET /v1/models using the stateService.
func rootModels(deps CompatDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := authorizeCompatRequest(r, deps, false); err != nil {
			http.Error(w, `{"error":{"message":"Unauthorized","type":"auth_error"}}`, http.StatusUnauthorized)
			return
		}
		listModels(w, r, deps)
	}
}

// listModels writes the OpenAI-compatible model list from stateService.
func listModels(w http.ResponseWriter, r *http.Request, deps CompatDeps) {
	limit := 100
	limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			http.Error(w, `{"error":{"message":"invalid limit","type":"invalid_request_error"}}`, http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	observed, err := observedModels(r.Context(), deps)
	if err != nil {
		http.Error(w, `{"error":{"message":"internal error","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}
	defaults := runtimeDefaults(r.Context(), deps)
	data := backendapi.OpenAIModelsFromObserved(observed, defaults.Model, unixNow())
	if limit < len(data) {
		data = data[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(backendapi.OpenAICompatibleModelList{Object: "list", Data: data})
}

func authorizeCompatRequest(r *http.Request, deps CompatDeps, mutating bool) error {
	if deps.Auth != nil {
		return authorize(r.Context(), deps.Auth)
	}
	token := strings.TrimSpace(deps.Token)
	if token == "" || !mutating {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(extractCompatToken(r)), []byte(token)) != 1 {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

func extractCompatToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
		return strings.TrimSpace(apiKey[7:])
	}
	return apiKey
}

// authorize checks the auth reader when it is non-nil.
func authorize(ctx context.Context, auth interface {
	GetIdentity(context.Context) (string, error)
}) error {
	if auth == nil {
		return nil
	}
	_, err := auth.GetIdentity(ctx)
	return err
}

func compatSessionID(ctx context.Context, w http.ResponseWriter, r *http.Request, deps CompatDeps) (string, error) {
	sessionID := strings.TrimSpace(r.Header.Get("X-Session-ID"))
	if sessionID == "" {
		return "", nil
	}
	if strings.EqualFold(sessionID, "new") {
		if deps.Agent == nil {
			return "", fmt.Errorf("agent is not configured")
		}
		created, err := deps.Agent.SessionNew(ctx, "compat-"+newCompletionID("session"))
		if err != nil {
			return "", err
		}
		sessionID = created
	}
	w.Header().Set("X-Session-ID", sessionID)
	return sessionID, nil
}

func openAIFinishReason(resp *agentservice.PromptResponse) string {
	if resp == nil {
		return "stop"
	}
	if resp.StopReason == agentservice.StopMaxTokens {
		return "length"
	}
	return "stop"
}

// finishReason returns a pointer to the given string (for optional JSON fields).
func finishReason(s string) *string { return &s }
func nullFinishReason() *string     { return nil }
