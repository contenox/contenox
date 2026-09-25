package vfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
)

// Stored returns a [Files] handle over one tenant's tree in svc, rooted at the
// tenant itself. Nothing is created by the call: a tenant holding no files has
// a root that exists and is empty, which is what lets a caller treat a store
// exactly as it treats a directory that has not been populated yet.
//
// When svc was built with an access list, the context's [Actor] is what every
// read and write is decided against, so a caller acting for the platform stamps
// one with [WithActor] before compiling anything out of the store.
func Stored(svc Service, tenantID string) Files {
	return &storedFiles{svc: svc, tenantID: tenantID}
}

// storedFiles is [Files] over a [Service]: a name is a slash-separated path
// inside one tenant, and every verb is a Service call.
type storedFiles struct {
	svc      Service
	tenantID string
	// prefix is the handle's root inside the tenant; "" is the tenant root.
	prefix string
}

var _ Files = (*storedFiles)(nil)

// storedPageLimit is the page size a listing reads with: large enough that a
// walk of a declaration tree is one round trip per directory, and under the
// store's own ceiling.
const storedPageLimit = 1000

func (s *storedFiles) Sub(dir string) (Files, error) {
	rel, err := cleanName(dir)
	if err != nil {
		return nil, err
	}
	return &storedFiles{svc: s.svc, tenantID: s.tenantID, prefix: joinName(s.prefix, rel)}, nil
}

