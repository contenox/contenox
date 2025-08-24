package chatapi

import (
	"net/http"

	"github.com/contenox/runtime/chatservice"
	"github.com/contenox/runtime/internal/apiframework"
	"github.com/contenox/runtime/taskengine"
)

// SetTaskChainRequest defines the expected structure for configuring the task chain
type SetTaskChainRequest struct {
	// The ID of the task chain to use for OpenAI-compatible chat completions
	TaskChainID string `json:"taskChainID" example:"openai-compatible-chain"`
}

func AddChatRoutes(mux *http.ServeMux, chatService chatservice.Service) {
	h := &handler{service: chatService}

	// OpenAI-compatible endpoints
	mux.HandleFunc("POST /{chainID}/v1/chat/completions", h.openAIChatCompletions)
}

type handler struct {
	service chatservice.Service
}

type openAIChatResponse struct {
	ID                string                                `json:"id" example:"chat_123"`
	Object            string                                `json:"object" example:"chat.completion"`
	Created           int64                                 `json:"created" example:"1690000000"`
	Model             string                                `json:"model" example:"mistral:instruct"`
	Choices           []taskengine.OpenAIChatResponseChoice `json:"choices" openapi_include_type:"taskengine.OpenAIChatResponseChoice"`
	Usage             taskengine.OpenAITokenUsage           `json:"usage" openapi_include_type:"taskengine.OpenAITokenUsage"`
	SystemFingerprint string                                `json:"system_fingerprint,omitempty" example:"system_456"`
	StackTrace        []taskengine.CapturedStateUnit        `json:"stackTrace,omitempty"`
}

// Processes chat requests using the configured task chain.
//
// This endpoint provides OpenAI-compatible chat completions by executing
// the configured task chain with the provided request data.
// The task chain must be configured first using the /chat/taskchain endpoint.
func (h *handler) openAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chainID := apiframework.GetPathParam(r, "chainID", "The ID of the task chain to use.")
	req, err := apiframework.Decode[taskengine.OpenAIChatRequest](r) // @request taskengine.OpenAIChatRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	addTraces := apiframework.GetQueryParam(r, "stackTrace", "false", "If provided the stacktraces will be added to the response.")

	chatResp, traces, err := h.service.OpenAIChatCompletions(ctx, chainID, req)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	resp := openAIChatResponse{
		ID:                chatResp.ID,
		Object:            chatResp.Object,
		Created:           chatResp.Created,
		Model:             chatResp.Model,
		Choices:           chatResp.Choices,
		Usage:             chatResp.Usage,
		SystemFingerprint: chatResp.SystemFingerprint,
		StackTrace:        traces,
	}
	if addTraces != "true" && addTraces != "True" {
		resp.StackTrace = nil
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response chatapi.OpenAIChatResponse
}

type chainIDResponse struct {
	// The ID of the Task-Chain used as default for Open-AI chat/completions.
	ChainID string `json:"taskChainID" example:"openai-compatible-chain"`
}
