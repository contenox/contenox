package internalchatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/apiframework"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chatservice"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/messagestore"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

type fakeAgent struct {
	sessions   []*agentservice.SessionInfo
	newID      string
	lastPrompt agentservice.PromptRequest
}

func (f *fakeAgent) Capabilities(context.Context) (*agentservice.AgentCapabilities, error) {
	return &agentservice.AgentCapabilities{SupportsSession: true}, nil
}

func (f *fakeAgent) SessionNew(context.Context, string) (string, error) {
	if f.newID != "" {
		return f.newID, nil
	}
	return "chat-1", nil
}

func (f *fakeAgent) SessionList(context.Context) ([]*agentservice.SessionInfo, error) {
	return f.sessions, nil
}

func (f *fakeAgent) SessionLoad(context.Context, string) (string, []taskengine.Message, error) {
	return "", nil, nil
}

func (f *fakeAgent) SessionEnsureDefault(context.Context) (string, error) {
	return "default", nil
}

func (f *fakeAgent) Prompt(_ context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	f.lastPrompt = req
	return &agentservice.PromptResponse{
		OutputType: taskengine.DataTypeChatHistory,
		Output: taskengine.ChatHistory{
			Messages: []taskengine.Message{
				{Role: "user", Content: req.Input},
				{Role: "assistant", Content: "hello from agent"},
			},
			InputTokens:  3,
			OutputTokens: 4,
		},
		StopReason: agentservice.StopEndTurn,
	}, nil
}

type fakeChains struct {
	chain *taskengine.TaskChainDefinition
}

func (f fakeChains) Get(context.Context, string) (*taskengine.TaskChainDefinition, error) {
	return f.chain, nil
}

func (f fakeChains) List(context.Context) ([]string, error) {
	return []string{"default-chain.json"}, nil
}

func (f fakeChains) CreateAtPath(context.Context, string, *taskengine.TaskChainDefinition) error {
	return nil
}

func (f fakeChains) UpdateAtPath(context.Context, string, *taskengine.TaskChainDefinition) error {
	return nil
}

func (f fakeChains) DeleteByPath(context.Context, string) error {
	return nil
}

type fakeStateService struct {
	config stateservice.CLIConfigSnapshot
}

func (f fakeStateService) Get(context.Context) ([]statetype.BackendRuntimeState, error) {
	return nil, nil
}

func (f fakeStateService) SetupStatus(context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}

func (f fakeStateService) Refresh(context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}

func (f fakeStateService) CLIConfig(context.Context) (stateservice.CLIConfigSnapshot, error) {
	return f.config, nil
}

func (f fakeStateService) SetCLIConfig(context.Context, stateservice.CLIConfigPatch) (stateservice.CLIConfigSnapshot, error) {
	return f.config, nil
}

func TestCreateListAndPromptChat(t *testing.T) {
	agent := &fakeAgent{
		newID: "chat-new",
		sessions: []*agentservice.SessionInfo{
			{ID: "chat-new", Name: "work", MessageCount: 2, IsActive: true},
		},
	}
	chain := &taskengine.TaskChainDefinition{
		ID:    "default",
		Tasks: []taskengine.TaskDefinition{{ID: "one", Handler: taskengine.HandleNoop}},
	}
	mux := http.NewServeMux()
	AddChatRoutes(mux, ChatDeps{
		Agent:  agent,
		Chains: fakeChains{chain: chain},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef:    "default-chain.json",
			Model:       "model-a",
			Provider:    "provider-a",
			AltModel:    "model-b",
			AltProvider: "provider-b",
			MaxTokens:   "8192",
			Think:       "medium",
		},
	}, nil)
	handler := apiframework.RequestIDMiddleware(mux)

	body := bytes.NewBufferString(`{"name":"work"}`)
	req := httptest.NewRequest(http.MethodPost, "/chats", body)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusCreated, res.Code)
	require.Contains(t, res.Body.String(), `"id":"chat-new"`)

	req = httptest.NewRequest(http.MethodGet, "/chats", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"messageCount":2`)

	body = bytes.NewBufferString(`{"message":"hi"}`)
	req = httptest.NewRequest(http.MethodPost, "/chats/chat-new/chat", body)
	req.Header.Set("X-Request-ID", "req-chat")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"requestId":"req-chat"`)
	require.Contains(t, res.Body.String(), `"response":"hello from agent"`)
	require.Equal(t, "chat-new", agent.lastPrompt.SessionID)
	require.Equal(t, "model-a", agent.lastPrompt.TemplateVars["model"])
	require.Equal(t, "provider-a", agent.lastPrompt.TemplateVars["provider"])
	require.Equal(t, "model-b", agent.lastPrompt.TemplateVars["alt_model"])
	require.Equal(t, "provider-b", agent.lastPrompt.TemplateVars["alt_provider"])
	require.Equal(t, "8192", agent.lastPrompt.TemplateVars["max_tokens"])
	require.Equal(t, "medium", agent.lastPrompt.TemplateVars["think"])
}

