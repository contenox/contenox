package vfs_test

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stored handle is the half of the Files seam a database-backed deployment
// runs on, so it must carry the contract the on-disk handle does: the same
// verbs, the same containment, and a missing entry that reports fs.ErrNotExist
// rather than a store sentinel. Without it a declaration can only be compiled
// from a filesystem.
func TestUnit_StoredIO_CarriesTheFilesContract(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	root := vfs.Stored(svc, testTenant)

	require.NoError(t, root.MkdirAll(ctx, "nested/deeper"))
	require.NoError(t, root.WriteFile(ctx, "nested/deeper/note.txt", []byte("hello")))

	got, err := root.ReadFile(ctx, "nested/deeper/note.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))

	info, err := root.Stat(ctx, "nested/deeper/note.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(5), info.Size())
	assert.Equal(t, "note.txt", info.Name())
	assert.False(t, info.IsDir())

	dir, err := root.Stat(ctx, "nested/deeper")
	require.NoError(t, err)
	assert.True(t, dir.IsDir())

	entries, err := root.ReadDir(ctx, "nested")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "deeper", entries[0].Name())
	assert.True(t, entries[0].IsDir())

	// The root lists what was written under it, and "." names the same place as
	// "" and "/" do.
	for _, name := range []string{"", ".", "/"} {
		top, err := root.ReadDir(ctx, name)
		require.NoError(t, err)
		require.Len(t, top, 1, "root read with %q", name)
		assert.Equal(t, "nested", top[0].Name())
	}

	// A folder holding a child is not removable, the same as os.Remove.
	require.ErrorIs(t, root.Remove(ctx, "nested/deeper"), vfs.ErrFolderNotEmpty)

	require.NoError(t, root.Remove(ctx, "nested/deeper/note.txt"))
	_, err = root.ReadFile(ctx, "nested/deeper/note.txt")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "a missing entry must satisfy os.IsNotExist, got %v", err)

	require.NoError(t, root.Remove(ctx, "nested/deeper"))
	require.NoError(t, root.Remove(ctx, "nested"))
}

func TestUnit_StoredIO_ContainsEveryVerb(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	root := vfs.Stored(svc, testTenant)

	for _, name := range []string{"../secret.txt", "nested/../../secret.txt", "a/b/../../../c"} {
		t.Run(name, func(t *testing.T) {
			_, err := root.ReadFile(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			assert.ErrorIs(t, root.WriteFile(ctx, name, []byte("written")), vfs.ErrEscape)

			_, err = root.Stat(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			_, err = root.ReadDir(ctx, name)
			assert.ErrorIs(t, err, vfs.ErrEscape)

			assert.ErrorIs(t, root.Remove(ctx, name), vfs.ErrEscape)

			assert.ErrorIs(t, root.MkdirAll(ctx, name), vfs.ErrEscape)

			_, err = root.Sub(name)
			assert.ErrorIs(t, err, vfs.ErrEscape)
		})
	}

	// Every refusal left the tenant empty: nothing was written under a name that
	// tried to leave the root.
	entries, err := root.ReadDir(ctx, ".")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Sub is a name, not a table: a handle over a directory that does not exist yet
// still writes, and creating that directory creates every folder above it.
func TestUnit_StoredIO_SubCreatesItsPrefixOnWrite(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	tenant := vfs.Stored(svc, testTenant)

	contenox, err := tenant.Sub(".contenox")
	require.NoError(t, err)
	generated, err := contenox.Sub(".generated")
	require.NoError(t, err)

	// Reading through a handle whose folders do not exist is a not-found, not a
	// failure.
	_, err = generated.ReadFile(ctx, "chain.json")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	require.NoError(t, generated.MkdirAll(ctx, "."))
	require.NoError(t, generated.WriteFile(ctx, "chain.json", []byte("{}")))

	// The same bytes are reachable from the tenant root by their full path, so a
	// sub-handle is a view of one tree rather than a second one.
	got, err := tenant.ReadFile(ctx, ".contenox/.generated/chain.json")
	require.NoError(t, err)
	assert.Equal(t, "{}", string(got))

	entries, err := contenox.ReadDir(ctx, ".")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, ".generated", entries[0].Name())
}

func TestUnit_StoredIO_WalksInLexicalOrderAndSkips(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	root := vfs.Stored(svc, testTenant)

	for _, name := range []string{"b/two.md", "a/one.md", "a/nested/three.md", "top.md"} {
		require.NoError(t, root.MkdirAll(ctx, path.Dir(name)))
		require.NoError(t, root.WriteFile(ctx, name, []byte(name)))
	}

	var walked []string
	require.NoError(t, root.WalkDir(ctx, ".", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		walked = append(walked, p)
		return nil
	}))
	assert.Equal(t, []string{
		".", "a", "a/nested", "a/nested/three.md", "a/one.md", "b", "b/two.md", "top.md",
	}, walked)

	// A directory returning fs.SkipDir takes its contents with it and leaves its
	// siblings alone, which is what the walk contract promises.
	walked = nil
	require.NoError(t, root.WalkDir(ctx, ".", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		walked = append(walked, p)
		if p == "a" {
			return fs.SkipDir
		}
		return nil
	}))
	assert.Equal(t, []string{".", "a", "b", "b/two.md", "top.md"}, walked)
}

