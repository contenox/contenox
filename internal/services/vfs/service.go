// Package vfsservice exposes a tenant-aware virtual file system that the runtime
// consumes through its service APIs and task chains. Every method takes an
// explicit tenantID; the OSS runtime passes runtimetypes.LocalTenantID and
// proprietary builds pass real tenant values.
//
// Two backends implement Service. New is backed by a SQL database, storing file
// and folder metadata in the vfs_files and vfs_filestree tables and content in
// vfs_blobs. NewLocalFS is backed by the host filesystem under a per-tenant root
// directory. Both honor the same Callbacks, which proprietary builds use to
// enforce tenancy policy and record ownership.
package vfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/google/uuid"
)

const (
	// MaxUploadSize is the maximum allowed size for a single file upload (100 MiB).
	MaxUploadSize = 100 * 1024 * 1024
	// MaxFilesRowCount is the maximum number of files and folders a single
	// tenant may hold. CreateFile and CreateFolder reject new entries once a
	// tenant reaches this count.
	MaxFilesRowCount = 50000
)

var (
	// ErrUnknownPath is returned when a path cannot be resolved to a file or folder.
	ErrUnknownPath = fmt.Errorf("unable to resolve path")
	// ErrFolderNotEmpty is returned by DeleteFolder when the folder still has children.
	ErrFolderNotEmpty = fmt.Errorf("folder is not empty")
	// ErrNotSupported is reserved for operations a backend cannot perform. It is
	// currently unused: both the SQL and local-filesystem backends implement
	// every Service method.
	ErrNotSupported = errors.New("operation not supported")
	// ErrUnauthorized is the canonical error proprietary Callbacks should
	// return when a tenant is not permitted to perform an operation.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidMove is returned by MoveFolder for a move that would create a
	// cycle (a folder into itself or one of its descendants). It's a caller
	// error, so handlers map it to 400 rather than 500.
	ErrInvalidMove = errors.New("invalid move")
)

// Callbacks holds optional policy and ownership hooks called around VFS
// operations. Every hook receives the tenantID the operation is running under so
// a single Callbacks value can serve many tenants. Any nil field is skipped.
//
// Before* hooks run before the operation and can abort it by returning an error.
// On* hooks run after the operation's transaction has committed and so cannot
// abort it; an error they return is delivered to OnError instead. If OnError is
// nil the error is dropped.
type Callbacks struct {
	// BeforeRead runs before a read; returning an error blocks it.
	BeforeRead func(ctx context.Context, tenantID, resourceID string) error
	// BeforeWrite runs before a create, update, rename, move, or delete;
	// returning an error blocks the operation.
	BeforeWrite func(ctx context.Context, tenantID, resourceID string) error
	// OnCreate runs after a file or folder has been created.
	OnCreate func(ctx context.Context, tenantID string, file *File) error
	// OnUpdate runs after a file's content has been updated.
	OnUpdate func(ctx context.Context, tenantID string, file *File) error
	// OnDelete runs after a file or folder has been deleted; resourceID is the
	// deleted item's ID.
	OnDelete func(ctx context.Context, tenantID, resourceID string) error
	// OnError receives any error returned by a post-commit hook (OnCreate,
	// OnUpdate, OnDelete). The operation has already committed; the caller
	// decides how to respond (log, emit a metric, enqueue a repair). op is the
	// operation name ("create", "update", "delete") and resourceID is the
	// affected file or folder. If nil, post-commit hook errors are dropped.
	OnError func(ctx context.Context, tenantID, op, resourceID string, err error)
}