func (s *storedFiles) ReadFile(ctx context.Context, name string) ([]byte, error) {
	entry, err := s.entry(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry.IsDirectory {
		return nil, pathErr("read", name, errors.New("is a directory"))
	}
	full, err := s.svc.GetFileByID(ctx, s.tenantID, entry.ID)
	if err != nil {
		return nil, storeErr("read", name, err)
	}
	return full.Data, nil
}

func (s *storedFiles) WriteFile(ctx context.Context, name string, data []byte) error {
	rel, err := cleanName(name)
	if err != nil {
		return err
	}
	if rel == "" {
		return pathErr("write", name, errors.New("the root is not a file"))
	}
	entry, err := s.entry(ctx, name)
	switch {
	case err == nil && entry.IsDirectory:
		return pathErr("write", name, errors.New("is a directory"))
	case err == nil:
		_, err = s.svc.UpdateFile(ctx, s.tenantID, &File{
			ID:          entry.ID,
			Name:        entry.Name,
			ParentID:    entry.ParentID,
			ContentType: entry.ContentType,
			Metadata:    entry.Metadata,
			CreatedAt:   entry.CreatedAt,
			UpdatedAt:   entry.UpdatedAt,
			Data:        data,
		})
		return err
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	parent, base := path.Split(rel)
	dir, err := s.entry(ctx, strings.TrimSuffix(parent, "/"))
	if err != nil {
		return err
	}
	if !dir.IsDirectory {
		return pathErr("write", name, errors.New("the containing path is not a directory"))
	}
	_, err = s.svc.CreateFile(ctx, s.tenantID, &File{
		Name:        base,
		ParentID:    dir.ID,
		ContentType: storedContentType(base),
		Data:        data,
	})
	return err
}

func (s *storedFiles) MkdirAll(ctx context.Context, name string) error {
	rel, err := cleanName(name)
	if err != nil {
		return err
	}
	parentID := ""
	built := ""
	for _, segment := range strings.Split(joinName(s.prefix, rel), "/") {
		if segment == "" {
			continue
		}
		built = joinName(built, segment)
		entry, err := s.entryAt(ctx, built)
		switch {
		case err == nil:
			if !entry.IsDirectory {
				return pathErr("mkdir", name, fmt.Errorf("%s is not a directory", built))
			}
			parentID = entry.ID
		case errors.Is(err, fs.ErrNotExist):
			folder, cErr := s.svc.CreateFolder(ctx, s.tenantID, parentID, segment)
			if cErr != nil {
				return cErr
			}
			parentID = folder.ID
		default:
			return err
		}
	}
	return nil
}

func (s *storedFiles) Remove(ctx context.Context, name string) error {
	rel, err := cleanName(name)
	if err != nil {
		return err
	}
	if rel == "" {
		return pathErr("remove", name, errors.New("the root is not removable"))
	}
	entry, err := s.entry(ctx, name)
	if err != nil {
		return err
	}
	if entry.IsDirectory {
		return s.svc.DeleteFolder(ctx, s.tenantID, entry.ID)
	}
	return s.svc.DeleteFile(ctx, s.tenantID, entry.ID)
}

func (s *storedFiles) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	rel, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	entry, err := s.entry(ctx, name)
	if err != nil {
		return nil, err
	}
	return storedInfo{name: baseName(rel, entry), size: entry.Size, dir: entry.IsDirectory, mod: entry.UpdatedAt}, nil
}

func (s *storedFiles) ReadDir(ctx context.Context, name string) ([]os.DirEntry, error) {
	rel, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	entry, err := s.entry(ctx, name)
	if err != nil {
		return nil, err
	}
	if !entry.IsDirectory {
		return nil, pathErr("readdir", name, errors.New("not a directory"))
	}
	children, err := s.list(ctx, rel)
	if err != nil {
		return nil, err
	}
	out := make([]os.DirEntry, 0, len(children))
	for i := range children {
		out = append(out, storedEntry{
			name: children[i].Name,
			size: children[i].Size,
			dir:  children[i].IsDirectory,
			mod:  children[i].UpdatedAt,
		})
	}
	return out, nil
}

func (s *storedFiles) WalkDir(ctx context.Context, name string, fn fs.WalkDirFunc) error {
	rel, err := cleanName(name)
	if err != nil {
		return err
	}
	if _, err := s.entry(ctx, name); err != nil {
		return fn(displayName(rel), nil, err)
	}
	return s.walk(ctx, rel, fn)
}

// walk visits rel and its children, reporting each through fn with the same
// relative names ReadDir and Stat take.
func (s *storedFiles) walk(ctx context.Context, rel string, fn fs.WalkDirFunc) error {
	entry, err := s.entry(ctx, rel)
	if err != nil {
		return fn(displayName(rel), nil, err)
	}
	err = fn(displayName(rel), storedEntry{
		name: baseName(rel, entry),
		size: entry.Size,
		dir:  entry.IsDirectory,
		mod:  entry.UpdatedAt,
	}, nil)
	if err != nil {
		if entry.IsDirectory && errors.Is(err, fs.SkipDir) {
			return nil
		}
		return err
	}
	if !entry.IsDirectory {
		return nil
	}
	children, err := s.list(ctx, rel)
	if err != nil {
		return err
	}
	for i := range children {
		if err := s.walk(ctx, joinName(rel, children[i].Name), fn); err != nil {
			return err
		}
	}
	return nil
}

// entry resolves a handle-relative name to the stored row for the file or
// folder it names. The root always exists and is a folder.
func (s *storedFiles) entry(ctx context.Context, name string) (*File, error) {
	rel, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	return s.entryAt(ctx, joinName(s.prefix, rel))
}

// entryAt resolves a tenant-relative path, which is what MkdirAll walks.
func (s *storedFiles) entryAt(ctx context.Context, full string) (*File, error) {
	if full == "" {
		return &File{IsDirectory: true}, nil
	}
	parent, base := path.Split(full)
	children, err := s.listAt(ctx, strings.TrimSuffix(parent, "/"))
	if err != nil {
		return nil, err
	}
	for i := range children {
		if children[i].Name == base {
			return &children[i], nil
		}
	}
	return nil, pathErr("stat", full, fs.ErrNotExist)
}

// list reads every child of a handle-relative directory, paging until the store
// says there is no more.
func (s *storedFiles) list(ctx context.Context, rel string) ([]File, error) {
	return s.listAt(ctx, joinName(s.prefix, rel))
}

func (s *storedFiles) listAt(ctx context.Context, full string) ([]File, error) {
	var out []File
	var after *Cursor
	for {
		files, next, err := s.svc.GetFilesByPath(ctx, s.tenantID, full, Page{
			SortBy: SortByName,
			After:  after,
			Limit:  storedPageLimit,
		})
		if err != nil {
			return nil, storeErr("list", full, err)
		}
		out = append(out, files...)
		if next == nil {
			break
		}
		after = next
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// cleanName normalizes a handle-relative name: "" and "." and a leading "/" all
// name the root, and a ".." that would leave the root is refused rather than
// clamped, so a name composed from model output cannot reach outside the handle.
func cleanName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", nil
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	depth := 0
	for _, segment := range strings.Split(trimmed, "/") {
		switch segment {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return "", fmt.Errorf("%w: %s escapes the stored root", ErrEscape, name)
			}
		default:
			depth++
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == "/" {
		return "", nil
	}
	return cleaned, nil
}

func joinName(prefix, rel string) string {
	switch {
	case prefix == "":
		return rel
	case rel == "":
		return prefix
	default:
		return prefix + "/" + rel
	}
}

// displayName is the path a walk reports: "." for the root it started from,
// otherwise the handle-relative name.
func displayName(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

// baseName is what a listing calls an entry, which is os.DirEntry's and
// os.FileInfo's Name: the last segment, "." for the root itself.
func baseName(rel string, entry *File) string {
	if entry != nil && entry.Name != "" {
		return entry.Name
	}
	if rel == "" {
		return "."
	}
	return path.Base(rel)
}

func pathErr(op, name string, err error) error {
	return &fs.PathError{Op: op, Path: name, Err: err}
}

// storeErr maps the store's own refusals onto the [Files] contract: a path that
// does not resolve is fs.ErrNotExist, and everything else passes through.
func storeErr(op, name string, err error) error {
	if errors.Is(err, ErrUnknownPath) || errors.Is(err, libdb.ErrNotFound) {
		return pathErr(op, name, fs.ErrNotExist)
	}
	return err
}

// storedContentType names the type of a file from its extension. The store
// keeps no content sniffing of its own, and a declaration tree holds text.
func storedContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	case ".toml":
		return "application/toml"
	case ".txt":
		return "text/plain"
	default:
		return ""
	}
}

// storedInfo is os.FileInfo over a stored entry. The store keeps no permission
// bits, so a folder is 0755 and a file 0644: enough for the IsDir and Size
// questions a caller asks, and no claim about an authority the store does not
// have.
type storedInfo struct {
	name string
	size int64
	dir  bool
	mod  time.Time
}

func (i storedInfo) Name() string       { return i.name }
func (i storedInfo) Size() int64        { return i.size }
func (i storedInfo) ModTime() time.Time { return i.mod }
func (i storedInfo) IsDir() bool        { return i.dir }
func (i storedInfo) Sys() any           { return nil }

func (i storedInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

// storedEntry is os.DirEntry over one child of a listing, holding what the
// listing already read so Info needs no second query.
type storedEntry struct {
	name string
	size int64
	dir  bool
	mod  time.Time
}

func (e storedEntry) Name() string { return e.name }
func (e storedEntry) IsDir() bool  { return e.dir }

func (e storedEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}

func (e storedEntry) Info() (fs.FileInfo, error) {
	return storedInfo{name: e.name, size: e.size, dir: e.dir, mod: e.mod}, nil
}
