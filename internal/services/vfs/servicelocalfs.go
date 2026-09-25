package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
)

// localFS implements Service against the local filesystem under a root directory.
// Each tenant gets its own subdirectory (root/<tenantID>/...) so a single
// localFS instance can host many tenants.
type localFS struct {
	root string
	cb   Callbacks
}

// NewLocalFS returns a Service backed by the local filesystem rooted at root.
// Each tenant's data is namespaced under root/<tenantID>/. Pass Callbacks{}
// for pure storage; proprietary callers use the hooks to enforce tenancy
// policy or record ownership.
func NewLocalFS(root string, cb Callbacks) Service {
	return &localFS{root: filepath.Clean(root), cb: cb}
}

var _ Service = (*localFS)(nil)

// tenantRoot returns the per-tenant base directory.
func (l *localFS) tenantRoot(tenantID string) string {
	return filepath.Join(l.root, tenantID)
}

// abs resolves path under the per-tenant root and rejects traversals that
// escape it.
func (l *localFS) abs(tenantID, path string) (string, error) {
	base := l.tenantRoot(tenantID)
	target := filepath.Join(base, filepath.FromSlash(path))
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes tenant root directory", path)
	}
	return target, nil
}

// rel expresses target relative to the per-tenant root.
func (l *localFS) rel(tenantID, target string) (string, error) {
	r, err := filepath.Rel(l.tenantRoot(tenantID), target)
	if err != nil {
		return "", fmt.Errorf("resolve path relative to tenant root: %w", err)
	}
	return r, nil
}

// --- callback shims ---

func (l *localFS) beforeRead(ctx context.Context, tenantID, id string) error {
	if l.cb.BeforeRead != nil {
		return l.cb.BeforeRead(ctx, tenantID, id)
	}
	return nil
}

func (l *localFS) beforeWrite(ctx context.Context, tenantID, id string) error {
	if l.cb.BeforeWrite != nil {
		return l.cb.BeforeWrite(ctx, tenantID, id)
	}
	return nil
}

func (l *localFS) reportHookError(ctx context.Context, tenantID, op, id string, err error) {
	if err != nil && l.cb.OnError != nil {
		l.cb.OnError(ctx, tenantID, op, id, err)
	}
}

func (l *localFS) onCreate(ctx context.Context, tenantID string, f *File) {
	if l.cb.OnCreate != nil {
		l.reportHookError(ctx, tenantID, "create", f.ID, l.cb.OnCreate(ctx, tenantID, f))
	}
}

func (l *localFS) onUpdate(ctx context.Context, tenantID string, f *File) {
	if l.cb.OnUpdate != nil {
		l.reportHookError(ctx, tenantID, "update", f.ID, l.cb.OnUpdate(ctx, tenantID, f))
	}
}

func (l *localFS) onDelete(ctx context.Context, tenantID, id string) {
	if l.cb.OnDelete != nil {
		l.reportHookError(ctx, tenantID, "delete", id, l.cb.OnDelete(ctx, tenantID, id))
	}
}

