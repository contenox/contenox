package runtimestate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/models/modelauth"
	"github.com/contenox/contenox/internal/models/modelrepo"
	_ "github.com/contenox/contenox/internal/models/modelrepo/openai"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type codexTestTransport func(*http.Request) (*http.Response, error)

func (f codexTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnit_CodexRuntime_LoginCatalogExecutionLogout(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "codex.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()
	store := runtimetypes.New(db.WithoutTransaction())
	b := &runtimetypes.Backend{ID: "codex", Name: "chatgpt", Type: modelauth.ProviderType, BaseURL: modelauth.BaseURL}
	require.NoError(t, store.CreateBackend(ctx, b))
	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	state, err := New(ctx, db, bus)
	require.NoError(t, err)
	state.processCodexBackend(ctx, b, nil)
	require.Contains(t, state.Get(ctx)[b.ID].Error, "login required")
	seed := func(generation string) {
		raw, err := json.Marshal(map[string]any{"generation": generation, "account": "account", "token": &oauth2.Token{AccessToken: "secret", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}})
		require.NoError(t, err)
		require.NoError(t, store.SetKV(ctx, "model-oauth:"+b.ID, raw))
	}
	seed("first")
	old := modelrepo.SharedHTTPClient
	t.Cleanup(func() { modelrepo.SharedHTTPClient = old })
	modelsCalls := 0
	modelrepo.SharedHTTPClient = &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "chatgpt.com", r.URL.Host)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n"
		if strings.HasSuffix(r.URL.Path, "/models") {
			modelsCalls++
			body = `{"models":[{"slug":"test-model","visibility":"list","context_window":32000,"input_modalities":["text","image"]}]}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	state.processCodexBackend(ctx, b, nil)
	state.processCodexBackend(ctx, b, nil)
	require.Equal(t, 1, modelsCalls)
	view := state.Get(ctx)
	require.Empty(t, view[b.ID].Error)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret")
	providers, err := LocalProviderAdapter(ctx, libtracker.NoopTracker{}, view)(ctx, modelauth.ProviderType)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.True(t, providers[0].CanVision())
	client, err := providers[0].GetChatConnection(ctx, "")
	require.NoError(t, err)
	r, err := client.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "hello"}})
	require.NoError(t, err)
	require.Equal(t, "hello", r.Message.Content)
	seed("replacement")
	state.processCodexBackend(ctx, b, nil)
	require.Equal(t, 2, modelsCalls)
	require.NoError(t, modelauth.New(db).Logout(ctx, b.ID))
	_, err = client.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "hello"}})
	require.ErrorIs(t, err, modelauth.ErrLoginRequired)
	state.processCodexBackend(ctx, b, nil)
	require.Contains(t, state.Get(ctx)[b.ID].Error, "login required")
}
