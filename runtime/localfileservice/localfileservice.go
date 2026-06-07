package localfileservice

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
)

const MaxWriteSize = 10 * 1024 * 1024

var ErrInvalidPath = errors.New("invalid local path")

type Entry struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType,omitempty"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	IsDirectory bool      `json:"isDirectory"`
}

type Service interface {
	Root() string
	List(ctx context.Context, relPath string) ([]Entry, error)
	Stat(ctx context.Context, relPath string) (*Entry, error)
	Read(ctx context.Context, relPath string) ([]byte, *Entry, error)
	Write(ctx context.Context, relPath string, data []byte, createOnly bool) (*Entry, error)
	Mkdir(ctx context.Context, relPath string) (*Entry, error)
	Delete(ctx context.Context, relPath string) error
	Move(ctx context.Context, fromPath, toPath string) (*Entry, error)
}

type localService struct {
	root     string
	rootReal string
}

func New(root string) (Service, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidPath)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0750); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve root symlinks: %w", err)
	}
	return &localService{root: abs, rootReal: real}, nil
}

func (s *localService) Root() string {
	return s.root
}

func NormalizeRelPath(raw string, allowRoot bool) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\\", "/")
	if strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("%w: nul byte", ErrInvalidPath)
	}
	if raw == "" || raw == "." || raw == "/" {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: absolute paths are not allowed", ErrInvalidPath)
	}
	clean := filepath.ToSlash(filepath.Clean(raw))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" {
		if allowRoot {
			return ".", nil
		}
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("%w: path traversal is not allowed", ErrInvalidPath)
	}
	return clean, nil
}

func (s *localService) List(ctx context.Context, relPath string) ([]Entry, error) {
	_ = ctx
	abs, rel, err := s.resolveExisting(relPath, true)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, mapOSError(err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidPath, rel)
	}
	items, err := os.ReadDir(abs)
	if err != nil {
		return nil, mapOSError(err)
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, mapOSError(err)
		}
		childRel := item.Name()
		if rel != "." {
			childRel = filepath.ToSlash(filepath.Join(rel, item.Name()))
		}
		entries = append(entries, entryFromInfo(childRel, info))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDirectory != entries[j].IsDirectory {
			return entries[i].IsDirectory
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (s *localService) Stat(ctx context.Context, relPath string) (*Entry, error) {
	_ = ctx
	abs, rel, err := s.resolveExisting(relPath, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, mapOSError(err)
	}
	entry := entryFromInfo(rel, info)
	return &entry, nil
}

func (s *localService) Read(ctx context.Context, relPath string) ([]byte, *Entry, error) {
	_ = ctx
	abs, rel, err := s.resolveExisting(relPath, false)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, nil, mapOSError(err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%w: cannot read directory", ErrInvalidPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, mapOSError(err)
	}
	entry := entryFromInfo(rel, info)
	return data, &entry, nil
}

func (s *localService) Write(ctx context.Context, relPath string, data []byte, createOnly bool) (*Entry, error) {
	_ = ctx
	if len(data) > MaxWriteSize {
		return nil, fmt.Errorf("%w: file exceeds %d byte limit", ErrInvalidPath, MaxWriteSize)
	}
	abs, rel, err := s.resolveForWrite(relPath)
	if err != nil {
		return nil, err
	}
	if createOnly {
		if _, err := os.Stat(abs); err == nil {
			return nil, fmt.Errorf("%w: file already exists", libdb.ErrUniqueViolation)
		} else if !os.IsNotExist(err) {
			return nil, mapOSError(err)
		}
	}
	if err := os.WriteFile(abs, data, 0644); err != nil {
		return nil, mapOSError(err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, mapOSError(err)
	}
	entry := entryFromInfo(rel, info)
	return &entry, nil
}

func (s *localService) Mkdir(ctx context.Context, relPath string) (*Entry, error) {
	_ = ctx
	abs, rel, err := s.resolveForWrite(relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, mapOSError(err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, mapOSError(err)
	}
	entry := entryFromInfo(rel, info)
	return &entry, nil
}

func (s *localService) Delete(ctx context.Context, relPath string) error {
	_ = ctx
	abs, _, err := s.resolveExisting(relPath, false)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(abs); err != nil {
		return mapOSError(err)
	}
	return nil
}

func (s *localService) Move(ctx context.Context, fromPath, toPath string) (*Entry, error) {
	_ = ctx
	fromAbs, _, err := s.resolveExisting(fromPath, false)
	if err != nil {
		return nil, err
	}
	toAbs, toRel, err := s.resolveForWrite(toPath)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return nil, mapOSError(err)
	}
	info, err := os.Stat(toAbs)
	if err != nil {
		return nil, mapOSError(err)
	}
	entry := entryFromInfo(toRel, info)
	return &entry, nil
}

func (s *localService) resolveExisting(raw string, allowRoot bool) (string, string, error) {
	rel, err := NormalizeRelPath(raw, allowRoot)
	if err != nil {
		return "", "", err
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", mapOSError(err)
	}
	if !s.isWithinRoot(real) {
		return "", "", fmt.Errorf("%w: symlink escapes root", ErrInvalidPath)
	}
	return abs, rel, nil
}

func (s *localService) resolveForWrite(raw string) (string, string, error) {
	rel, err := NormalizeRelPath(raw, false)
	if err != nil {
		return "", "", err
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", "", mapOSError(err)
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", mapOSError(err)
	}
	if !s.isWithinRoot(parentReal) {
		return "", "", fmt.Errorf("%w: parent symlink escapes root", ErrInvalidPath)
	}
	return abs, rel, nil
}

func (s *localService) isWithinRoot(abs string) bool {
	rel, err := filepath.Rel(s.rootReal, abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func entryFromInfo(rel string, info os.FileInfo) Entry {
	rel = filepath.ToSlash(rel)
	contentType := ""
	if !info.IsDir() {
		contentType = mime.TypeByExtension(filepath.Ext(info.Name()))
		if contentType == "" {
			contentType = http.DetectContentType([]byte(info.Name()))
		}
	}
	return Entry{
		Path:        rel,
		Name:        info.Name(),
		ContentType: contentType,
		Size:        info.Size(),
		CreatedAt:   info.ModTime().UTC(),
		UpdatedAt:   info.ModTime().UTC(),
		IsDirectory: info.IsDir(),
	}
}

func mapOSError(err error) error {
	if os.IsNotExist(err) {
		return libdb.ErrNotFound
	}
	if os.IsPermission(err) {
		return fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	return err
}