// Service defines all VFS operations. Every method takes tenantID explicitly so
// a single Service can host many tenants; OSS callers pass runtimetypes.LocalTenantID.
type Service interface {
	// CreateFile stores a new file with its content under file.ParentID and
	// returns it with its resolved path. file.Name must be non-empty and must
	// not contain "/", and the content may not exceed MaxUploadSize.
	CreateFile(ctx context.Context, tenantID string, file *File) (*File, error)
	// GetFileByID returns the file with the given ID, including its content.
	GetFileByID(ctx context.Context, tenantID, id string) (*File, error)
	// GetFolderByID returns the folder with the given ID.
	GetFolderByID(ctx context.Context, tenantID, id string) (*Folder, error)
	// GetFilesByPath resolves a slash-separated path. When the path names a
	// folder its children are returned keyset-paginated per page, along with a
	// non-nil cursor when more pages remain (nil on the last page). When the path
	// names a file, a single-element slice with that file and a nil cursor are
	// returned (page is ignored). It reports ErrUnknownPath if the path does not
	// resolve.
	GetFilesByPath(ctx context.Context, tenantID, path string, page Page) ([]File, *Cursor, error)
	// UpdateFile replaces the content of an existing file. It is an error to
	// target a folder or to exceed MaxUploadSize.
	UpdateFile(ctx context.Context, tenantID string, file *File) (*File, error)
	// DeleteFile removes a file and its content. It is an error to target a folder.
	DeleteFile(ctx context.Context, tenantID, id string) error
	// CreateFolder creates a folder named name under parentID and returns it. A
	// parentID of "" creates the folder at the tenant root.
	CreateFolder(ctx context.Context, tenantID, parentID, name string) (*Folder, error)
	// RenameFile changes a file's name within its current folder. newName must
	// not contain "/". It is an error to target a folder.
	RenameFile(ctx context.Context, tenantID, fileID, newName string) (*File, error)
	// RenameFolder changes a folder's name within its current parent. newName
	// must not contain "/". It is an error to target a file.
	RenameFolder(ctx context.Context, tenantID, folderID, newName string) (*Folder, error)
	// DeleteFolder removes an empty folder. It reports ErrFolderNotEmpty if the
	// folder still contains children.
	DeleteFolder(ctx context.Context, tenantID, folderID string) error
	// MoveFile reparents a file to newParentID, or to the tenant root when
	// newParentID is "". It is an error if newParentID refers to a non-folder or
	// the destination already contains an item with the same name.
	MoveFile(ctx context.Context, tenantID, fileID, newParentID string) (*File, error)
	// MoveFolder reparents a folder to newParentID, or to the tenant root when
	// newParentID is "", rejecting any move that would place a folder inside
	// itself or a descendant. It is an error if newParentID refers to a
	// non-folder or the destination already contains an item with the same name.
	MoveFolder(ctx context.Context, tenantID, folderID, newParentID string) (*Folder, error)
}

var _ Service = (*service)(nil)

type service struct {
	db libdb.DBManager
	cb Callbacks
}

// New creates a DB-backed VFS service, creating its tables first. Pass
// Callbacks{} for pure storage.
//
// The schema is created here rather than left to the caller because a
// deployment that forgets it fails at the first query with "no such table",
// which is a worse failure than a constructor error: it surfaces mid-request
// rather than at startup. InitSchema is idempotent and dispatches on the
// driver, so this is correct on a fresh SQLite file and on a Postgres that
// predates tenancy alike. NewLocalFS needs no schema and does not call it.
func New(ctx context.Context, db libdb.DBManager, cb Callbacks) (Service, error) {
	if db == nil {
		return nil, errors.New("vfsservice: a database manager is required")
	}
	if err := InitSchema(ctx, db.WithoutTransaction()); err != nil {
		return nil, fmt.Errorf("vfsservice: create schema: %w", err)
	}
	return &service{db: db, cb: cb}, nil
}

// --- callback shims ---

func (s *service) beforeRead(ctx context.Context, tenantID, id string) error {
	if s.cb.BeforeRead != nil {
		return s.cb.BeforeRead(ctx, tenantID, id)
	}
	return nil
}

func (s *service) beforeWrite(ctx context.Context, tenantID, id string) error {
	if s.cb.BeforeWrite != nil {
		return s.cb.BeforeWrite(ctx, tenantID, id)
	}
	return nil
}

func (s *service) reportHookError(ctx context.Context, tenantID, op, id string, err error) {
	if err != nil && s.cb.OnError != nil {
		s.cb.OnError(ctx, tenantID, op, id, err)
	}
}

func (s *service) onCreate(ctx context.Context, tenantID string, f *File) {
	if s.cb.OnCreate != nil {
		s.reportHookError(ctx, tenantID, "create", f.ID, s.cb.OnCreate(ctx, tenantID, f))
	}
}

func (s *service) onUpdate(ctx context.Context, tenantID string, f *File) {
	if s.cb.OnUpdate != nil {
		s.reportHookError(ctx, tenantID, "update", f.ID, s.cb.OnUpdate(ctx, tenantID, f))
	}
}

