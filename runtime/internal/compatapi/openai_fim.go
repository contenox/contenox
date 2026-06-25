package compatapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

type fimHandler struct {
	deps CompatDeps
}

func (h *fimHandler) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := authorizeCompatRequest(r, h.deps, true); err != nil {
		http.Error(w, `{"error":{"message":"Unauthorized","type":"auth_error"}}`, http.StatusUnauthorized)
		return
	}

	var req FIMCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body","type":"invalid_request_error"}}`, http.StatusBadRequest)
		return
	}
	if h.deps.Agent == nil || h.deps.Chains == nil {
		http.Error(w, `{"error":{"message":"compat dependencies are not configured","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}

	chainID := strings.TrimSpace(r.PathValue("chainID"))
	chainRef := h.deps.DefaultFIMChainRef
	if chainID != "" {
		chainRef = chainID
	}
	if chainRef == "" {
		http.Error(w, `{"error":{"message":"no FIM chain configured","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}

	chain, err := h.deps.Chains.Get(ctx, chainRef)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"chain not found: %s","type":"invalid_request_error"}}`, chainRef), http.StatusBadRequest)
		return
	}

	model := resolveRequestedModel(ctx, h.deps, req.Model)
	templateVars := buildTemplateVars(chain, model, h.deps.DefaultProvider, h.deps.DefaultMaxTokens, req.MaxTokens)
	chain = patchExecOverrides(chain, req.Temperature)
	sessionID, err := compatSessionID(ctx, w, r, h.deps)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s","type":"server_error"}}`, jsonEscape(err.Error())), http.StatusInternalServerError)
		return
	}

	resp, err := h.deps.Agent.Prompt(ctx, agentservice.PromptRequest{
		SessionID:    sessionID,
		InputValue:   buildFIMHistory(req.Prompt, req.Suffix),
		InputType:    taskengine.DataTypeChatHistory,
		Chain:        chain,
		TemplateVars: templateVars,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s","type":"server_error"}}`, jsonEscape(err.Error())), http.StatusInternalServerError)
		return
	}

	completion := extractFIMCompletion(resp)
	completion = applyStopStrings(completion, req.Stop)
	id := newCompletionID("cmpl")
	ts := unixNow()

	if req.Stream {
		h.writeStreamingResponse(w, id, ts, model, completion, resp)
		return
	}
	h.writeJSONResponse(w, id, ts, model, completion, resp)
}

func (h *fimHandler) writeStreamingResponse(w http.ResponseWriter, id string, ts int64, model, completion string, resp *agentservice.PromptResponse) {
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)

	// Frame 1: full completion text (fake streaming — single chunk)
	_ = writeSSE(w, fimChunk{
		Choices: []fimChunkChoice{{
			Text:         completion,
			FinishReason: nullFinishReason(),
		}},
	})

	// Frame 2: stop + usage
	usage := extractUsage(resp)
	stopReason := openAIFinishReason(resp)
	_ = writeSSE(w, fimChunk{
		Choices: []fimChunkChoice{{
			Text:         "",
			FinishReason: finishReason(stopReason),
		}},
		Usage: &usage,
	})

	writeDone(w)
}

func (h *fimHandler) writeJSONResponse(w http.ResponseWriter, id string, ts int64, model, completion string, resp *agentservice.PromptResponse) {
	usage := extractUsage(resp)
	out := fimCompletionResponse{
		ID:      id,
		Object:  "text_completion",
		Created: ts,
		Model:   model,
		Choices: []fimChoiceResponse{{
			Text:         completion,
			FinishReason: openAIFinishReason(resp),
		}},
		Usage: usage,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func extractFIMCompletion(resp *agentservice.PromptResponse) string {
	if resp == nil {
		return ""
	}
	switch resp.OutputType {
	case taskengine.DataTypeString:
		if s, ok := resp.Output.(string); ok {
			return s
		}
	case taskengine.DataTypeChatHistory:
		if hist, ok := resp.Output.(taskengine.ChatHistory); ok {
			return lastAssistantMessage(hist.Messages)
		}
		if hist, ok := resp.Output.(*taskengine.ChatHistory); ok && hist != nil {
			return lastAssistantMessage(hist.Messages)
		}
	}
	if s, ok := resp.Output.(string); ok {
		return s
	}
	return ""
}