func TestChatUsesCurrentCLIConfigDefaults(t *testing.T) {
	agent := &fakeAgent{newID: "chat-current"}
	chain := &taskengine.TaskChainDefinition{
		ID:    "default",
		Tasks: []taskengine.TaskDefinition{{ID: "one", Handler: taskengine.HandleNoop}},
	}
	mux := http.NewServeMux()
	AddChatRoutes(mux, ChatDeps{
		Agent:  agent,
		Chains: fakeChains{chain: chain},
		Defaults: stateservice.RuntimeDefaults{
			ChainRef:  "default-chain.json",
			Model:     "stale-model",
			Provider:  "stale-provider",
			MaxTokens: "1024",
		},
		StateService: fakeStateService{config: stateservice.CLIConfigSnapshot{
			DefaultModel:       "current-model",
			DefaultProvider:    "current-provider",
			DefaultAltModel:    "current-alt-model",
			DefaultAltProvider: "current-alt-provider",
			DefaultMaxTokens:   "8192",
		}},
	}, nil)

	body := bytes.NewBufferString(`{"message":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/chats/chat-current/chat", body)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "current-model", agent.lastPrompt.TemplateVars["model"])
	require.Equal(t, "current-provider", agent.lastPrompt.TemplateVars["provider"])
	require.Equal(t, "current-alt-model", agent.lastPrompt.TemplateVars["alt_model"])
	require.Equal(t, "current-alt-provider", agent.lastPrompt.TemplateVars["alt_provider"])
	require.Equal(t, "8192", agent.lastPrompt.TemplateVars["max_tokens"])
}

func TestHistoryReadsPersistedMessages(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	const workspaceID = "ws-history"
	const chatID = "chat-history"

	store := messagestore.New(db.WithoutTransaction(), workspaceID)
	require.NoError(t, store.CreateNamedMessageIndex(ctx, chatID, "local-user", "history"))

	mgr := chatservice.NewManager(workspaceID)
	require.NoError(t, mgr.PersistDiff(ctx, db.WithoutTransaction(), chatID, []taskengine.Message{
		{ID: "m1", Role: "user", Content: "hi", Timestamp: time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)},
		{ID: "m2", Role: "assistant", Content: "hello", Thinking: "short", Timestamp: time.Date(2026, 6, 7, 10, 0, 1, 0, time.UTC)},
	}))

	mux := http.NewServeMux()
	AddChatRoutes(mux, ChatDeps{ChatMgr: mgr, DB: db}, nil)

	req := httptest.NewRequest(http.MethodGet, "/chats/"+chatID, nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var messages []chatMessage
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &messages))
	require.Len(t, messages, 2)
	require.True(t, messages[1].IsLatest)
	require.Equal(t, "short", messages[1].Thinking)
}

func openTestDB(t *testing.T, ctx context.Context) libdb.DBManager {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "chatapi.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}
