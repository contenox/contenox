package internalchatapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
)

type ChatDeps struct {
	Agent              agentservice.Agent
	ChatMgr            *chatservice.Manager
	Chains             taskchainservice.Service
	DB                 libdb.DBManager
	DefaultChainRef    string
	DefaultModel       string
	DefaultProvider    string
	AltDefaultModel    string
	AltDefaultProvider string
	DefaultMaxTokens   string
	DefaultThink       string
}

func AddChatRoutes(mux *http.ServeMux, deps ChatDeps, auth middleware.AuthZReader) {
	h := &chatHandler{deps: deps, auth: auth}
	mux.HandleFunc("POST /chats", h.createChat)
	mux.HandleFunc("GET /chats", h.listChats)
	mux.HandleFunc("GET /chats/{id}", h.history)
	mux.HandleFunc("POST /chats/{id}/chat", h.chat)
}

type chatHandler struct {
	deps ChatDeps
	auth middleware.AuthZReader
}

type newChatInstanceRequest struct {
	Name  string `json:"name,omitempty"`
	Model string `json:"model,omitempty"`
}

type chatSession struct {
	ID           string       `json:"id"`
	Name         string       `json:"name,omitempty"`
	StartedAt    time.Time    `json:"startedAt,omitempty"`
	Model        string       `json:"model,omitempty"`
	MessageCount int          `json:"messageCount,omitempty"`
	IsActive     bool         `json:"isActive,omitempty"`
	LastMessage  *chatMessage `json:"lastMessage,omitempty"`
}

type chatMessage struct {
	ID         string                `json:"id"`
	Role       string                `json:"role"`
	Content    string                `json:"content,omitempty"`
	Thinking   string                `json:"thinking,omitempty"`
	SentAt     time.Time             `json:"sentAt"`
	IsUser     bool                  `json:"isUser"`
	IsLatest   bool                  `json:"isLatest"`
	CallTools  []taskengine.ToolCall `json:"callTools,omitempty" openapi_include_type:"taskengine.ToolCall"`
	ToolCallID string                `json:"toolCallId,omitempty"`
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	RequestID        string                         `json:"requestId,omitempty"`
	Response         string                         `json:"response"`
	State            []taskengine.CapturedStateUnit `json:"state" openapi_include_type:"taskengine.CapturedStateUnit"`
	InputTokenCount  int                            `json:"inputTokenCount"`
	OutputTokenCount int                            `json:"outputTokenCount"`
	StopReason       agentservice.StopReason        `json:"stopReason,omitempty"`
	Error            string                         `json:"error,omitempty"`
}

func (h *chatHandler) authorize(ctx context.Context) error {
	if h.auth == nil {
		return nil
	}
	_, err := h.auth.GetIdentity(ctx)
	return err
}

func (h *chatHandler) createChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(ctx); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.deps.Agent == nil {
		_ = apiframework.Error(w, r, fmt.Errorf("chat agent is not configured"), apiframework.ServerOperation)
		return
	}

	req, err := apiframework.Decode[newChatInstanceRequest](r) // @request internalchatapi.newChatInstanceRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	name := strings.TrimSpace(req.Name)
	chatID, err := h.deps.Agent.SessionNew(ctx, name)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	resp := chatSession{
		ID:        chatID,
		Name:      name,
		StartedAt: time.Now().UTC(),
		Model:     strings.TrimSpace(req.Model),
		IsActive:  true,
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, resp) // @response internalchatapi.chatSession
}

