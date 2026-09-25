package vfs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

const testTenant = runtimetypes.LocalTenantID

func quiet() func() {
	null, _ := os.Open(os.DevNull)
	sout := os.Stdout
	serr := os.Stderr
	os.Stdout = null
	os.Stderr = null
	return func() {
		defer null.Close()
		os.Stdout = sout
		os.Stderr = serr
	}
}

// setupService returns a DB-backed Service over a fresh database. It does NOT
// create the schema itself: that is the constructor's job, so every test below
// also proves the constructor did it.
func setupService(t *testing.T, cb vfs.Callbacks) (context.Context, vfs.Service) {
	t.Helper()

	ctx := context.TODO()
	dbManager, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "vfs.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dbManager.Close()) })

	svc, err := vfs.New(ctx, dbManager, cb)
	require.NoError(t, err)
	return ctx, svc
}

// TestUnit_NewCreatesItsOwnSchema pins the constructor's contract: a caller that
// does nothing but construct the service gets working tables, because the
// alternative — a missing table discovered mid-request — is the failure this
// exists to prevent.
func TestUnit_NewCreatesItsOwnSchema(t *testing.T) {
	ctx := context.TODO()
	dbManager, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "fresh.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dbManager.Close()) })

	// Nothing has created the vfs tables on this database yet.
	var before int
	require.NoError(t, dbManager.WithoutTransaction().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('vfs_files','vfs_filestree','vfs_blobs')`).Scan(&before))
	require.Zero(t, before, "the fixture must start without vfs tables for this to prove anything")

	svc, err := vfs.New(ctx, dbManager, vfs.Callbacks{})
	require.NoError(t, err)

	var after int
	require.NoError(t, dbManager.WithoutTransaction().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('vfs_files','vfs_filestree','vfs_blobs')`).Scan(&after))
	require.Equal(t, 3, after, "constructing the service must create all three tables")

	// And it is usable straight away, which is the point of doing it there.
	_, err = svc.CreateFolder(ctx, testTenant, "", "documents")
	require.NoError(t, err)

	// A nil database is a caller error rather than a panic on first use.
	_, err = vfs.New(ctx, nil, vfs.Callbacks{})
	require.Error(t, err)
}

func TestUnit_CreateFile_SatisfiesTreeFileFK(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})

	created, err := svc.CreateFile(ctx, testTenant, &vfs.File{
		Name:        "hello.txt",
		ContentType: "text/plain",
		Data:        []byte("hello"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	got, err := svc.GetFileByID(ctx, testTenant, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "hello.txt", got.Name)
	require.Equal(t, []byte("hello"), got.Data)
}

func TestUnit_CreateFolder_SatisfiesTreeFileFK(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})

	folder, err := svc.CreateFolder(ctx, testTenant, "", "docs")
	require.NoError(t, err)
	require.NotEmpty(t, folder.ID)
	require.Equal(t, "docs", folder.Name)

	got, err := svc.GetFolderByID(ctx, testTenant, folder.ID)
	require.NoError(t, err)
	require.Equal(t, folder.ID, got.ID)
	require.Equal(t, "docs", got.Name)
}

func TestUnit_CreateFile_InvokesOnCreate(t *testing.T) {
	var gotTenant string
	var gotFile *vfs.File
	calls := 0
	cb := vfs.Callbacks{
		OnCreate: func(_ context.Context, tenantID string, file *vfs.File) error {
			calls++
			gotTenant = tenantID
			gotFile = file
			return nil
		},
	}
	ctx, svc := setupService(t, cb)

	created, err := svc.CreateFile(ctx, testTenant, &vfs.File{
		Name:        "hello.txt",
		ContentType: "text/plain",
		Data:        []byte("hello"),
	})
	require.NoError(t, err)

	require.Equal(t, 1, calls)
	require.Equal(t, testTenant, gotTenant)
	require.NotNil(t, gotFile)
	require.Equal(t, created.ID, gotFile.ID)
}

