package vfs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/runtime/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_Contain_AllowsPathsWithinRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "a.txt"), []byte("x"), 0o644))

	// Relative candidate.
	got, err := vfs.Contain(root, "sub/a.txt")
	require.NoError(t, err)
	real, _ := filepath.EvalSymlinks(filepath.Join(root, "sub", "a.txt"))
	assert.Equal(t, real, got)

	// The root itself.
	got, err = vfs.Contain(root, ".")
	require.NoError(t, err)
	realRoot, _ := filepath.EvalSymlinks(root)
	assert.Equal(t, realRoot, got)

	// A non-existent leaf (write target) validates.
	got, err = vfs.Contain(root, "sub/new.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realRoot, "sub", "new.txt"), got)
}

func TestUnit_Contain_RejectsTraversalEscape(t *testing.T) {
	root := t.TempDir()
	_, err := vfs.Contain(root, "../escape")
	require.Error(t, err)
	assert.True(t, errors.Is(err, vfs.ErrEscape), "traversal must wrap ErrEscape, got %v", err)
}

func TestUnit_Contain_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	// Reading through an escaping symlink is rejected.
	_, err := vfs.Contain(root, "link/secret.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, vfs.ErrEscape))

	// Writing a new file under an escaping symlink is rejected before any I/O.
	_, err = vfs.Contain(root, "link/new.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, vfs.ErrEscape))
}

func TestUnit_Within(t *testing.T) {
	root := t.TempDir()
	assert.True(t, vfs.Within(root, filepath.Join(root, "a", "b")))
	assert.True(t, vfs.Within(root, root))
	assert.False(t, vfs.Within(root, filepath.Dir(root)))
	assert.False(t, vfs.Within(root, t.TempDir()))
}

func TestUnit_Factory_AllowlistAndDefault(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	outside := t.TempDir()

	f, err := vfs.NewFactory(a, b, a /* dup ignored */)
	require.NoError(t, err)

	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	assert.Equal(t, []string{ra, rb}, f.Roots())
	assert.Equal(t, ra, f.Default())

	// "/" and "" resolve to the default (compat for clients that send cwd:"/").
	got, ok := f.Allows("/")
	require.True(t, ok)
	assert.Equal(t, ra, got)
	got, ok = f.Allows("")
	require.True(t, ok)
	assert.Equal(t, ra, got)

	// An allowlisted root resolves to itself.
	got, ok = f.Allows(b)
	require.True(t, ok)
	assert.Equal(t, rb, got)

	// A non-allowlisted absolute path is refused.
	_, ok = f.Allows(outside)
	assert.False(t, ok)

	_, err = f.Resolve(outside)
	require.Error(t, err)
	assert.True(t, errors.Is(err, vfs.ErrNotAllowed))
}

func TestUnit_Factory_EmptyIsError(t *testing.T) {
	_, err := vfs.NewFactory()
	require.Error(t, err)
	_, err = vfs.NewFactory("", "")
	require.Error(t, err)
}

func TestUnit_View_ResolveAndContains(t *testing.T) {
	a := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(a, "f.txt"), []byte("x"), 0o644))
	f, err := vfs.NewFactory(a)
	require.NoError(t, err)

	v, err := f.Open("/")
	require.NoError(t, err)
	ra, _ := filepath.EvalSymlinks(a)
	assert.Equal(t, ra, v.Root())

	got, err := v.Resolve("f.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(ra, "f.txt"), got)

	assert.True(t, v.Contains(filepath.Join(ra, "f.txt")))
	assert.False(t, v.Contains(filepath.Dir(ra)))

	_, err = f.Open(t.TempDir())
	require.Error(t, err, "opening a non-allowlisted root must fail")
}