func (s *service) onDelete(ctx context.Context, tenantID, id string) {
	if s.cb.OnDelete != nil {
		s.reportHookError(ctx, tenantID, "delete", id, s.cb.OnDelete(ctx, tenantID, id))
	}
}

// --- internal helpers ---

func (s *service) getFileByID(ctx context.Context, tx libdb.Exec, tenantID, id string, withBlob bool) (*File, error) {
	storeInstance := newFileStore(tx)
	fileRecord, err := storeInstance.GetFileByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	var data []byte
	if withBlob {
		blob, err := storeInstance.GetBlobByID(ctx, tenantID, fileRecord.BlobsID)
		if err != nil {
			return nil, err
		}
		data = blob.Data
	}
	// Reconstruct path by walking up the tree with depth + cycle guards.
	const maxDepth = 256
	var pathSegments []string
	currentItemID := id
	fileName := ""
	seen := make(map[string]bool)
	for depth := 0; ; depth++ {
		if depth > maxDepth || seen[currentItemID] {
			return nil, fmt.Errorf("getFileByID: circular reference or max depth exceeded at ID '%s'", currentItemID)
		}
		seen[currentItemID] = true
		itemName, err := storeInstance.GetFileNameByID(ctx, tenantID, currentItemID)
		if err != nil {
			return nil, fmt.Errorf("getFileByID: failed to get name for item ID '%s': %w", currentItemID, err)
		}
		if fileName == "" {
			fileName = itemName
		}
		pathSegments = append([]string{itemName}, pathSegments...)
		parentOfCurrentItem, err := storeInstance.GetFileParentID(ctx, tenantID, currentItemID)
		if err != nil {
			return nil, fmt.Errorf("getFileByID: failed to get parent ID for item ID '%s': %w", currentItemID, err)
		}
		if parentOfCurrentItem == "" {
			break
		}
		currentItemID = parentOfCurrentItem
	}
	resolvedPath := strings.Join(pathSegments, "/")
	resolvedPath, _ = strings.CutPrefix(resolvedPath, "/")
	resolvedPath, _ = strings.CutSuffix(resolvedPath, "/")

	directParentID, err := storeInstance.GetFileParentID(ctx, tenantID, id)
	if err != nil && !errors.Is(err, libdb.ErrNotFound) {
		return nil, fmt.Errorf("getFileByID: failed to get direct parent ID for item ID '%s' from filestree: %w", id, err)
	}

	var metaData Metadata
	if err := json.Unmarshal(fileRecord.Meta, &metaData); err != nil {
		return nil, fmt.Errorf("failed to reconstruct metadata %w", err)
	}
	return &File{
		ID:          fileRecord.ID,
		Path:        resolvedPath,
		Name:        fileName,
		ContentType: fileRecord.Type,
		Data:        data,
		Size:        metaData.Size,
		ParentID:    directParentID,
		IsDirectory: fileRecord.IsFolder,
		CreatedAt:   fileRecord.CreatedAt,
		UpdatedAt:   fileRecord.UpdatedAt,
		Metadata:    metaData.KV,
	}, nil
}

func (s *service) isDescendantOrSelf(ctx context.Context, tx libdb.Exec, tenantID, checkID string, ancestorID string) (bool, error) {
	if checkID == "" {
		return false, nil
	}
	if checkID == ancestorID {
		return true, nil
	}
	const maxDepth = 256
	storeInstance := newFileStore(tx)
	currentParentID := checkID
	seen := make(map[string]bool)
	for depth := 0; ; depth++ {
		if depth > maxDepth || seen[currentParentID] {
			return false, fmt.Errorf("isDescendantOrSelf: circular reference or max depth at %s", currentParentID)
		}
		seen[currentParentID] = true
		parentOfCurrent, err := storeInstance.GetFileParentID(ctx, tenantID, currentParentID)
		if err != nil {
			if errors.Is(err, libdb.ErrNotFound) {
				return false, fmt.Errorf("isDescendantOrSelf: inconsistency, item %s not found while traversing path from %s", currentParentID, checkID)
			}
			return false, fmt.Errorf("isDescendantOrSelf: failed to get parent for %s: %w", currentParentID, err)
		}
		if parentOfCurrent == ancestorID {
			return true, nil
		}
		if parentOfCurrent == "" {
			return false, nil
		}
		currentParentID = parentOfCurrent
	}
}

