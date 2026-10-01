package openai_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	_ "github.com/contenox/contenox/internal/models/modelrepo/openai"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

func codexLiveCatalog(t *testing.T, ctx context.Context) modelrepo.CatalogProvider {
	t.Helper()
	path := os.Getenv("CONTENOX_CHATGPT_SMOKE_DB")
	backend := os.Getenv("CONTENOX_CHATGPT_SMOKE_BACKEND")
	if path == "" || backend == "" {
		t.Skip("set CONTENOX_CHATGPT_SMOKE_DB and CONTENOX_CHATGPT_SMOKE_BACKEND after backend login")
	}
	_, err := os.Stat(path)
	require.NoError(t, err)
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	b, err := runtimetypes.New(db.WithoutTransaction()).GetBackendByName(ctx, backend)
	require.NoError(t, err)
	require.Equal(t, modelauth.ProviderType, b.Type)
	auth := modelauth.New(db)
	catalog, err := modelrepo.NewCatalogProvider(modelrepo.BackendSpec{Type: b.Type, BaseURL: b.BaseURL}, modelrepo.WithCatalogAuthorizer(func(ctx context.Context) (http.Header, error) { return auth.Headers(ctx, b.ID) }))
	require.NoError(t, err)
	return catalog
}

func TestLive_CodexCatalog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	catalog := codexLiveCatalog(t, ctx)
	models, err := catalog.ListModels(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, models)
	for _, m := range models {
		require.NotEmpty(t, m.Name)
		require.Equal(t, modelauth.ProviderType, catalog.ProviderFor(m).GetType())
	}
	t.Logf("discovered %d selectable models from the authenticated backend", len(models))
}

func TestLive_CodexSubscription(t *testing.T) {
	model := os.Getenv("CONTENOX_CHATGPT_SMOKE_MODEL")
	if model == "" {
		t.Skip("set CONTENOX_CHATGPT_SMOKE_MODEL to run inference; this test uses subscription quota")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	catalog := codexLiveCatalog(t, ctx)
	models, err := catalog.ListModels(ctx)
	require.NoError(t, err)
	var chosen *modelrepo.ObservedModel
	for _, m := range models {
		if m.Name == model {
			copy := m
			chosen = &copy
			break
		}
	}
	require.NotNil(t, chosen, "selected model must appear in the subscription catalog")
	client, err := catalog.ProviderFor(*chosen).GetChatConnection(ctx, "")
	require.NoError(t, err)
	tools := modelrepo.WithTools(modelrepo.Tool{Type: "function", Function: &modelrepo.FunctionTool{Name: "subscription_probe", Description: "Return a harmless test value.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}})
	inputs := []modelrepo.Message{{Role: "user", Content: "Call subscription_probe exactly once with no arguments; then reply with exactly its result."}}
	first, err := client.Chat(ctx, inputs, tools)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "subscription_probe", first.ToolCalls[0].Function.Name)
	require.JSONEq(t, "{}", first.ToolCalls[0].Function.Arguments)
	first.Message.ToolCalls = first.ToolCalls
	inputs = append(inputs, first.Message, modelrepo.Message{Role: "tool", ToolCallID: first.ToolCalls[0].ID, Content: "subscription-probe-ok"})
	last, err := client.Chat(ctx, inputs, tools)
	require.NoError(t, err)
	require.Empty(t, last.ToolCalls)
	require.Contains(t, last.Message.Content, "subscription-probe-ok")
}
