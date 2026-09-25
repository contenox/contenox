package gitservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func testSignature() *object.Signature {
	return &object.Signature{
		Name:  "Test Author",
		Email: "test@example.com",
		When:  time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
	}
}

func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test Author"
	cfg.User.Email = "test@example.com"
	require.NoError(t, repo.SetConfig(cfg))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	for name, content := range files {
		writeFile(t, dir, name, content)
		_, err := wt.Add(name)
		require.NoError(t, err)
	}
	_, err = wt.Commit("initial commit", &git.CommitOptions{Author: testSignature()})
	require.NoError(t, err)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func codeFor(t *testing.T, entries []StatusEntry, path string) string {
	t.Helper()
	for _, e := range entries {
		if e.Path == path {
			return e.Code
		}
	}
	return ""
}

func TestService_NotRepo(t *testing.T) {
	svc := New(t.TempDir())
	_, err := svc.Status(context.Background())
	require.ErrorIs(t, err, ErrNotRepo)
}

func TestService_StatusDiffBlameStageCommit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "hello\n"})

	svc := New(dir)

	st, err := svc.Status(ctx)
	require.NoError(t, err)
	require.True(t, st.Clean)
	require.Equal(t, "master", st.Branch)
	require.NotNil(t, st.Head)

	writeFile(t, dir, "a.txt", "hello modified\n")
	writeFile(t, dir, "b.txt", "new\n")

	st, err = svc.Status(ctx)
	require.NoError(t, err)
	require.False(t, st.Clean)
	require.Equal(t, "M", codeFor(t, st.Unstaged, "a.txt"))
	require.Equal(t, "?", codeFor(t, st.Files, "b.txt"))
	require.Contains(t, st.Untracked, "b.txt")

	diff, err := svc.Diff(ctx, "a.txt")
	require.NoError(t, err)
	require.Equal(t, "hello\n", diff.Old)
	require.Equal(t, "hello modified\n", diff.New)
	require.False(t, diff.IsNew)

	diff, err = svc.Diff(ctx, "b.txt")
	require.NoError(t, err)
	require.True(t, diff.IsNew)
	require.Empty(t, diff.Old)

	blame, err := svc.Blame(ctx, "a.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blame.Lines)
	require.Equal(t, "Test Author", blame.Lines[0].Author)

	require.NoError(t, svc.Stage(ctx, []string{"a.txt"}))
	st, err = svc.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "M", codeFor(t, st.Staged, "a.txt"), "a modified tracked file stages as Modified")

	commit, err := svc.Commit(ctx, "second commit")
	require.NoError(t, err)
	require.Equal(t, "second commit", commit.Subject)
	require.Equal(t, 1, commit.Files)

	st, err = svc.Status(ctx)
	require.NoError(t, err)
	require.False(t, st.Clean, "b.txt is still untracked after the commit")
	require.Contains(t, st.Untracked, "b.txt")
}