// --- Service implementation ---

func (s *service) CreateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	if err := s.beforeWrite(ctx, tenantID, ""); err != nil {
		return nil, err
	}
	if file.Name == "" {
		return nil, fmt.Errorf("name is required for files")
	}
	if strings.Contains(file.Name, "/") {
		return nil, fmt.Errorf("filename is not allowed to contain /")
	}
	if int64(len(file.Data)) > MaxUploadSize {
		return nil, fmt.Errorf("file size exceeds the maximum allowed size")
	}

	fileID := uuid.NewString()
	blobID := uuid.NewString()

	hashBytes := sha256.Sum256(file.Data)
	hashString := hex.EncodeToString(hashBytes[:])

	meta := Metadata{
		SpecVersion: "1.0",
		Hash:        hashString,
		Size:        int64(len(file.Data)),
		FileID:      fileID,
		KV:          file.Metadata,
	}
	bMeta, err := json.Marshal(&meta)
	if err != nil {
		return nil, err
	}

	blob := &blobRow{ID: blobID, Data: file.Data, Meta: bMeta}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	storeInstance := newFileStore(tx)

	if err = storeInstance.EnforceMaxFileCount(ctx, tenantID, MaxFilesRowCount); err != nil {
		return nil, fmt.Errorf("too many files in the system: %w", err)
	}
	if err = storeInstance.CreateBlob(ctx, tenantID, blob); err != nil {
		return nil, fmt.Errorf("failed to create blob: %w", err)
	}
	fileRecord := &fileRow{ID: fileID, Type: file.ContentType, Meta: bMeta, BlobsID: blobID}
	if err = storeInstance.CreateFile(ctx, tenantID, fileRecord); err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	if err = storeInstance.CreateFileNameID(ctx, tenantID, fileID, file.ParentID, file.Name); err != nil {
		return nil, fmt.Errorf("failed to create path-id mapping: %w", err)
	}
	resFiles, err := s.getFileByID(ctx, tx, tenantID, fileID, true)
	if err != nil {
		return nil, err
	}
	if err = commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	s.onCreate(ctx, tenantID, resFiles)
	return resFiles, nil
}