func (l *localFS) CreateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	if err := l.beforeWrite(ctx, tenantID, ""); err != nil {
		return nil, err
	}
	if file.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if int64(len(file.Data)) > MaxUploadSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size")
	}
	dir := l.tenantRoot(tenantID)
	if file.ParentID != "" {
		p, err := l.abs(tenantID, file.ParentID)
		if err != nil {
			return nil, err
		}
		dir = p
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	dest := filepath.Join(dir, file.Name)
	if err := os.WriteFile(dest, file.Data, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	now := time.Now().UTC()
	rel, err := l.rel(tenantID, dest)
	if err != nil {
		return nil, err
	}
	res := &File{
		ID:          rel,
		Path:        filepath.ToSlash(rel),
		Name:        file.Name,
		ParentID:    file.ParentID,
		Size:        int64(len(file.Data)),
		ContentType: file.ContentType,
		Data:        file.Data,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	l.onCreate(ctx, tenantID, res)
	return res, nil
}

func (l *localFS) GetFileByID(ctx context.Context, tenantID, id string) (*File, error) {
	if err := l.beforeRead(ctx, tenantID, id); err != nil {
		return nil, err
	}
	path, err := l.abs(tenantID, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %w", libdb.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	rel, err := l.rel(tenantID, path)
	if err != nil {
		return nil, err
	}
	return &File{
		ID:        rel,
		Path:      filepath.ToSlash(rel),
		Name:      name,
		Size:      info.Size(),
		Data:      data,
		UpdatedAt: info.ModTime().UTC(),
	}, nil
}

func (l *localFS) GetFolderByID(ctx context.Context, tenantID, id string) (*Folder, error) {
	if err := l.beforeRead(ctx, tenantID, id); err != nil {
		return nil, err
	}
	path, err := l.abs(tenantID, id)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("folder not found: %w", libdb.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}
	rel, err := l.rel(tenantID, path)
	if err != nil {
		return nil, err
	}
	return &Folder{
		ID:   rel,
		Path: filepath.ToSlash(rel),
		Name: filepath.Base(path),
	}, nil
}

func (l *localFS) GetFilesByPath(ctx context.Context, tenantID, path string, page Page) ([]File, *Cursor, error) {
	if err := page.Validate(); err != nil {
		return nil, nil, err
	}
	if path == "/" {
		path = ""
	}
	dir, err := l.abs(tenantID, path)
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var files []File
	for _, e := range entries {
		rel, err := l.rel(tenantID, filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, nil, err
		}
		if err := l.beforeRead(ctx, tenantID, rel); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, nil, fmt.Errorf("stat directory entry %q: %w", e.Name(), err)
		}
		files = append(files, File{
			ID:          rel,
			Path:        filepath.ToSlash(rel),
			Name:        e.Name(),
			IsDirectory: e.IsDir(),
			Size:        info.Size(),
			UpdatedAt:   info.ModTime().UTC(),
		})
	}
	out, next := paginateLocalFiles(files, page)
	return out, next, nil
}

// paginateLocalFiles sorts files by the page's sort field and direction, applies
// the keyset cursor, and caps the result at the page limit. The local FS has no
// creation time, so SortByCreatedAt orders by modification time (UpdatedAt). It
// returns a non-nil cursor when more entries remain.
func paginateLocalFiles(files []File, page Page) ([]File, *Cursor) {
	limit := page.EffectiveLimit()
	byName := page.SortOrDefault() == SortByName

	less := func(a, b File) bool {
		if byName {
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.ID < b.ID
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		return a.ID < b.ID
	}
	sort.Slice(files, func(i, j int) bool {
		if page.Desc {
			return less(files[j], files[i])
		}
		return less(files[i], files[j])
	})

	if page.After != nil {
		cur := File{Name: page.After.Name, ID: page.After.ID, UpdatedAt: page.After.CreatedAt}
		i := 0
		for i < len(files) {
			var after bool
			if page.Desc {
				after = less(files[i], cur)
			} else {
				after = less(cur, files[i])
			}
			if after {
				break
			}
			i++
		}
		files = files[i:]
	}

	var next *Cursor
	if len(files) > limit {
		files = files[:limit]
		last := files[len(files)-1]
		next = &Cursor{CreatedAt: last.UpdatedAt, Name: last.Name, ID: last.ID}
	}
	return files, next
}

func (l *localFS) UpdateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	if err := l.beforeWrite(ctx, tenantID, file.ID); err != nil {
		return nil, err
	}
	if int64(len(file.Data)) > MaxUploadSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size")
	}
	path, err := l.abs(tenantID, file.ID)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, file.Data, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	now := time.Now().UTC()
	rel, err := l.rel(tenantID, path)
	if err != nil {
		return nil, err
	}
	res := &File{
		ID:          rel,
		Path:        filepath.ToSlash(rel),
		Name:        filepath.Base(path),
		Size:        int64(len(file.Data)),
		ContentType: file.ContentType,
		Data:        file.Data,
		UpdatedAt:   now,
	}
	l.onUpdate(ctx, tenantID, res)
	return res, nil
}

func (l *localFS) DeleteFile(ctx context.Context, tenantID, id string) error {
	if err := l.beforeWrite(ctx, tenantID, id); err != nil {
		return err
	}
	path, err := l.abs(tenantID, id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	l.onDelete(ctx, tenantID, id)
	return nil
}

func (l *localFS) CreateFolder(ctx context.Context, tenantID, parentID, name string) (*Folder, error) {
	if err := l.beforeWrite(ctx, tenantID, parentID); err != nil {
		return nil, err
	}
	base, err := l.abs(tenantID, parentID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rel, err := l.rel(tenantID, dir)
	if err != nil {
		return nil, err
	}
	return &Folder{
		ID:       rel,
		Path:     filepath.ToSlash(rel),
		Name:     name,
		ParentID: parentID,
	}, nil
}

func (l *localFS) RenameFile(ctx context.Context, tenantID, fileID, newName string) (*File, error) {
	if err := l.beforeWrite(ctx, tenantID, fileID); err != nil {
		return nil, err
	}
	if strings.Contains(newName, "/") {
		return nil, fmt.Errorf("name cannot contain slashes")
	}
	src, err := l.abs(tenantID, fileID)
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(filepath.Dir(src), newName)
	if err := os.Rename(src, dst); err != nil {
		return nil, err
	}
	rel, err := l.rel(tenantID, dst)
	if err != nil {
		return nil, err
	}
	return &File{
		ID:   rel,
		Path: filepath.ToSlash(rel),
		Name: newName,
	}, nil
}

func (l *localFS) RenameFolder(ctx context.Context, tenantID, folderID, newName string) (*Folder, error) {
	if err := l.beforeWrite(ctx, tenantID, folderID); err != nil {
		return nil, err
	}
	if strings.Contains(newName, "/") {
		return nil, fmt.Errorf("name cannot contain slashes")
	}
	src, err := l.abs(tenantID, folderID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(src)
	dst := filepath.Join(dir, newName)
	if err := os.Rename(src, dst); err != nil {
		return nil, err
	}
	rel, err := l.rel(tenantID, dst)
	if err != nil {
		return nil, err
	}
	parentRel, err := l.rel(tenantID, dir)
	if err != nil {
		return nil, err
	}
	if parentRel == "." {
		parentRel = ""
	}
	return &Folder{
		ID:       rel,
		Path:     filepath.ToSlash(rel),
		Name:     newName,
		ParentID: filepath.ToSlash(parentRel),
	}, nil
}

func (l *localFS) DeleteFolder(ctx context.Context, tenantID, folderID string) error {
	if err := l.beforeWrite(ctx, tenantID, folderID); err != nil {
		return err
	}
	path, err := l.abs(tenantID, folderID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	l.onDelete(ctx, tenantID, folderID)
	return nil
}

func (l *localFS) MoveFile(ctx context.Context, tenantID, fileID, newParentID string) (*File, error) {
	if err := l.beforeWrite(ctx, tenantID, fileID); err != nil {
		return nil, err
	}
	src, err := l.abs(tenantID, fileID)
	if err != nil {
		return nil, err
	}
	dstDir, err := l.abs(tenantID, newParentID)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(src)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, err
	}
	dst := filepath.Join(dstDir, name)
	if err := os.Rename(src, dst); err != nil {
		return nil, err
	}
	rel, err := l.rel(tenantID, dst)
	if err != nil {
		return nil, err
	}
	return &File{
		ID:       rel,
		Path:     filepath.ToSlash(rel),
		Name:     name,
		ParentID: newParentID,
	}, nil
}

func (l *localFS) MoveFolder(ctx context.Context, tenantID, folderID, newParentID string) (*Folder, error) {
	if err := l.beforeWrite(ctx, tenantID, folderID); err != nil {
		return nil, err
	}
	src, err := l.abs(tenantID, folderID)
	if err != nil {
		return nil, err
	}
	dstDir, err := l.abs(tenantID, newParentID)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(src)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, err
	}
	dst := filepath.Join(dstDir, name)
	if err := os.Rename(src, dst); err != nil {
		return nil, err
	}
	rel, err := l.rel(tenantID, dst)
	if err != nil {
		return nil, err
	}
	return &Folder{
		ID:       rel,
		Path:     filepath.ToSlash(rel),
		Name:     name,
		ParentID: newParentID,
	}, nil
}
