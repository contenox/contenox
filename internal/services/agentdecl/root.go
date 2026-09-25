package agentdecl

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/contenox/contenox/internal/services/vfs"
)

// Root is one place declarations and their generated output live: an open
// [vfs.Files] handle, the path that names it in reports, and whether that path
// is an operating-system one.
//
// The handle is what lets a declaration compile from a directory on disk and
// from a tenant's tree in a store through one code path. Key is what a human
// reads in a refusal and what the sync state keys a source by, so it has to
// stay stable across passes: a key that changes retires and rewrites every chain
// it names.
type Root struct {
	// Key names the root, and the files under it, in reports and sync state.
	Key string
	// FS is the handle over Key. A call that takes a Root refuses a nil one.
	FS vfs.Files
	// Local marks a root whose Key is an operating-system path, so a path under
	// it is spelled with the host's separator. A stored root spells them with
	// slashes.
	Local bool
}

// LocalRoot opens a directory on disk as a Root. The directory need not exist:
// a root is a name, and the first read is what decides whether it holds
// anything.
// It permits the runtime to manage its own declarations inside the control
// plane; the returned handle must never be exposed as an agent file tool.
//
// Key is the RESOLVED directory, not the string the caller passed. The sync
// state keys every source by the path it was read from, so a Key that changed
// spelling — a symlinked checkout, a relative path — would retire every chain
// the previous pass compiled, and chain discovery disables an agent whose chain
// file has vanished.
func LocalRoot(dir string) (Root, error) {
	handle, err := vfs.OpenPrivilegedView(dir)
	if err != nil {
		return Root{}, err
	}
	return Root{Key: handle.Root(), FS: handle, Local: true}, nil
}

// SourceDir is one directory of declarations. A native directory keeps the
// names its files declare; a foreign directory's are scoped by the product they
// came from.
type SourceDir struct {
	Root
	Native bool
}

// Child returns the root of a directory inside this one. The directory need not
// exist.
func (r Root) Child(rel string) (Root, error) {
	if r.FS == nil {
		return Root{}, fmt.Errorf("agentdecl: root %q holds no handle", r.Key)
	}
	handle, err := r.FS.Sub(rel)
	if err != nil {
		return Root{}, err
	}
	return Root{Key: r.Path(rel), FS: handle, Local: r.Local}, nil
}

// Path names a path inside the root the way this root's Key spells paths: with
// the host's separator under a directory on disk, with slashes under a store.
func (r Root) Path(rel string) string {
	if rel == "" || rel == "." {
		return r.Key
	}
	if r.Local {
		return filepath.Join(r.Key, filepath.FromSlash(rel))
	}
	return path.Join(r.Key, rel)
}

// Base is the root's own name, which is what an agent tree takes its name from.
func (r Root) Base() string {
	if r.Local {
		return filepath.Base(r.Key)
	}
	return path.Base(r.Key)
}