func (s *service) GetFolderByID(ctx context.Context, tenantID, id string) (*Folder, error) {
	if err := s.beforeRead(ctx, tenantID, id); err != nil {
		return nil, err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	resFile, err := s.getFileByID(ctx, tx, tenantID, id, false)
	if err != nil {
		return nil, err
	}
	if err := commit(ctx); err != nil {
		return nil, err
	}
	return &Folder{ID: resFile.ID, Name: resFile.Name, ParentID: resFile.ParentID, Path: resFile.Path}, nil
}

func (s *service) GetFileByID(ctx context.Context, tenantID, id string) (*File, error) {
	if err := s.beforeRead(ctx, tenantID, id); err != nil {
		return nil, err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	resFile, err := s.getFileByID(ctx, tx, tenantID, id, true)
	if err != nil {
		return nil, err
	}
	if err := commit(ctx); err != nil {
		return nil, err
	}
	return resFile, nil
}

func (s *service) GetFilesByPath(ctx context.Context, tenantID, path string, page Page) ([]File, *Cursor, error) {
	if path == "/" {
		path = ""
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("GetFilesByPath: failed to start transaction: %w", err)
	}
	defer rTx()

	storeInstance := newFileStore(tx)

	var parentIDForListing string
	var resolvedParentPath string

	if path == "" {
		parentIDForListing = ""
		resolvedParentPath = ""
	} else {
		segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
		currentParentID := ""
		var lastResolvedItemID string
		for _, segmentName := range segments {
			id, err := storeInstance.FindFileIDByName(ctx, tenantID, currentParentID, segmentName)
			if err != nil {
				if errors.Is(err, libdb.ErrNotFound) {
					return nil, nil, ErrUnknownPath
				}
				return nil, nil, fmt.Errorf("GetFilesByPath: failed to resolve path segment '%s': %w", segmentName, err)
			}
			lastResolvedItemID = id
			currentParentID = id
		}
		finalItemRecord, err := storeInstance.GetFileByID(ctx, tenantID, lastResolvedItemID)
		if err != nil {
			return nil, nil, fmt.Errorf("GetFilesByPath: failed to get details for resolved path: %w", err)
		}
		if finalItemRecord.IsFolder {
			parentIDForListing = lastResolvedItemID
			resolvedParentPath = strings.Join(segments, "/")
		} else {
			fileData, err := s.getFileByID(ctx, tx, tenantID, lastResolvedItemID, false)
			if err != nil {
				return nil, nil, err
			}
			if err := commit(ctx); err != nil {
				return nil, nil, fmt.Errorf("GetFilesByPath: failed to commit: %w", err)
			}
			return []File{*fileData}, nil, nil
		}
	}

	children, err := storeInstance.ListChildrenByParentID(ctx, tenantID, parentIDForListing, page)
	if err != nil {
		return nil, nil, fmt.Errorf("GetFilesByPath: failed to list children: %w", err)
	}

	var files []File
	for _, child := range children {
		if err := s.beforeRead(ctx, tenantID, child.ID); err != nil {
			continue
		}
		var childPath string
		if resolvedParentPath == "" {
			childPath = child.Name
		} else {
			childPath = resolvedParentPath + "/" + child.Name
		}
		var meta Metadata
		if err := json.Unmarshal(child.Meta, &meta); err != nil {
			return nil, nil, fmt.Errorf("GetFilesByPath: failed to decode metadata for %s: %w", child.ID, err)
		}
		files = append(files, File{
			ID:          child.ID,
			Path:        childPath,
			Name:        child.Name,
			ContentType: child.Type,
			Size:        meta.Size,
			ParentID:    parentIDForListing,
			CreatedAt:   child.CreatedAt,
			UpdatedAt:   child.UpdatedAt,
			IsDirectory: child.IsFolder,
			Metadata:    meta.KV,
		})
	}

	// A full store page means more rows may follow. Derive the next keyset
	// cursor from the last row read (before beforeRead filtering) so callers can
	// page reliably even when some children are filtered out.
	var next *Cursor
	if len(children) == page.EffectiveLimit() {
		last := children[len(children)-1]
		next = &Cursor{CreatedAt: last.CreatedAt, Name: last.Name, ID: last.ID}
	}

	if err := commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("GetFilesByPath: failed to commit: %w", err)
	}
	return files, next, nil
}

func (s *service) UpdateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	if err := s.beforeWrite(ctx, tenantID, file.ID); err != nil {
		return nil, err
	}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}

	existing, err := newFileStore(tx).GetFileByID(ctx, tenantID, file.ID)
	if err != nil {
		return nil, err
	}
	if existing.IsFolder {
		return nil, fmt.Errorf("UpdateFile: target %s is a folder, use folder operations instead", file.ID)
	}
	if int64(len(file.Data)) > MaxUploadSize {
		return nil, fmt.Errorf("file size exceeds the maximum allowed size")
	}

	hashBytes := sha256.Sum256(file.Data)
	hashString := hex.EncodeToString(hashBytes[:])
	meta := Metadata{
		SpecVersion: "1.0",
		Hash:        hashString,
		Size:        int64(len(file.Data)),
		FileID:      file.ID,
		KV:          file.Metadata,
	}
	bMeta, err := json.Marshal(&meta)
	if err != nil {
		return nil, err
	}

	if err := newFileStore(tx).UpdateBlob(ctx, tenantID, existing.BlobsID, file.Data, bMeta); err != nil {
		return nil, fmt.Errorf("failed to update blob: %w", err)
	}
	updated := &fileRow{
		ID:        file.ID,
		Type:      file.ContentType,
		Meta:      bMeta,
		BlobsID:   existing.BlobsID,
		CreatedAt: file.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	if err := newFileStore(tx).UpdateFile(ctx, tenantID, updated); err != nil {
		return nil, fmt.Errorf("failed to update file record: %w", err)
	}
	res, err := s.getFileByID(ctx, tx, tenantID, file.ID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to reload file: %w", err)
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	s.onUpdate(ctx, tenantID, res)
	return res, nil
}

func (s *service) DeleteFile(ctx context.Context, tenantID, id string) error {
	if err := s.beforeWrite(ctx, tenantID, id); err != nil {
		return err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return err
	}
	storeInstance := newFileStore(tx)

	file, err := storeInstance.GetFileByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file.IsFolder {
		return fmt.Errorf("DeleteFile: target %s is a folder, use DeleteFolder instead", id)
	}
	if err := storeInstance.DeleteBlob(ctx, tenantID, file.BlobsID); err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}
	// Delete the filestree row (child of fk_tree_file) before the vfs_files row.
	// The FK is ON DELETE CASCADE, so deleting vfs_files first would auto-remove
	// the filestree row and DeleteFileNameID would then hit zero rows / ErrNotFound.
	if err := storeInstance.DeleteFileNameID(ctx, tenantID, id); err != nil {
		return fmt.Errorf("failed to delete from file tree: %w", err)
	}
	if err := storeInstance.DeleteFile(ctx, tenantID, id); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	if err := commit(ctx); err != nil {
		return err
	}
	s.onDelete(ctx, tenantID, id)
	return nil
}

