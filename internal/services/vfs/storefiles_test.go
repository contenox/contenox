package vfs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const testTenant = "00000000-0000-0000-0000-000000000001"

// newFileID inserts a minimal vfs_files record and returns its ID.
// Required by tree tests to satisfy vfs_filestree.id → vfs_files.id FK.
func newFileID(t *testing.T, s fileStore, ctx context.Context) string {
	t.Helper()
	id := uuid.NewString()
	err := s.CreateFile(ctx, testTenant, &fileRow{
		ID:   id,
		Type: "text/plain",
		Meta: []byte(`{}`),
	})
	require.NoError(t, err)
	return id
}

func TestUnit_CreateAndGetFile(t *testing.T) {
	ctx, s := setupStore(t)

	// Create the blob first to satisfy the FK constraint.
	blob := &blobRow{
		ID:   uuid.NewString(),
		Meta: []byte(`{}`),
		Data: []byte("hello"),
	}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))

	file := &fileRow{
		ID:      uuid.NewString(),
		Type:    "text/plain",
		Meta:    []byte(`{"description": "Test file"}`),
		BlobsID: blob.ID,
	}

	err := s.CreateFile(ctx, testTenant, file)
	require.NoError(t, err)
	require.NotZero(t, file.CreatedAt)
	require.NotZero(t, file.UpdatedAt)

	retrieved, err := s.GetFileByID(ctx, testTenant, file.ID)
	require.NoError(t, err)
	require.Equal(t, file.ID, retrieved.ID)
	require.Equal(t, file.Type, retrieved.Type)
	require.Equal(t, file.Meta, retrieved.Meta)
	require.Equal(t, file.BlobsID, retrieved.BlobsID)
	require.WithinDuration(t, file.CreatedAt, retrieved.CreatedAt, time.Second)
	require.WithinDuration(t, file.UpdatedAt, retrieved.UpdatedAt, time.Second)
}

func TestUnit_UpdateFile(t *testing.T) {
	ctx, s := setupStore(t)

	blob := &blobRow{ID: uuid.NewString(), Meta: []byte(`{}`), Data: []byte("v1")}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))

	file := &fileRow{
		ID:      uuid.NewString(),
		Type:    "text/plain",
		Meta:    []byte(`{"description": "Old description"}`),
		BlobsID: blob.ID,
	}
	require.NoError(t, s.CreateFile(ctx, testTenant, file))

	blob2 := &blobRow{ID: uuid.NewString(), Meta: []byte(`{}`), Data: []byte("v2")}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob2))

	file.Type = "application/json"
	file.Meta = []byte(`{"description": "New description"}`)
	file.BlobsID = blob2.ID
	require.NoError(t, s.UpdateFile(ctx, testTenant, file))

	updated, err := s.GetFileByID(ctx, testTenant, file.ID)
	require.NoError(t, err)
	require.Equal(t, "application/json", updated.Type)
	require.Equal(t, file.Meta, updated.Meta)
	require.Equal(t, blob2.ID, updated.BlobsID)
	require.True(t, updated.UpdatedAt.After(updated.CreatedAt))
}