func TestUnit_CreateFile_OnCreateError_RoutedToOnError(t *testing.T) {
	hookErr := errors.New("ownership grant failed")
	var (
		gotOp, gotID string
		gotErr       error
		errCalls     int
	)
	cb := vfs.Callbacks{
		OnCreate: func(context.Context, string, *vfs.File) error {
			return hookErr
		},
		OnError: func(_ context.Context, _ /*tenantID*/, op, resourceID string, err error) {
			errCalls++
			gotOp, gotID, gotErr = op, resourceID, err
		},
	}
	ctx, svc := setupService(t, cb)

	// The post-commit hook error must not fail the operation: the file is committed.
	created, err := svc.CreateFile(ctx, testTenant, &vfs.File{
		Name:        "hello.txt",
		ContentType: "text/plain",
		Data:        []byte("hello"),
	})
	require.NoError(t, err)

	got, err := svc.GetFileByID(ctx, testTenant, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	// The error is surfaced to OnError rather than swallowed.
	require.Equal(t, 1, errCalls)
	require.Equal(t, "create", gotOp)
	require.Equal(t, created.ID, gotID)
	require.ErrorIs(t, gotErr, hookErr)
}

func TestUnit_CreateFolder_InvokesOnCreate(t *testing.T) {
	var gotTenant string
	var gotFile *vfs.File
	calls := 0
	cb := vfs.Callbacks{
		OnCreate: func(_ context.Context, tenantID string, file *vfs.File) error {
			calls++
			gotTenant = tenantID
			gotFile = file
			return nil
		},
	}
	ctx, svc := setupService(t, cb)

	folder, err := svc.CreateFolder(ctx, testTenant, "", "docs")
	require.NoError(t, err)

	require.Equal(t, 1, calls)
	require.Equal(t, testTenant, gotTenant)
	require.NotNil(t, gotFile)
	require.Equal(t, folder.ID, gotFile.ID)
	require.Equal(t, "docs", gotFile.Name)
}

func TestUnit_GetFilesByPath_PaginatesRoot(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})

	names := []string{"a.json", "b.json", "c.json", "d.json", "e.json"}
	for _, n := range names {
		_, err := svc.CreateFile(ctx, testTenant, &vfs.File{
			Name: n, ContentType: "application/json", Data: []byte("{}"),
		})
		require.NoError(t, err)
	}

	var got []string
	page := vfs.Page{SortBy: vfs.SortByName, Limit: 2}
	pages := 0
	for {
		files, next, err := svc.GetFilesByPath(ctx, testTenant, "", page)
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, pages, 10, "pagination did not terminate")
		for _, f := range files {
			got = append(got, f.Name)
		}
		if next == nil {
			break
		}
		page.After = next
	}
	require.Equal(t, names, got)
}

func TestUnit_MoveFolder_CyclicIsErrInvalidMove(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})

	parent, err := svc.CreateFolder(ctx, testTenant, "", "parent")
	require.NoError(t, err)
	child, err := svc.CreateFolder(ctx, testTenant, parent.ID, "child")
	require.NoError(t, err)

	// Into itself.
	_, err = svc.MoveFolder(ctx, testTenant, parent.ID, parent.ID)
	require.ErrorIs(t, err, vfs.ErrInvalidMove)

	// Into one of its own descendants.
	_, err = svc.MoveFolder(ctx, testTenant, parent.ID, child.ID)
	require.ErrorIs(t, err, vfs.ErrInvalidMove)
}

// TestUnit_File_MetadataRoundTrips locks the KV metadata channel: it persists on
// create, surfaces on GetFileByID and on a GetFilesByPath listing, and UpdateFile
// replaces it wholesale (independent of content).
func TestUnit_File_MetadataRoundTrips(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})

	created, err := svc.CreateFile(ctx, testTenant, &vfs.File{
		Name:        "doc.md",
		ContentType: "text/markdown",
		Data:        []byte("# body"),
		Metadata:    map[string]string{"connector.sourceVersion": "etag-1", "title": "Doc"},
	})
	require.NoError(t, err)

	got, err := svc.GetFileByID(ctx, testTenant, created.ID)
	require.NoError(t, err)
	require.Equal(t, "etag-1", got.Metadata["connector.sourceVersion"])
	require.Equal(t, "Doc", got.Metadata["title"])

	// A listing surfaces the same metadata without a per-file blob read.
	listed, _, err := svc.GetFilesByPath(ctx, testTenant, "", vfs.Page{Limit: 100})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "etag-1", listed[0].Metadata["connector.sourceVersion"])

	// Update replaces metadata wholesale — the old keys are gone, not merged.
	_, err = svc.UpdateFile(ctx, testTenant, &vfs.File{
		ID:          created.ID,
		ContentType: "text/markdown",
		Data:        []byte("# body changed"),
		Metadata:    map[string]string{"connector.sourceVersion": "etag-2"},
	})
	require.NoError(t, err)

	got, err = svc.GetFileByID(ctx, testTenant, created.ID)
	require.NoError(t, err)
	require.Equal(t, "etag-2", got.Metadata["connector.sourceVersion"])
	require.NotContains(t, got.Metadata, "title", "UpdateFile replaces metadata, not merges")
}

// TestUnit_File_NoMetadataIsNil confirms files written without metadata read
// back with a nil map (existing rows decode cleanly).
func TestUnit_File_NoMetadataIsNil(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	created, err := svc.CreateFile(ctx, testTenant, &vfs.File{
		Name: "plain.txt", ContentType: "text/plain", Data: []byte("x"),
	})
	require.NoError(t, err)
	got, err := svc.GetFileByID(ctx, testTenant, created.ID)
	require.NoError(t, err)
	require.Nil(t, got.Metadata)
}