func (s *service) CreateFolder(ctx context.Context, tenantID, parentID, name string) (*Folder, error) {
	if err := s.beforeWrite(ctx, tenantID, parentID); err != nil {
		return nil, err
	}
	folderID := uuid.NewString()
	meta := Metadata{SpecVersion: "1.0", FileID: folderID}
	bMeta, err := json.Marshal(&meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	storeInstance := newFileStore(tx)

	if err := storeInstance.EnforceMaxFileCount(ctx, tenantID, MaxFilesRowCount); err != nil {
		return nil, fmt.Errorf("too many files in the system: %w", err)
	}
	folderRecord := &fileRow{ID: folderID, Meta: bMeta, IsFolder: true}
	if err := storeInstance.CreateFile(ctx, tenantID, folderRecord); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	if err = storeInstance.CreateFileNameID(ctx, tenantID, folderID, parentID, name); err != nil {
		return nil, fmt.Errorf("failed to create path-id mapping: %w", err)
	}
	folder, err := s.getFileByID(ctx, tx, tenantID, folderID, false)
	if err != nil {
		return nil, err
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	s.onCreate(ctx, tenantID, folder)
	return &Folder{ID: folderID, Name: name, Path: folder.Path, ParentID: parentID}, nil
}

func (s *service) RenameFile(ctx context.Context, tenantID, fileID, newName string) (*File, error) {
	if err := s.beforeWrite(ctx, tenantID, fileID); err != nil {
		return nil, err
	}
	if strings.Contains(newName, "/") {
		return nil, fmt.Errorf("name cannot contain slashes")
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	storeInstance := newFileStore(tx)

	fileRecord, err := storeInstance.GetFileByID(ctx, tenantID, fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if fileRecord.IsFolder {
		return nil, fmt.Errorf("target is a folder, use RenameFolder instead")
	}
	if err = storeInstance.UpdateFileNameByID(ctx, tenantID, fileID, newName); err != nil {
		return nil, fmt.Errorf("failed to update name %w", err)
	}
	n, err := s.getFileByID(ctx, tx, tenantID, fileID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch changes %w", err)
	}
	if err := commit(ctx); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *service) RenameFolder(ctx context.Context, tenantID, folderID, newName string) (*Folder, error) {
	if err := s.beforeWrite(ctx, tenantID, folderID); err != nil {
		return nil, err
	}
	if strings.Contains(newName, "/") {
		return nil, fmt.Errorf("name cannot contain slashes")
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer rTx()
	if err != nil {
		return nil, err
	}
	storeInstance := newFileStore(tx)

	folderRecord, err := storeInstance.GetFileByID(ctx, tenantID, folderID)
	if err != nil {
		return nil, fmt.Errorf("folder not found: %w", err)
	}
	if !folderRecord.IsFolder {
		return nil, fmt.Errorf("target is not a folder")
	}
	if err = storeInstance.UpdateFileNameByID(ctx, tenantID, folderID, newName); err != nil {
		return nil, fmt.Errorf("failed to update path: %w", err)
	}
	n, err := s.getFileByID(ctx, tx, tenantID, folderID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch changes %w", err)
	}
	if err := commit(ctx); err != nil {
		return nil, err
	}
	return &Folder{ID: folderID, ParentID: n.ParentID, Name: newName, Path: n.Path}, nil
}

func (s *service) DeleteFolder(ctx context.Context, tenantID, folderID string) error {
	if err := s.beforeWrite(ctx, tenantID, folderID); err != nil {
		return err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rTx()
	storeInstance := newFileStore(tx)

	folderRecord, err := storeInstance.GetFileByID(ctx, tenantID, folderID)
	if err != nil {
		return fmt.Errorf("failed to get folder details for ID '%s': %w", folderID, err)
	}
	if !folderRecord.IsFolder {
		return fmt.Errorf("resource with ID '%s' is not a folder", folderID)
	}
	hasChildren, err := storeInstance.HasChildren(ctx, tenantID, folderID)
	if err != nil {
		return fmt.Errorf("failed to check if folder '%s' is empty: %w", folderID, err)
	}
	if hasChildren {
		return ErrFolderNotEmpty
	}
	// Delete the filestree row before vfs_files. The FK is ON DELETE CASCADE,
	// so deleting vfs_files first would auto-remove the filestree row and make
	// the explicit name-mapping delete report ErrNotFound.
	if err = storeInstance.DeleteFileNameID(ctx, tenantID, folderID); err != nil {
		return fmt.Errorf("failed to delete folder name mapping for ID '%s': %w", folderID, err)
	}
	if err = storeInstance.DeleteFile(ctx, tenantID, folderID); err != nil {
		return fmt.Errorf("failed to delete folder record for ID '%s': %w", folderID, err)
	}
	if err = commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction for deleting folder ID '%s': %w", folderID, err)
	}
	s.onDelete(ctx, tenantID, folderID)
	return nil
}

func (s *service) MoveFile(ctx context.Context, tenantID, fileID, newParentID string) (*File, error) {
	if err := s.beforeWrite(ctx, tenantID, fileID); err != nil {
		return nil, err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("MoveFile: failed to start transaction: %w", err)
	}
	defer rTx()

	storeInstance := newFileStore(tx)

	fileRecord, err := storeInstance.GetFileByID(ctx, tenantID, fileID)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, fmt.Errorf("MoveFile: file with ID %s not found", fileID)
		}
		return nil, fmt.Errorf("MoveFile: failed to get file %s: %w", fileID, err)
	}
	if fileRecord.IsFolder {
		return nil, fmt.Errorf("MoveFile: item with ID %s is a folder, use MoveFolder instead", fileID)
	}
	if newParentID != "" {
		parentFolderRecord, err := storeInstance.GetFileByID(ctx, tenantID, newParentID)
		if err != nil {
			if errors.Is(err, libdb.ErrNotFound) {
				return nil, fmt.Errorf("MoveFile: target parent folder with ID %s not found", newParentID)
			}
			return nil, fmt.Errorf("MoveFile: failed to get target parent folder %s: %w", newParentID, err)
		}
		if !parentFolderRecord.IsFolder {
			return nil, fmt.Errorf("MoveFile: target parent with ID %s is not a folder", newParentID)
		}
	}
	currentFileName, err := storeInstance.GetFileNameByID(ctx, tenantID, fileID)
	if err != nil {
		return nil, fmt.Errorf("MoveFile: failed to get current name for file %s: %w", fileID, err)
	}
	originalParentID, err := storeInstance.GetFileParentID(ctx, tenantID, fileID)
	if err != nil && !errors.Is(err, libdb.ErrNotFound) {
		return nil, fmt.Errorf("MoveFile: failed to get original parent for file %s: %w", fileID, err)
	}
	if errors.Is(err, libdb.ErrNotFound) {
		originalParentID = ""
	}

	if originalParentID != newParentID {
		existingID, err := storeInstance.FindFileIDByName(ctx, tenantID, newParentID, currentFileName)
		if err != nil && !errors.Is(err, libdb.ErrNotFound) {
			return nil, fmt.Errorf("MoveFile: failed to check for existing items in target folder: %w", err)
		}
		if err == nil && existingID != fileID {
			return nil, fmt.Errorf("MoveFile: an item named '%s' already exists in the target folder", currentFileName)
		}
	}
	if originalParentID != newParentID {
		if err = storeInstance.UpdateFileParentID(ctx, tenantID, fileID, newParentID); err != nil {
			return nil, fmt.Errorf("MoveFile: failed to move file %s to parent %s: %w", fileID, newParentID, err)
		}
	}
	updatedFile, err := s.getFileByID(ctx, tx, tenantID, fileID, true)
	if err != nil {
		return nil, fmt.Errorf("MoveFile: failed to retrieve updated file details for %s: %w", fileID, err)
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("MoveFile: failed to commit transaction: %w", err)
	}
	return updatedFile, nil
}

func (s *service) MoveFolder(ctx context.Context, tenantID, folderID, newParentID string) (*Folder, error) {
	if err := s.beforeWrite(ctx, tenantID, folderID); err != nil {
		return nil, err
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("MoveFolder: failed to start transaction: %w", err)
	}
	defer rTx()

	storeInstance := newFileStore(tx)

	folderRecord, err := storeInstance.GetFileByID(ctx, tenantID, folderID)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			return nil, fmt.Errorf("MoveFolder: folder with ID %s not found", folderID)
		}
		return nil, fmt.Errorf("MoveFolder: failed to get folder %s: %w", folderID, err)
	}
	if !folderRecord.IsFolder {
		return nil, fmt.Errorf("MoveFolder: item with ID %s is not a folder", folderID)
	}
	if newParentID == folderID {
		return nil, fmt.Errorf("%w: cannot move a folder into itself", ErrInvalidMove)
	}
	if newParentID != "" {
		parentFolderRecord, err := storeInstance.GetFileByID(ctx, tenantID, newParentID)
		if err != nil {
			if errors.Is(err, libdb.ErrNotFound) {
				return nil, fmt.Errorf("MoveFolder: target parent folder with ID %s not found", newParentID)
			}
			return nil, fmt.Errorf("MoveFolder: failed to get target parent folder %s: %w", newParentID, err)
		}
		if !parentFolderRecord.IsFolder {
			return nil, fmt.Errorf("MoveFolder: target parent with ID %s is not a folder", newParentID)
		}
		isCircular, err := s.isDescendantOrSelf(ctx, tx, tenantID, newParentID, folderID)
		if err != nil {
			return nil, fmt.Errorf("MoveFolder: failed to check for circular dependency: %w", err)
		}
		if isCircular {
			return nil, fmt.Errorf("%w: cannot move a folder into one of its own subfolders", ErrInvalidMove)
		}
	}
	currentFolderName, err := storeInstance.GetFileNameByID(ctx, tenantID, folderID)
	if err != nil {
		return nil, fmt.Errorf("MoveFolder: failed to get current name for folder %s: %w", folderID, err)
	}
	originalParentID, err := storeInstance.GetFileParentID(ctx, tenantID, folderID)
	if err != nil && !errors.Is(err, libdb.ErrNotFound) {
		return nil, fmt.Errorf("MoveFolder: failed to get original parent for folder %s: %w", folderID, err)
	}
	if errors.Is(err, libdb.ErrNotFound) {
		originalParentID = ""
	}

	if originalParentID != newParentID {
		existingID, err := storeInstance.FindFileIDByName(ctx, tenantID, newParentID, currentFolderName)
		if err != nil && !errors.Is(err, libdb.ErrNotFound) {
			return nil, fmt.Errorf("MoveFolder: failed to check for existing items in target folder: %w", err)
		}
		if err == nil && existingID != folderID {
			return nil, fmt.Errorf("MoveFolder: an item named '%s' already exists in the target folder", currentFolderName)
		}
	}
	if originalParentID != newParentID {
		if err = storeInstance.UpdateFileParentID(ctx, tenantID, folderID, newParentID); err != nil {
			return nil, fmt.Errorf("MoveFolder: failed to move folder %s to parent %s: %w", folderID, newParentID, err)
		}
	}
	updatedFolderData, err := s.getFileByID(ctx, tx, tenantID, folderID, false)
	if err != nil {
		return nil, fmt.Errorf("MoveFolder: failed to retrieve updated folder details for %s: %w", folderID, err)
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("MoveFolder: failed to commit transaction: %w", err)
	}
	return &Folder{
		ID:       updatedFolderData.ID,
		Name:     currentFolderName,
		Path:     updatedFolderData.Path,
		ParentID: updatedFolderData.ParentID,
	}, nil
}
