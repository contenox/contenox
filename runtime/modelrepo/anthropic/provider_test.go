package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/agent/runtime/modelrepo"
	msgcodec "github.com/contenox/agent/runtime/modelrepo/codec/messages"
	"github.com/stretchr/testify/require"
)

func TestUnit_AnthropicChat_RequestShapeAndResponse(t *testing.T) {
	var gotPath, gotAPIKey, gotVersion string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"hi there"}]}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider("secret-key", "claude-sonnet-4-5", []string{srv.URL},
		modelrepo.CapabilityConfig{CanChat: true}, srv.Client(), nil)
	chat, err := p.GetChatConnection(context.Background(), "")
	require.NoError(t, err)

	res, err := chat.Chat(context.Background(), []modelrepo.Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)

	require.Equal(t, "/v1/messages", gotPath)
	require.Equal(t, "secret-key", gotAPIKey, "direct Anthropic must auth via x-api-key")
	require.Equal(t, anthropicAPIVersion, gotVersion, "direct Anthropic must send anthropic-version header")
	require.Equal(t, "claude-sonnet-4-5", gotBody["model"], "direct Anthropic puts model in the body")
	require.Nil(t, gotBody["anthropic_version"], "anthropic_version is a Vertex-only body field")
	require.Equal(t, "hi there", res.Message.Content)
}

func TestUnit_AnthropicCatalog_RegisteredAndChatCapable(t *testing.T) {
	cp, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: "anthropic", APIKey: "k"})
	require.NoError(t, err, "anthropic must be registered in the catalog registry")
	require.Equal(t, "anthropic", cp.Type())

	prov := cp.ProviderFor(modelrepo.ObservedModel{
		Name:             "claude-sonnet-4-5",
		CapabilityConfig: modelrepo.CapabilityConfig{CanChat: true, CanStream: true, CanPrompt: true},
	})
	require.Equal(t, "anthropic", prov.GetType())
	require.True(t, prov.CanChat())
	require.False(t, prov.CanEmbed())
}

func TestUnit_AnthropicThinking_ManualBudgetAndAdaptiveEffort(t *testing.T) {
	manualCfg := &modelrepo.ChatConfig{}
	modelrepo.WithThink("medium").Apply(manualCfg)
	manualReq := msgcodec.Request{MaxTokens: 1500}
	applyAnthropicThinking(&manualReq, "claude-3-7-sonnet-latest", manualCfg)
	require.NotNil(t, manualReq.Thinking)
	require.Equal(t, "enabled", manualReq.Thinking.Type)
	require.Equal(t, 1499, manualReq.Thinking.BudgetTokens, "budget must stay below max_tokens")
	require.Nil(t, manualReq.OutputConfig)

	adaptiveCfg := &modelrepo.ChatConfig{}
	modelrepo.WithThink("xhigh").Apply(adaptiveCfg)
	adaptiveReq := msgcodec.Request{MaxTokens: 4096}
	applyAnthropicThinking(&adaptiveReq, "claude-opus-4-7", adaptiveCfg)
	require.NotNil(t, adaptiveReq.Thinking)
	require.Equal(t, "adaptive", adaptiveReq.Thinking.Type)
	require.Equal(t, "summarized", adaptiveReq.Thinking.Display)
	require.NotNil(t, adaptiveReq.OutputConfig)
	require.Equal(t, "xhigh", adaptiveReq.OutputConfig.Effort)
}

func TestUnit_AnthropicThinking_OffDisablesWhenSupported(t *testing.T) {
	cfg := &modelrepo.ChatConfig{}
	modelrepo.WithThink("off").Apply(cfg)
	req := msgcodec.Request{MaxTokens: 4096}
	applyAnthropicThinking(&req, "claude-3-7-sonnet-latest", cfg)
	require.NotNil(t, req.Thinking)
	require.Equal(t, "disabled", req.Thinking.Type)
}
