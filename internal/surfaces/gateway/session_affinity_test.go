package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/ollamatokenizer"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libbus "github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/stretchr/testify/require"
)

func stickyScript(t *testing.T, dir, name string) string {
	t.Helper()
	turns := make([]map[string]any, 0, 24)
	for i := range 24 {
		turns = append(turns, map[string]any{"text": fmt.Sprintf("%s-%d", name, i)})
	}
	body, err := json.Marshal(map[string]any{"model": "sticky-model", "turns": turns})
	require.NoError(t, err)
	path := filepath.Join(dir, name+".json")
	require.NoError(t, os.WriteFile(path, body, 0o600))
	return path
}

func stickyRepo(t *testing.T, backends int) llmrepo.ModelRepo {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(dir, "sticky.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	bus := libbus.NewInMem()
	rt, err := runtimestate.New(ctx, db, bus, runtimestate.WithAutoDiscoverModels())
	require.NoError(t, err)

	store := runtimetypes.New(db.WithoutTransaction())
	for i := range backends {
		name := fmt.Sprintf("upstream-%d", i)
		require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
			ID: name, Name: name,
			Type:    modelrepo.ScriptedTestBackendType,
			BaseURL: stickyScript(t, dir, name),
		}))
	}
	require.NoError(t, rt.RunBackendCycle(ctx))

	repo, err := llmrepo.NewModelManager(rt, ollamatokenizer.NewEstimateTokenizer(), llmrepo.ModelManagerConfig{}, libtracker.NoopTracker{})
	require.NoError(t, err)
	return repo
}

// A conversation reaching the same backend every turn is the whole point of
// carrying a session: a prefix cache is per backend, so a session that lands
// somewhere new each turn pays to re-read its own history every time.
func TestSystem_ASessionLandsOnOneBackendEveryTurn(t *testing.T) {
	ctx := context.Background()
	repo := stickyRepo(t, 2)
	system := modelrepo.Message{Role: "system", Content: "be terse"}

	key := sessionKeyFor(testKey("k1"), nil, "sticky-model", "", []modelrepo.Message{system, user("plan a trip")})
	require.NotEmpty(t, key)

	landed := map[string]bool{}
	for i := range 8 {
		req := llmrepo.Request{ModelNames: []string{"sticky-model"}, SessionKey: key}
		res, meta, err := repo.Chat(ctx, req, []modelrepo.Message{system, user("plan a trip"), assistant("sure"), user(fmt.Sprintf("turn %d", i))})
		require.NoError(t, err)
		require.NotEmpty(t, meta.BackendID)
		require.NotEmpty(t, res.Message.Content)
		landed[meta.BackendID] = true
	}

	require.Len(t, landed, 1, "one session, one backend; it landed on %v", landed)
}

// The contrast that makes the assertion mean something: without a session the
// same turns spread across the deployment, which is what gateway traffic did
// before the key was derived.
func TestSystem_WithoutASessionTurnsSpreadAcrossBackends(t *testing.T) {
	ctx := context.Background()
	repo := stickyRepo(t, 2)
	system := modelrepo.Message{Role: "system", Content: "be terse"}

	landed := map[string]bool{}
	for i := range 20 {
		res, meta, err := repo.Chat(ctx,
			llmrepo.Request{ModelNames: []string{"sticky-model"}},
			[]modelrepo.Message{system, user(fmt.Sprintf("unrelated question %d", i))})
		require.NoError(t, err)
		require.NotEmpty(t, res.Message.Content)
		landed[meta.BackendID] = true
	}

	require.Len(t, landed, 2, "unpinned turns reach more than one backend; they landed on %v", landed)
}
