package vfsservice

import (
	"context"
	"fmt"

	"github.com/contenox/agent/libtracker"
)

type activityTrackerDecorator struct {
	svc     Service
	tracker libtracker.ActivityTracker
}

// WithActivityTracker wraps a Service with activity logging.
func WithActivityTracker(svc Service, tracker libtracker.ActivityTracker) Service {
	return &activityTrackerDecorator{svc: svc, tracker: tracker}
}

var _ Service = (*activityTrackerDecorator)(nil)

func sanitizeFile(f *File) *File {
	if f == nil {
		return nil
	}
	return &File{
		ID:          f.ID,
		Path:        f.Path,
		Name:        f.Name,
		ParentID:    f.ParentID,
		Size:        f.Size,
		ContentType: f.ContentType,
		IsDirectory: f.IsDirectory,
	}
}

func (d *activityTrackerDecorator) CreateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "create", "file",
		"tenantID", tenantID, "path", file.Path, "contentType", file.ContentType, "size", fmt.Sprintf("%d", file.Size))
	defer end()
	result, err := d.svc.CreateFile(ctx, tenantID, file)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, sanitizeFile(result))
	}
	return result, err
}

func (d *activityTrackerDecorator) GetFileByID(ctx context.Context, tenantID, id string) (*File, error) {
	reportErr, _, end := d.tracker.Start(ctx, "read", "file", "tenantID", tenantID, "fileID", id)
	defer end()
	result, err := d.svc.GetFileByID(ctx, tenantID, id)
	if err != nil {
		reportErr(err)
	}
	return result, err
}

func (d *activityTrackerDecorator) GetFolderByID(ctx context.Context, tenantID, id string) (*Folder, error) {
	reportErr, _, end := d.tracker.Start(ctx, "read", "folder", "tenantID", tenantID, "folderID", id)
	defer end()
	result, err := d.svc.GetFolderByID(ctx, tenantID, id)
	if err != nil {
		reportErr(err)
	}
	return result, err
}

func (d *activityTrackerDecorator) GetFilesByPath(ctx context.Context, tenantID, path string) ([]File, error) {
	reportErr, _, end := d.tracker.Start(ctx, "read", "file", "tenantID", tenantID, "path", path)
	defer end()
	result, err := d.svc.GetFilesByPath(ctx, tenantID, path)
	if err != nil {
		reportErr(err)
	}
	return result, err
}

func (d *activityTrackerDecorator) UpdateFile(ctx context.Context, tenantID string, file *File) (*File, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "update", "file",
		"tenantID", tenantID, "fileID", file.ID, "contentType", file.ContentType, "size", fmt.Sprintf("%d", file.Size))
	defer end()
	result, err := d.svc.UpdateFile(ctx, tenantID, file)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, sanitizeFile(result))
	}
	return result, err
}

func (d *activityTrackerDecorator) DeleteFile(ctx context.Context, tenantID, id string) error {
	reportErr, reportChange, end := d.tracker.Start(ctx, "delete", "file", "tenantID", tenantID, "fileID", id)
	defer end()
	err := d.svc.DeleteFile(ctx, tenantID, id)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(id, nil)
	}
	return err
}

func (d *activityTrackerDecorator) CreateFolder(ctx context.Context, tenantID, parentID, name string) (*Folder, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "create", "folder", "tenantID", tenantID, "name", name)
	defer end()
	result, err := d.svc.CreateFolder(ctx, tenantID, parentID, name)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, result)
	}
	return result, err
}

func (d *activityTrackerDecorator) RenameFile(ctx context.Context, tenantID, fileID, newName string) (*File, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "rename", "file", "tenantID", tenantID, "fileID", fileID, "newName", newName)
	defer end()
	result, err := d.svc.RenameFile(ctx, tenantID, fileID, newName)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, sanitizeFile(result))
	}
	return result, err
}

func (d *activityTrackerDecorator) RenameFolder(ctx context.Context, tenantID, folderID, newName string) (*Folder, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "rename", "folder", "tenantID", tenantID, "folderID", folderID, "newName", newName)
	defer end()
	result, err := d.svc.RenameFolder(ctx, tenantID, folderID, newName)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, result)
	}
	return result, err
}

func (d *activityTrackerDecorator) DeleteFolder(ctx context.Context, tenantID, folderID string) error {
	reportErr, reportChange, end := d.tracker.Start(ctx, "delete", "folder", "tenantID", tenantID, "folderID", folderID)
	defer end()
	err := d.svc.DeleteFolder(ctx, tenantID, folderID)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(folderID, nil)
	}
	return err
}

func (d *activityTrackerDecorator) MoveFile(ctx context.Context, tenantID, fileID, newParentID string) (*File, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "move", "file", "tenantID", tenantID, "fileID", fileID, "newParentID", newParentID)
	defer end()
	result, err := d.svc.MoveFile(ctx, tenantID, fileID, newParentID)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, sanitizeFile(result))
	}
	return result, err
}

func (d *activityTrackerDecorator) MoveFolder(ctx context.Context, tenantID, folderID, newParentID string) (*Folder, error) {
	reportErr, reportChange, end := d.tracker.Start(ctx, "move", "folder", "tenantID", tenantID, "folderID", folderID, "newParentID", newParentID)
	defer end()
	result, err := d.svc.MoveFolder(ctx, tenantID, folderID, newParentID)
	if err != nil {
		reportErr(err)
	} else {
		reportChange(result.ID, result)
	}
	return result, err
}