func (h *chatHandler) listChats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(ctx); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.deps.Agent == nil {
		_ = apiframework.Error(w, r, fmt.Errorf("chat agent is not configured"), apiframework.ServerOperation)
		return
	}

	sessions, err := h.deps.Agent.SessionList(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	// Build sessions with last message timestamps for sorting
	type sessionWithTimestamp struct {
		session   *agentservice.SessionInfo
		lastMsg   *chatMessage
		lastMsgAt time.Time
	}

	var withTs []sessionWithTimestamp
	for _, s := range sessions {
		var lastMsg *chatMessage
		var lastMsgAt time.Time
		if h.deps.ChatMgr != nil && h.deps.DB != nil {
			msgs, err := h.deps.ChatMgr.ListMessages(ctx, h.deps.DB.WithoutTransaction(), s.ID)
			if err == nil && len(msgs) > 0 {
				lastMsgAt = msgs[len(msgs)-1].Timestamp
				lastMsg = &chatMessage{
					ID:         msgs[len(msgs)-1].ID,
					Role:       msgs[len(msgs)-1].Role,
					Content:    msgs[len(msgs)-1].Content,
					Thinking:   msgs[len(msgs)-1].Thinking,
					SentAt:     msgs[len(msgs)-1].Timestamp,
					IsUser:     msgs[len(msgs)-1].Role == "user",
					IsLatest:   true,
					CallTools:  msgs[len(msgs)-1].CallTools,
					ToolCallID: msgs[len(msgs)-1].ToolCallID,
				}
			}
		}
		withTs = append(withTs, sessionWithTimestamp{
			session:   s,
			lastMsg:   lastMsg,
			lastMsgAt: lastMsgAt,
		})
	}

	// Sort by last message timestamp, newest first
	sort.Slice(withTs, func(i, j int) bool {
		return withTs[i].lastMsgAt.After(withTs[j].lastMsgAt)
	})

	resp := make([]chatSession, 0, len(withTs))
	for _, w := range withTs {
		resp = append(resp, chatSession{
			ID:           w.session.ID,
			Name:         w.session.Name,
			MessageCount: w.session.MessageCount,
			IsActive:     w.session.IsActive,
			LastMessage:  w.lastMsg,
		})
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response []internalchatapi.chatSession
}

func (h *chatHandler) history(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(ctx); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.deps.ChatMgr == nil || h.deps.DB == nil {
		_ = apiframework.Error(w, r, fmt.Errorf("chat manager is not configured"), apiframework.ServerOperation)
		return
	}

	id := strings.TrimSpace(apiframework.GetPathParam(r, "id", "The unique identifier of the chat session.")) // @param id string
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("chat ID is required: %w", apiframework.ErrBadPathValue), apiframework.GetOperation)
		return
	}

	msgs, err := h.deps.ChatMgr.ListMessages(ctx, h.deps.DB.WithoutTransaction(), id)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}

	resp := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		resp = append(resp, chatMessage{
			ID:         m.ID,
			Role:       m.Role,
			Content:    m.Content,
			Thinking:   m.Thinking,
			SentAt:     m.Timestamp,
			IsUser:     m.Role == "user",
			CallTools:  m.CallTools,
			ToolCallID: m.ToolCallID,
		})
	}
	if len(resp) > 0 {
		resp[len(resp)-1].IsLatest = true
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response []internalchatapi.chatMessage
}

