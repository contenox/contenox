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

// TestUnit_ResolveSessionCwd pins the ONE decision procedure every session
// bring-up shares — the ACP session/new, session/load and session/resume paths
// and the REST fleet dispatch. Three variants of it used to exist and had
// already drifted: only the ACP entry points guarded against a relative cwd, and
// the two no-allowlist branches disagreed about what an absent cwd means.
func TestUnit_ResolveSessionCwd(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()
	f, err := vfs.NewFactory(allowed)
	require.NoError(t, err)
	resolvedAllowed, err := vfs.ResolveRoot(allowed)
	require.NoError(t, err)

	t.Run("relative cwd is refused with or without an allowlist", func(t *testing.T) {
		for _, factory := range []*vfs.Factory{nil, f} {
			for _, cwd := range []string{"../..", ".", "relative/path"} {
				_, err := vfs.ResolveSessionCwd(factory, cwd, "/fallback")
				require.Error(t, err, "cwd %q", cwd)
				require.ErrorIs(t, err, vfs.ErrCwdNotPermitted)
				require.Contains(t, err.Error(), "absolute path")
			}
		}
	})

	t.Run("no allowlist: an absolute cwd passes through, an absent one takes the caller's fallback", func(t *testing.T) {
		got, err := vfs.ResolveSessionCwd(nil, "/anywhere/at/all", "/fallback")
		require.NoError(t, err)
		assert.Equal(t, "/anywhere/at/all", got, "the editor owns the filesystem on the stdio path")

		// The sentinel is still a path when nothing constrains it.
		got, err = vfs.ResolveSessionCwd(nil, "/", "/fallback")
		require.NoError(t, err)
		assert.Equal(t, "/", got)

		got, err = vfs.ResolveSessionCwd(nil, "", "/fallback")
		require.NoError(t, err)
		assert.Equal(t, "/fallback", got, "unspecified means the CALLER's default root")

		got, err = vfs.ResolveSessionCwd(nil, "", "")
		require.NoError(t, err)
		assert.Equal(t, "", got, "a caller with no default leaves it unspecified")
	})

	t.Run("allowlist: sentinel and empty resolve to the default root", func(t *testing.T) {
		for _, cwd := range []string{"", "/"} {
			got, err := vfs.ResolveSessionCwd(f, cwd, "/ignored")
			require.NoError(t, err, "cwd %q", cwd)
			assert.Equal(t, resolvedAllowed, got,
				"a configured allowlist owns the default; the caller's fallback is not consulted")
		}
	})

	t.Run("allowlist: an allowlisted root resolves, anything else is refused", func(t *testing.T) {
		got, err := vfs.ResolveSessionCwd(f, allowed, "")
		require.NoError(t, err)
		assert.Equal(t, resolvedAllowed, got)

		_, err = vfs.ResolveSessionCwd(f, other, "")
		require.Error(t, err)
		require.ErrorIs(t, err, vfs.ErrCwdNotPermitted)
		require.Contains(t, err.Error(), "is not permitted")
	})

	t.Run("the refusal message is the operator-facing text, not the sentinel's", func(t *testing.T) {
		_, err := vfs.ResolveSessionCwd(f, other, "")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), vfs.ErrCwdNotPermitted.Error(),
			"callers forward Error() to a user; the sentinel is for errors.Is, not for reading")
	})
}