func TestUnit_DeleteFile(t *testing.T) {
	ctx, s := setupStore(t)

	blob := &blobRow{ID: uuid.NewString(), Meta: []byte(`{}`), Data: []byte("data")}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))

	file := &fileRow{
		ID:      uuid.NewString(),
		Type:    "text/plain",
		Meta:    []byte(`{"description": "To be deleted"}`),
		BlobsID: blob.ID,
	}
	require.NoError(t, s.CreateFile(ctx, testTenant, file))
	require.NoError(t, s.DeleteFile(ctx, testTenant, file.ID))

	_, err := s.GetFileByID(ctx, testTenant, file.ID)
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_GetFileByIDNotFound(t *testing.T) {
	ctx, s := setupStore(t)

	_, err := s.GetFileByID(ctx, testTenant, uuid.NewString())
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_ListAll(t *testing.T) {
	ctx, s := setupStore(t)

	files, err := s.ListFiles(ctx, testTenant, Page{})
	require.NoError(t, err)
	require.Len(t, files, 0)

	for i := 0; i < 3; i++ {
		// Files may have no blob (blobs_id is nullable) — omit BlobsID to avoid FK violation.
		require.NoError(t, s.CreateFile(ctx, testTenant, &fileRow{
			ID:   uuid.NewString(),
			Type: "text/plain",
			Meta: []byte(`{}`),
		}))
	}

	files, err = s.ListFiles(ctx, testTenant, Page{})
	require.NoError(t, err)
	require.Len(t, files, 3)
}

func TestUnit_CreateAndGetFileNameID(t *testing.T) {
	ctx, s := setupStore(t)

	id := newFileID(t, s, ctx)
	parentID := newFileID(t, s, ctx)
	name := "example.txt"

	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, parentID, name))

	gotName, err := s.GetFileNameByID(ctx, testTenant, id)
	require.NoError(t, err)
	require.Equal(t, name, gotName)

	gotParentID, err := s.GetFileParentID(ctx, testTenant, id)
	require.NoError(t, err)
	require.Equal(t, parentID, gotParentID)
}

func TestUnit_UpdateFileNameByID(t *testing.T) {
	ctx, s := setupStore(t)

	id := newFileID(t, s, ctx)
	parentID := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, parentID, "initial.txt"))

	require.NoError(t, s.UpdateFileNameByID(ctx, testTenant, id, "updated.txt"))

	gotName, err := s.GetFileNameByID(ctx, testTenant, id)
	require.NoError(t, err)
	require.Equal(t, "updated.txt", gotName)
}

func TestUnit_DeleteFileNameID(t *testing.T) {
	ctx, s := setupStore(t)

	id := newFileID(t, s, ctx)
	parentID := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, parentID, "todelete.txt"))
	require.NoError(t, s.DeleteFileNameID(ctx, testTenant, id))

	_, err := s.GetFileNameByID(ctx, testTenant, id)
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_HasChildren(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	has, err := s.HasChildren(ctx, testTenant, parentID)
	require.NoError(t, err)
	require.False(t, has)

	child := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, child, parentID, "a.txt"))

	has, err = s.HasChildren(ctx, testTenant, parentID)
	require.NoError(t, err)
	require.True(t, has)
}

func TestUnit_HasChildren_RootScope(t *testing.T) {
	ctx, s := setupStore(t)

	has, err := s.HasChildren(ctx, testTenant, "")
	require.NoError(t, err)
	require.False(t, has)

	id := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, "", "a.txt"))

	has, err = s.HasChildren(ctx, testTenant, "")
	require.NoError(t, err)
	require.True(t, has)
}

func TestUnit_FindFileIDByName(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	id := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, parentID, "unique.txt"))

	got, err := s.FindFileIDByName(ctx, testTenant, parentID, "unique.txt")
	require.NoError(t, err)
	require.Equal(t, id, got)

	_, err = s.FindFileIDByName(ctx, testTenant, parentID, "missing.txt")
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

// childNamed creates a vfs_files row and a tree entry named name under parentID.
func childNamed(t *testing.T, s fileStore, ctx context.Context, parentID, name string) string {
	t.Helper()
	id := newFileID(t, s, ctx)
	require.NoError(t, s.CreateFileNameID(ctx, testTenant, id, parentID, name))
	return id
}

func TestUnit_ListChildren_NamePaginationWalksAllPages(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	names := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
	for _, n := range names {
		childNamed(t, s, ctx, parentID, n)
	}

	var got []string
	var after *Cursor
	for {
		page := Page{SortBy: SortByName, Limit: 2, After: after}
		children, err := s.ListChildrenByParentID(ctx, testTenant, parentID, page)
		require.NoError(t, err)
		if len(children) == 0 {
			break
		}
		for _, c := range children {
			got = append(got, c.Name)
		}
		if len(children) < 2 {
			break
		}
		last := children[len(children)-1]
		after = &Cursor{CreatedAt: last.CreatedAt, Name: last.Name, ID: last.ID}
	}
	// Ascending, no gaps, no duplicates across pages.
	require.Equal(t, names, got)
}