// A second write replaces the content rather than adding a sibling: a compiled
// chain is rewritten in place, and a store that accumulated versions would grow
// without bound.
func TestUnit_StoredIO_WriteReplacesContent(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	root := vfs.Stored(svc, testTenant)

	require.NoError(t, root.MkdirAll(ctx, "."))
	require.NoError(t, root.WriteFile(ctx, "chain.json", []byte(`{"id":"one"}`)))
	require.NoError(t, root.WriteFile(ctx, "chain.json", []byte(`{"id":"two"}`)))

	got, err := root.ReadFile(ctx, "chain.json")
	require.NoError(t, err)
	assert.Equal(t, `{"id":"two"}`, string(got))

	entries, err := root.ReadDir(ctx, ".")
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	info, err := root.Stat(ctx, "chain.json")
	require.NoError(t, err)
	assert.Equal(t, int64(len(`{"id":"two"}`)), info.Size())
}

// Writing a file into a directory that does not exist reports not-found on the
// containing path, mirroring os.WriteFile: the caller creates folders first, so
// a typo in a directory name is not silently turned into a tree.
func TestUnit_StoredIO_WriteDoesNotCreateMissingFolders(t *testing.T) {
	ctx, svc := setupService(t, vfs.Callbacks{})
	root := vfs.Stored(svc, testTenant)

	err := root.WriteFile(ctx, "absent/chain.json", []byte("{}"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "a missing containing folder must report not-found, got %v", err)

	entries, err := root.ReadDir(ctx, ".")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// An access-list-backed store decides every call against the context's actor,
// which is why Stored documents that a server-side caller stamps one, and what
// makes a tenant's files invisible to another identity in it.
func TestUnit_StoredIO_AccessListDecidesAgainstTheContextActor(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "storedacl.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	acl, err := vfs.NewACL(ctx, db)
	require.NoError(t, err)
	svc, err := vfs.New(ctx, db, acl.Callbacks())
	require.NoError(t, err)

	owner := vfs.Actor{TenantID: testTenant, UserID: "user-1"}
	stranger := vfs.Actor{TenantID: testTenant, UserID: "user-2"}
	acting := vfs.WithActor(ctx, owner)

	root := vfs.Stored(svc, testTenant)
	require.NoError(t, root.MkdirAll(acting, "."))
	require.NoError(t, root.WriteFile(acting, "note.md", []byte("hi")))

	// The creator's grant is installed on create, so the owner reads what the
	// owner wrote.
	got, err := root.ReadFile(acting, "note.md")
	require.NoError(t, err)
	assert.Equal(t, "hi", string(got))

	// Another identity in the same tenant holds no grant, and neither does a
	// call carrying no actor at all.
	_, err = root.ReadFile(vfs.WithActor(ctx, stranger), "note.md")
	require.Error(t, err)
	_, err = root.ReadFile(ctx, "note.md")
	require.Error(t, err)

	info, err := root.Stat(acting, "note.md")
	require.NoError(t, err)
	assert.Equal(t, "note.md", info.Name())

	// A handle built for one actor carries it, so a caller that holds the
	// authority once does not restamp it on every call — and a sub-handle keeps
	// it, which is what lets a compiler hand one out without also handing out
	// the authority to read another tenant.
	bound := vfs.AsActor(vfs.Stored(svc, testTenant), owner)
	got, err = bound.ReadFile(context.Background(), "note.md")
	require.NoError(t, err)
	assert.Equal(t, "hi", string(got))

	nested, err := bound.Sub(".")
	require.NoError(t, err)
	_, err = nested.Stat(context.Background(), "note.md")
	require.NoError(t, err)

	_, err = vfs.AsActor(vfs.Stored(svc, testTenant), stranger).ReadFile(context.Background(), "note.md")
	require.Error(t, err, "an actor with no grant reads nothing, bound or not")
}
