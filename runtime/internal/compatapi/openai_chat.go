package compatapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

type chatHandler struct {
	deps CompatDeps
}

// handle serves an OpenAI-compatible chat completion, answering with a single
// JSON completion object or, when the request sets stream, the same content
// as SSE chat.completion.chunk frames over text/event-stream.
func (h *chatHandler) handle(w http.ResponseWriter, r *http.Request) {
	// @request compatapi.ChatCompletionRequest
	// @response compatapi.chatCompletionResponse
	ctx := r.Context()
	if err := authorizeCompatRequest(r, h.deps, true); err != nil {
		http.Error(w, `{"error":{"message":"Unauthorized","type":"auth_error"}}`, http.StatusUnauthorized)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body","type":"invalid_request_error"}}`, http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, `{"error":{"message":"messages is required","type":"invalid_request_error"}}`, http.StatusBadRequest)
		return
	}
	defaults := runtimeDefaults(ctx, h.deps)
	if h.deps.Agent == nil || h.deps.Chains == nil {
		http.Error(w, `{"error":{"message":"compat dependencies are not configured","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}

	chainID := strings.TrimSpace(r.PathValue("chainID"))
	chainRef := defaults.ChainRef
	if chainID != "" {
		chainRef = chainID
	}
	if chainRef == "" {
		http.Error(w, `{"error":{"message":"no compat chain configured","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}

	chain, err := h.deps.Chains.Get(ctx, chainRef)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"chain not found: %s","type":"invalid_request_error"}}`, chainRef), http.StatusBadRequest)
		return
	}

	model := resolveRequestedModel(ctx, h.deps, defaults, req.Model)
	maxTokens := req.MaxTokens
	if req.MaxCompletionTokens != nil {
		maxTokens = req.MaxCompletionTokens
	}
	templateVars := buildTemplateVars(chain, defaults, model, maxTokens)
	chain = patchExecOverrides(chain, req.Temperature)
	sessionID, err := compatSessionID(ctx, w, r, h.deps)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s","type":"server_error"}}`, jsonEscape(err.Error())), http.StatusInternalServerError)
		return
	}

	messages := make([]taskengine.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, taskengine.Message{
			Role:    m.Role,
			Content: m.Content,
			Images:  m.Images,
		})
	}
	chatHistory := taskengine.ChatHistory{Messages: messages}

	resp, err := h.deps.Agent.Prompt(ctx, agentservice.PromptRequest{
		SessionID:    sessionID,
		InputValue:   chatHistory,
		InputType:    taskengine.DataTypeChatHistory,
		Chain:        chain,
		TemplateVars: templateVars,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s","type":"server_error"}}`, jsonEscape(err.Error())), http.StatusInternalServerError)
		return
	}

	reply := extractChatReply(resp)
	reply = applyStopStrings(reply, req.Stop)
	id := newCompletionID("chatcmpl")
	ts := unixNow()

	if req.Stream {
		h.writeStreamingResponse(w, id, ts, model, reply, resp)
		return
	}
	h.writeJSONResponse(w, id, ts, model, reply, resp)
}

func (h *chatHandler) writeStreamingResponse(w http.ResponseWriter, id string, ts int64, model, reply string, resp *agentservice.PromptResponse) {
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)

	// Frame 1: role init
	_ = writeSSE(w, chatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: ts,
		Model:   model,
		Choices: []chatChunkChoice{{
			Index:        0,
			Delta:        chatDelta{Role: "assistant"},
			FinishReason: nullFinishReason(),
		}},
	})

	// Frame 2: full content (fake streaming — single chunk)
	_ = writeSSE(w, chatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: ts,
		Model:   model,
		Choices: []chatChunkChoice{{
			Index:        0,
			Delta:        chatDelta{Content: reply},
			FinishReason: nullFinishReason(),
		}},
	})

	// Frame 3: stop + usage
	usage := extractUsage(resp)
	stopReason := openAIFinishReason(resp)
	_ = writeSSE(w, chatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: ts,
		Model:   model,
		Choices: []chatChunkChoice{{
			Index:        0,
			Delta:        chatDelta{},
			FinishReason: finishReason(stopReason),
		}},
		Usage: &usage,
	})

	writeDone(w)
}

func (h *chatHandler) writeJSONResponse(w http.ResponseWriter, id string, ts int64, model, reply string, resp *agentservice.PromptResponse) {
	usage := extractUsage(resp)
	out := chatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: ts,
		Model:   model,
		Choices: []chatChoiceResponse{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: reply,
			},
			FinishReason: openAIFinishReason(resp),
		}},
		Usage: usage,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func extractChatReply(resp *agentservice.PromptResponse) string {
	if resp == nil {
		return ""
	}
	switch resp.OutputType {
	case taskengine.DataTypeChatHistory:
		if hist, ok := resp.Output.(taskengine.ChatHistory); ok {
			return lastAssistantMessage(hist.Messages)
		}
		if hist, ok := resp.Output.(*taskengine.ChatHistory); ok && hist != nil {
			return lastAssistantMessage(hist.Messages)
		}
	case taskengine.DataTypeString:
		if s, ok := resp.Output.(string); ok {
			return s
		}
	}
	if s, ok := resp.Output.(string); ok {
		return s
	}
	return ""
}

func extractUsage(resp *agentservice.PromptResponse) chatCompletionUsage {
	if resp == nil {
		return chatCompletionUsage{}
	}
	if hist, ok := resp.Output.(taskengine.ChatHistory); ok {
		return chatCompletionUsage{
			PromptTokens:     hist.InputTokens,
			CompletionTokens: hist.OutputTokens,
			TotalTokens:      hist.InputTokens + hist.OutputTokens,
		}
	}
	if hist, ok := resp.Output.(*taskengine.ChatHistory); ok && hist != nil {
		return chatCompletionUsage{
			PromptTokens:     hist.InputTokens,
			CompletionTokens: hist.OutputTokens,
			TotalTokens:      hist.InputTokens + hist.OutputTokens,
		}
	}
	var usage chatCompletionUsage
	for _, step := range resp.Steps {
		if step.TokenUsage == nil {
			continue
		}
		usage.PromptTokens += step.TokenUsage.Prompt
		usage.CompletionTokens += step.TokenUsage.Completion
		usage.TotalTokens += step.TokenUsage.Total
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.TotalTokens != 0 {
		return usage
	}
	return chatCompletionUsage{}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	// json.Marshal wraps in quotes; strip them.
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}