func (h *chatHandler) chat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.authorize(ctx); err != nil {
		_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
		return
	}
	if h.deps.Agent == nil || h.deps.Chains == nil {
		_ = apiframework.Error(w, r, fmt.Errorf("chat dependencies are not configured"), apiframework.ServerOperation)
		return
	}

	id := strings.TrimSpace(apiframework.GetPathParam(r, "id", "The unique identifier of the chat session.")) // @param id string
	if id == "" {
		_ = apiframework.Error(w, r, fmt.Errorf("chat ID is required: %w", apiframework.ErrBadPathValue), apiframework.CreateOperation)
		return
	}

	chainRef := strings.TrimSpace(apiframework.GetQueryParam(r, "chainId", "", "The ID or JSON path of the task chain. Defaults to the workspace default chain.")) // @param chainId string
	model := strings.TrimSpace(apiframework.GetQueryParam(r, "model", "", "Optional model override."))                                                             // @param model string
	provider := strings.TrimSpace(apiframework.GetQueryParam(r, "provider", "", "Optional provider override."))                                                    // @param provider string
	think := strings.TrimSpace(apiframework.GetQueryParam(r, "think", "", "Optional reasoning level override."))                                                   // @param think string

	req, err := apiframework.Decode[chatRequest](r) // @request internalchatapi.chatRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("message is required"), apiframework.CreateOperation)
		return
	}

	if chainRef == "" {
		chainRef = h.deps.DefaultChainRef
	}
	if chainRef == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest(
			"no chain selected: pass ?chainId=... or configure a workspace default with `contenox config set default-chain <path>`",
		), apiframework.CreateOperation)
		return
	}

	chain, err := h.deps.Chains.Get(ctx, chainRef)
	if err != nil {
		_ = apiframework.Error(w, r, apiframework.BadRequest("chain not found: "+err.Error()), apiframework.CreateOperation)
		return
	}

	if model == "" {
		model = h.deps.DefaultModel
	}
	if provider == "" {
		provider = h.deps.DefaultProvider
	}
	if think == "" {
		think = h.deps.DefaultThink
	}

	templateVars := map[string]string{}
	if model != "" {
		templateVars["model"] = model
	}
	if provider != "" {
		templateVars["provider"] = provider
	}
	if h.deps.AltDefaultModel != "" {
		templateVars["alt_model"] = h.deps.AltDefaultModel
	}
	if h.deps.AltDefaultProvider != "" {
		templateVars["alt_provider"] = h.deps.AltDefaultProvider
	}
	if h.deps.DefaultMaxTokens != "" {
		templateVars["max_tokens"] = h.deps.DefaultMaxTokens
	}
	if think != "" {
		templateVars["think"] = think
	}

	resp, err := h.deps.Agent.Prompt(ctx, agentservice.PromptRequest{
		SessionID:    id,
		Input:        message,
		Chain:        chain,
		TemplateVars: templateVars,
	})
	if err != nil {
		if isMissingModelProvider(err) {
			_ = apiframework.Error(w, r, apiframework.BadRequest(
				"model and provider are required when the chain references {{var:model}} / {{var:provider}}. "+
					"Set them with `contenox config set default-model <name>` and `contenox config set default-provider <type>`, "+
					"or pass ?model=...&provider=... on the request.",
			), apiframework.CreateOperation)
			return
		}
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	out := chatResponse{
		RequestID:  requestID(ctx),
		Response:   extractReply(resp.Output, resp.OutputType),
		State:      resp.Steps,
		StopReason: resp.StopReason,
	}
	if hist, ok := resp.Output.(taskengine.ChatHistory); ok {
		out.InputTokenCount = hist.InputTokens
		out.OutputTokenCount = hist.OutputTokens
	}
	_ = apiframework.Encode(w, r, http.StatusOK, out) // @response internalchatapi.chatResponse
}

func requestID(ctx context.Context) string {
	reqID, _ := ctx.Value(libtracker.ContextKeyRequestID).(string)
	return reqID
}

func extractReply(output any, outputType taskengine.DataType) string {
	switch outputType {
	case taskengine.DataTypeChatHistory:
		if hist, ok := output.(taskengine.ChatHistory); ok {
			return lastAssistantMessage(hist.Messages)
		}
		if hist, ok := output.(*taskengine.ChatHistory); ok && hist != nil {
			return lastAssistantMessage(hist.Messages)
		}
	case taskengine.DataTypeString:
		if s, ok := output.(string); ok {
			return s
		}
	}
	if s, ok := output.(string); ok {
		return s
	}
	return ""
}

func lastAssistantMessage(messages []taskengine.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" && m.Content != "" {
			return m.Content
		}
	}
	return ""
}

func isMissingModelProvider(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "missing model provider") ||
		strings.Contains(s, "no model resolver") ||
		strings.Contains(s, "no provider configured")
}
