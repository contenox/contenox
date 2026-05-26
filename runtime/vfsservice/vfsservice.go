// Package vfsservice exposes a tenant-aware file system to chains. Every
// method takes an explicit tenantID; the OSS runtime passes
// runtimetypes.LocalTenantID and proprietary builds pass real tenant values.
package vfsservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/vfsstore"
	"github.com/google/uuid"
)

const (
	// MaxUploadSize is the maximum allowed size for a single file upload (100 MiB).
	MaxUploadSize    = 100 * 1024 * 1024
	MaxFilesRowCount = 50000
)

var (
	ErrUnknownPath    = fmt.Errorf("unable to resolve path")
	ErrFolderNotEmpty = fmt.Errorf("folder is not empty")
	ErrNotSupported   = errors.New("operation not supported")
	// ErrUnauthorized is the canonical error proprietary Callbacks should
	// return when a tenant is not permitted to perform an operation.
	ErrUnauthorized = errors.New("unauthorized")
)

// Callbacks holds optional lifecycle hooks called around VFS operations.
// Every hook receives the tenantID the operation is running under so a single
// Callbacks value can serve many tenants. Any nil field is silently skipped.
type Callbacks struct {
	BeforeRead  func(ctx context.Context, tenantID, resourceID string) error
	BeforeWrite func(ctx context.Context, tenantID, resourceID string) error
	OnCreate    func(ctx context.Context, tenantID string, file *File) error
	OnUpdate    func(ctx context.Context, tenantID string, file *File) error
	OnDelete    func(ctx context.Context, tenantID, resourceID string) error
}

// Service defines all VFS operations. Every method takes tenantID explicitly.
type Service interface {
	CreateFile(ctx context.Context, tenantID string, file *File) (*File, error)
	GetFileByID(ctx context.Context, tenantID, id string) (*File, error)
	GetFolderByID(ctx context.Context, tenantID, id string) (*Folder, error)
	GetFilesByPath(ctx context.Context, tenantID, path string) ([]File, error)
	UpdateFile(ctx context.Context, tenantID string, file *File) (*File, error)
	DeleteFile(ctx context.Context, tenantID, id string) error
	CreateFolder(ctx context.Context, tenantID, parentID, name string) (*Folder, error)
	RenameFile(ctx context.Context, tenantID, fileID, newName string) (*File, error)
	RenameFolder(ctx context.Context, tenantID, folderID, newName string) (*Folder, error)
	DeleteFolder(ctx context.Context, tenantID, folderID string) error
	MoveFile(ctx context.Context, tenantID, fileID, newParentID string) (*File, error)
	MoveFolder(ctx context.Context, tenantID, folderID, newParentID string) (*Folder, error)
}

var _ Service = (*service)(nil)

type service struct {
	db libdb.DBManager
	cb Callbacks
}

