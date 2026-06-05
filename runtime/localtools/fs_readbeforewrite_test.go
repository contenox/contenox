package localtools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func setupFSReadGuard(t *testing.T) (context.Context, taskengine.ToolsRepo, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "guard.db")
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	allowedDir := t.TempDir()
	tools := localtools.NewLocalFSTools(allowedDir, db)
	ctxWithSession := context.WithValue(ctx, runtimetypes.SessionIDContextKey, "test-session")
	return ctxWithSession, tools, allowedDir
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

func execTool(t *testing.T, ctx context.Context, h taskengine.ToolsRepo, name string, args map[string]any) (any, error) {
	t.Helper()
	res, _, err := h.Exec(ctx, time.Now(), args, false, &taskengine.ToolsCall{ToolName: name})
	return res, err
}

func TestUnit_ReadBeforeWrite_AllowedAfterRead(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)
	writeFile(t, dir, "a.txt", "original")

	_, err := execTool(t, ctx, tools, "read_file", map[string]any{"path": "a.txt"})
	require.NoError(t, err)

	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": "a.txt", "content": "updated"})
	require.NoError(t, err)
	fw, ok := res.(localtools.FsWriteResult)
	require.True(t, ok, "expected FsWriteResult, got %T", res)
	require.True(t, fw.Written)
	require.Equal(t, "original", fw.OldText)
	require.Equal(t, "updated", fw.NewText)

	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "updated", string(got))
}

func TestUnit_ReadBeforeWrite_DeniedWithoutRead(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)
	writeFile(t, dir, "a.txt", "original")

	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": "a.txt", "content": "updated"})
	require.NoError(t, err, "denial must be a soft string result, not a chain error")
	msg, ok := res.(string)
	require.True(t, ok, "denial result must be string, got %T", res)
	require.Contains(t, msg, "read_file")
	require.Contains(t, msg, "without reading it first")

	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "original", string(got), "file must not have been mutated when denied")
}

func TestUnit_ReadBeforeWrite_NewFileAllowed(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)

	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": "new.txt", "content": "fresh"})
	require.NoError(t, err)
	fw, ok := res.(localtools.FsWriteResult)
	require.True(t, ok, "creating a new file should not require a prior read; got %T", res)
	require.True(t, fw.Written)
	require.Empty(t, fw.OldText)
	require.Equal(t, "fresh", fw.NewText)

	got, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "fresh", string(got))
}

func TestUnit_ReadBeforeWrite_SedDeniedWithoutRead(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)
	writeFile(t, dir, "a.txt", "alpha bravo")

	res, err := execTool(t, ctx, tools, "sed", map[string]any{
		"path":        "a.txt",
		"pattern":     "alpha",
		"replacement": "ALPHA",
	})
	require.NoError(t, err)
	msg, ok := res.(string)
	require.True(t, ok)
	require.Contains(t, msg, "read_file")

	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "alpha bravo", string(got), "sed must not have run when denied")
}

func TestUnit_ReadBeforeWrite_SedAllowedAfterRangeRead(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)
	writeFile(t, dir, "a.txt", "alpha\nbravo\ncharlie")

	_, err := execTool(t, ctx, tools, "read_file_range", map[string]any{
		"path":       "a.txt",
		"start_line": float64(1),
		"end_line":   float64(2),
	})
	require.NoError(t, err)

	res, err := execTool(t, ctx, tools, "sed", map[string]any{
		"path":        "a.txt",
		"pattern":     "alpha",
		"replacement": "ALPHA",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", res, "read_file_range must satisfy the read-before-write contract")

	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.Contains(t, string(got), "ALPHA")
}

func TestUnit_ReadBeforeWrite_BypassWithoutSession(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "guard.db")
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "original")
	tools := localtools.NewLocalFSTools(dir, db)

	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": "a.txt", "content": "updated"})
	require.NoError(t, err)
	fw, ok := res.(localtools.FsWriteResult)
	require.True(t, ok, "without a session ID the guard must fall open; got %T", res)
	require.True(t, fw.Written)
}

func TestUnit_ReadBeforeWrite_NilDBBypasses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "original")

	tools := localtools.NewLocalFSTools(dir, nil)
	ctx := context.WithValue(context.Background(), runtimetypes.SessionIDContextKey, "irrelevant")

	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": "a.txt", "content": "updated"})
	require.NoError(t, err)
	fw, ok := res.(localtools.FsWriteResult)
	require.True(t, ok, "nil db must disable the guard; got %T", res)
	require.True(t, fw.Written)
}

func TestUnit_ReadBeforeWrite_PathNormalization(t *testing.T) {
	ctx, tools, dir := setupFSReadGuard(t)
	writeFile(t, dir, "a.txt", "original")

	_, err := execTool(t, ctx, tools, "read_file", map[string]any{"path": "a.txt"})
	require.NoError(t, err)

	absPath := filepath.Join(dir, "a.txt")
	res, err := execTool(t, ctx, tools, "write_file", map[string]any{"path": absPath, "content": "updated"})
	require.NoError(t, err)
	fw, ok := res.(localtools.FsWriteResult)
	require.True(t, ok, "absolute and relative paths must canonicalize to the same key; got %T", res)
	require.True(t, fw.Written)
}

func TestUnit_ReadBeforeWrite_SessionScoping(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "guard.db")
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "original")
	tools := localtools.NewLocalFSTools(dir, db)

	ctxA := context.WithValue(ctx, runtimetypes.SessionIDContextKey, "session-A")
	_, err = execTool(t, ctxA, tools, "read_file", map[string]any{"path": "a.txt"})
	require.NoError(t, err)

	ctxB := context.WithValue(ctx, runtimetypes.SessionIDContextKey, "session-B")
	res, err := execTool(t, ctxB, tools, "write_file", map[string]any{"path": "a.txt", "content": "updated"})
	require.NoError(t, err)
	msg, _ := res.(string)
	require.Contains(t, msg, "without reading it first", "a read in session A must not satisfy a write in session B")
}