func TestUnit_ListChildren_NameDescending(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		childNamed(t, s, ctx, parentID, n)
	}

	children, err := s.ListChildrenByParentID(ctx, testTenant, parentID,
		Page{SortBy: SortByName, Desc: true, Limit: 10})
	require.NoError(t, err)

	var names []string
	for _, c := range children {
		names = append(names, c.Name)
	}
	require.Equal(t, []string{"c.txt", "b.txt", "a.txt"}, names)
}

func TestUnit_ListChildren_CreatedAtKeysetNoGapsNoDupes(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	want := map[string]bool{}
	for i := 0; i < 5; i++ {
		want[childNamed(t, s, ctx, parentID, fmt.Sprintf("f%d.txt", i))] = true
	}

	got := map[string]bool{}
	var after *Cursor
	for {
		page := Page{Limit: 2, After: after} // default: created_at
		children, err := s.ListChildrenByParentID(ctx, testTenant, parentID, page)
		require.NoError(t, err)
		if len(children) == 0 {
			break
		}
		for _, c := range children {
			require.False(t, got[c.ID], "duplicate id %s across pages", c.ID)
			got[c.ID] = true
		}
		if len(children) < 2 {
			break
		}
		last := children[len(children)-1]
		after = &Cursor{CreatedAt: last.CreatedAt, Name: last.Name, ID: last.ID}
	}
	require.Equal(t, want, got)
}

func TestUnit_ListChildren_LimitExceeded(t *testing.T) {
	ctx, s := setupStore(t)

	parentID := newFileID(t, s, ctx)
	_, err := s.ListChildrenByParentID(ctx, testTenant, parentID,
		Page{Limit: runtimetypes.MAXLIMIT + 1})
	require.ErrorIs(t, err, runtimetypes.ErrLimitParamExceeded)
}

func TestUnit_Blob_CreatesAndFetchesByID(t *testing.T) {
	ctx, s := setupStore(t)

	blob := &blobRow{
		ID:   uuid.NewString(),
		Meta: []byte(`{"description": "Test blob"}`),
		Data: []byte("binary data"),
	}

	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))
	require.NotZero(t, blob.CreatedAt)
	require.NotZero(t, blob.UpdatedAt)

	retrieved, err := s.GetBlobByID(ctx, testTenant, blob.ID)
	require.NoError(t, err)
	require.Equal(t, blob.ID, retrieved.ID)
	require.Equal(t, blob.Meta, retrieved.Meta)
	require.Equal(t, blob.Data, retrieved.Data)
	require.WithinDuration(t, blob.CreatedAt, retrieved.CreatedAt, time.Second)
}

func TestUnit_Blob_GetNonexistentReturnsNotFound(t *testing.T) {
	ctx, s := setupStore(t)

	_, err := s.GetBlobByID(ctx, testTenant, uuid.NewString())
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_Blob_DeletesSuccessfully(t *testing.T) {
	ctx, s := setupStore(t)

	blob := &blobRow{
		ID:   uuid.NewString(),
		Meta: []byte(`{}`),
		Data: []byte("data"),
	}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))
	require.NoError(t, s.DeleteBlob(ctx, testTenant, blob.ID))

	_, err := s.GetBlobByID(ctx, testTenant, blob.ID)
	require.ErrorIs(t, err, libdb.ErrNotFound)
}

func TestUnit_UpdateBlob(t *testing.T) {
	ctx, s := setupStore(t)

	blob := &blobRow{
		ID:   uuid.NewString(),
		Meta: []byte(`{"v":1}`),
		Data: []byte("original"),
	}
	require.NoError(t, s.CreateBlob(ctx, testTenant, blob))

	require.NoError(t, s.UpdateBlob(ctx, testTenant, blob.ID, []byte("updated"), []byte(`{"v": 2}`)))

	got, err := s.GetBlobByID(ctx, testTenant, blob.ID)
	require.NoError(t, err)
	require.Equal(t, []byte("updated"), got.Data)
	require.Equal(t, []byte(`{"v": 2}`), got.Meta)
}