// New creates a DB-backed VFS service. Pass Callbacks{} for pure storage.
func New(db libdb.DBManager, cb Callbacks) Service {
	return &service{db: db, cb: cb}
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

func (s *service) onCreate(ctx context.Context, tenantID string, f *File) {
	if s.cb.OnCreate != nil {
		_ = s.cb.OnCreate(ctx, tenantID, f)
	}
}

func (s *service) onUpdate(ctx context.Context, tenantID string, f *File) {
	if s.cb.OnUpdate != nil {
		_ = s.cb.OnUpdate(ctx, tenantID, f)
	}
}

func (s *service) onDelete(ctx context.Context, tenantID, id string) {
	if s.cb.OnDelete != nil {
		_ = s.cb.OnDelete(ctx, tenantID, id)
	}
}

// --- internal helpers ---

func (s *service) getFileByID(ctx context.Context, tx libdb.Exec, tenantID, id string, withBlob bool) (*File, error) {
	storeInstance := vfsstore.New(tx)
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
		CreatedAt:   fileRecord.CreatedAt,
		UpdatedAt:   fileRecord.UpdatedAt,
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
	storeInstance := vfsstore.New(tx)
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
	}
	bMeta, err := json.Marshal(&meta)
	if err != nil {
		return nil, err
	}

	blob := &vfsstore.Blob{ID: blobID, Data: file.Data, Meta: bMeta}

	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
	if err != nil {
		return nil, err
	}
	storeInstance := vfsstore.New(tx)

	if err = storeInstance.EnforceMaxFileCount(ctx, tenantID, MaxFilesRowCount); err != nil {
		return nil, fmt.Errorf("too many files in the system: %w", err)
	}
	if err = storeInstance.CreateFileNameID(ctx, tenantID, fileID, file.ParentID, file.Name); err != nil {
		return nil, fmt.Errorf("failed to create path-id mapping: %w", err)
	}
	if err = storeInstance.CreateBlob(ctx, tenantID, blob); err != nil {
		return nil, fmt.Errorf("failed to create blob: %w", err)
	}
	fileRecord := &vfsstore.File{ID: fileID, Type: file.ContentType, Meta: bMeta, BlobsID: blobID}
	if err = storeInstance.CreateFile(ctx, tenantID, fileRecord); err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
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
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
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
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
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

func (s *service) GetFilesByPath(ctx context.Context, tenantID, path string) ([]File, error) {
	if path == "/" {
		path = ""
	}
	tx, commit, rTx, err := s.db.WithTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetFilesByPath: failed to start transaction: %w", err)
	}
	defer func() {
		if err := rTx(); err != nil {
			log.Printf("GetFilesByPath: failed to rollback transaction: %v", err)
		}
	}()

	storeInstance := vfsstore.New(tx)

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
			idsInSegment, err := storeInstance.ListFileIDsByName(ctx, tenantID, currentParentID, segmentName)
			if err != nil {
				return nil, fmt.Errorf("GetFilesByPath: failed to resolve path segment '%s': %w", segmentName, err)
			}
			if len(idsInSegment) == 0 {
				return nil, ErrUnknownPath
			}
			if len(idsInSegment) > 1 {
				return nil, fmt.Errorf("GetFilesByPath: ambiguous path, multiple items named '%s'", segmentName)
			}
			lastResolvedItemID = idsInSegment[0]
			currentParentID = lastResolvedItemID
		}
		finalItemRecord, err := storeInstance.GetFileByID(ctx, tenantID, lastResolvedItemID)
		if err != nil {
			return nil, fmt.Errorf("GetFilesByPath: failed to get details for resolved path: %w", err)
		}
		if finalItemRecord.IsFolder {
			parentIDForListing = lastResolvedItemID
			resolvedParentPath = strings.Join(segments, "/")
		} else {
			fileData, err := s.getFileByID(ctx, tx, tenantID, lastResolvedItemID, false)
			if err != nil {
				return nil, err
			}
			if err := commit(ctx); err != nil {
				return nil, fmt.Errorf("GetFilesByPath: failed to commit: %w", err)
			}
			return []File{*fileData}, nil
		}
	}

	children, err := storeInstance.ListChildrenByParentID(ctx, tenantID, parentIDForListing)
	if err != nil {
		return nil, fmt.Errorf("GetFilesByPath: failed to list children: %w", err)
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
		_ = json.Unmarshal(child.Meta, &meta)
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
		})
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("GetFilesByPath: failed to commit: %w", err)
	}
	return files, nil
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

	existing, err := vfsstore.New(tx).GetFileByID(ctx, tenantID, file.ID)
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
	}
	bMeta, err := json.Marshal(&meta)
	if err != nil {
		return nil, err
	}

	if err := vfsstore.New(tx).UpdateBlob(ctx, tenantID, existing.BlobsID, file.Data, bMeta); err != nil {
		return nil, fmt.Errorf("failed to update blob: %w", err)
	}
	updated := &vfsstore.File{
		ID:        file.ID,
		Type:      file.ContentType,
		Meta:      bMeta,
		BlobsID:   existing.BlobsID,
		CreatedAt: file.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	if err := vfsstore.New(tx).UpdateFile(ctx, tenantID, updated); err != nil {
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
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
	if err != nil {
		return err
	}
	storeInstance := vfsstore.New(tx)

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
	if err := storeInstance.DeleteFile(ctx, tenantID, id); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	if err := storeInstance.DeleteFileNameID(ctx, tenantID, id); err != nil {
		return fmt.Errorf("failed to delete from file tree: %w", err)
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
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
	if err != nil {
		return nil, err
	}
	storeInstance := vfsstore.New(tx)

	if err := storeInstance.EnforceMaxFileCount(ctx, tenantID, MaxFilesRowCount); err != nil {
		return nil, fmt.Errorf("too many files in the system: %w", err)
	}
	if err = storeInstance.CreateFileNameID(ctx, tenantID, folderID, parentID, name); err != nil {
		return nil, fmt.Errorf("failed to create path-id mapping: %w", err)
	}
	folderRecord := &vfsstore.File{ID: folderID, Meta: bMeta, IsFolder: true}
	if err := storeInstance.CreateFile(ctx, tenantID, folderRecord); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	folder, err := s.getFileByID(ctx, tx, tenantID, folderID, false)
	if err != nil {
		return nil, err
	}
	if err := commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
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
	storeInstance := vfsstore.New(tx)

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
	storeInstance := vfsstore.New(tx)

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
	defer func() {
		if err := rTx(); err != nil {
			log.Println("failed to rollback transaction", err)
		}
	}()
	storeInstance := vfsstore.New(tx)

	folderRecord, err := storeInstance.GetFileByID(ctx, tenantID, folderID)
	if err != nil {
		return fmt.Errorf("failed to get folder details for ID '%s': %w", folderID, err)
	}
	if !folderRecord.IsFolder {
		return fmt.Errorf("resource with ID '%s' is not a folder", folderID)
	}
	children, err := storeInstance.ListFileIDsByParentID(ctx, tenantID, folderID)
	if err != nil {
		return fmt.Errorf("failed to check if folder '%s' is empty: %w", folderID, err)
	}
	if len(children) > 0 {
		return ErrFolderNotEmpty
	}
	if err = storeInstance.DeleteFile(ctx, tenantID, folderID); err != nil {
		return fmt.Errorf("failed to delete folder record for ID '%s': %w", folderID, err)
	}
	if err = storeInstance.DeleteFileNameID(ctx, tenantID, folderID); err != nil {
		return fmt.Errorf("failed to delete folder name mapping for ID '%s': %w", folderID, err)
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
	defer func() {
		if err := rTx(); err != nil {
			log.Printf("MoveFile: failed to rollback transaction: %v", err)
		}
	}()

	storeInstance := vfsstore.New(tx)

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
		existingIDsInNewParent, err := storeInstance.ListFileIDsByName(ctx, tenantID, newParentID, currentFileName)
		if err != nil {
			return nil, fmt.Errorf("MoveFile: failed to check for existing items in target folder: %w", err)
		}
		for _, existingID := range existingIDsInNewParent {
			if existingID != fileID {
				return nil, fmt.Errorf("MoveFile: an item named '%s' already exists in the target folder", currentFileName)
			}
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
	defer func() {
		if err := rTx(); err != nil {
			log.Printf("MoveFolder: failed to rollback transaction: %v", err)
		}
	}()

	storeInstance := vfsstore.New(tx)

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
		return nil, fmt.Errorf("MoveFolder: cannot move a folder into itself (folderID: %s, newParentID: %s)", folderID, newParentID)
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
			return nil, fmt.Errorf("MoveFolder: cannot move folder %s into itself or one of its subfolders (target %s)", folderID, newParentID)
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
		existingIDsInNewParent, err := storeInstance.ListFileIDsByName(ctx, tenantID, newParentID, currentFolderName)
		if err != nil {
			return nil, fmt.Errorf("MoveFolder: failed to check for existing items in target folder: %w", err)
		}
		for _, existingID := range existingIDsInNewParent {
			if existingID != folderID {
				return nil, fmt.Errorf("MoveFolder: an item named '%s' already exists in the target folder", currentFolderName)
			}
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
