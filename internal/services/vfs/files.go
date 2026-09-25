package vfs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// Files is the file interface the runtime reads and writes a workspace through.
// A service takes one of these instead of reaching for os, so the same code
// runs against a workspace on disk and one held in the database.
//
// Names are relative to the handle's root: "" and "." are the root itself, a
// leading "/" names the same place, and every path an implementation hands back
// is in that form. A name is contained before it is touched — a ".." that would
// leave the root is refused with [ErrEscape] rather than clamped — so a handle
// is safe to hand to code that composes paths from model output. A missing
// entry reports an error satisfying [fs.ErrNotExist], which is what lets a
// caller skip an absent optional file the same way on both backends.
//
// [View] is the handle over a directory on disk; [Stored] is the handle over a
// tenant's tree in a [Service].
type Files interface {
	ReadFile(ctx context.Context, name string) ([]byte, error)
	WriteFile(ctx context.Context, name string, data []byte) error
	MkdirAll(ctx context.Context, name string) error
	Remove(ctx context.Context, name string) error
	Stat(ctx context.Context, name string) (os.FileInfo, error)
	ReadDir(ctx context.Context, name string) ([]os.DirEntry, error)
	// WalkDir visits name and everything under it, depth first and in lexical
	// order per directory, with fs.WalkDir's semantics: fn receives paths
	// relative to the handle's root ("." for name itself), and returning
	// fs.SkipDir skips that directory's contents.
	WalkDir(ctx context.Context, name string, fn fs.WalkDirFunc) error
	// Sub returns a handle over dir, a directory inside this handle's root. The
	// directory need not exist: a handle is a name, and the first read or write
	// is what decides whether it does.
	Sub(dir string) (Files, error)
}

var _ Files = (*View)(nil)

// Sub returns a handle rooted at dir, which is contained by this view.
func (v *View) Sub(dir string) (Files, error) {
	resolved, err := v.Resolve(dir)
	if err != nil {
		return nil, err
	}
	child, err := newView(resolved)
	if err != nil {
		return nil, err
	}
	child.privileged = v.privileged
	return child, nil
}

var _ Files = (*View)(nil)

// ReadFile returns the contents of name.
func (v *View) ReadFile(_ context.Context, name string) ([]byte, error) {
	abs, err := v.Resolve(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// WriteFile replaces name's contents, creating it when absent. Intermediate
// folders are not created: a caller that composes a nested path creates them
// with [View.MkdirAll] first, so a typo in a directory name is not silently
// turned into a tree.
func (v *View) WriteFile(_ context.Context, name string, data []byte) error {
	abs, err := v.Resolve(name)
	if err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// MkdirAll creates name and any missing parents.
func (v *View) MkdirAll(_ context.Context, name string) error {
	abs, err := v.Resolve(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o750)
}

// Remove deletes name, which must be a file or an empty folder.
func (v *View) Remove(_ context.Context, name string) error {
	abs, err := v.Resolve(name)
	if err != nil {
		return err
	}
	return os.Remove(abs)
}

// Stat reports name's metadata.
func (v *View) Stat(_ context.Context, name string) (os.FileInfo, error) {
	abs, err := v.Resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(abs)
}

// ReadDir lists name's immediate children.
func (v *View) ReadDir(_ context.Context, name string) ([]os.DirEntry, error) {
	abs, err := v.Resolve(name)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(abs)
}

// WalkDir visits every file and folder under name, depth first and in lexical
// order per directory. fn receives paths relative to the view's root — "." for
// name itself — so a caller reads them with the same handle it walked, and a
// callback returning fs.SkipDir skips that folder's contents.
func (v *View) WalkDir(_ context.Context, name string, fn fs.WalkDirFunc) error {
	abs, err := v.Resolve(name)
	if err != nil {
		return err
	}
	return filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		rel, relErr := filepath.Rel(v.realRoot, p)
		if relErr != nil {
			return relErr
		}
		return fn(filepath.ToSlash(rel), d, err)
	})
}
