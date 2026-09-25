package acpsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/services/localfileservice"
	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/libacp"
	libdb "github.com/contenox/contenox/libdbexec"
)

const (
	extMethodFSStat      = "_contenox/fs/stat"
	extMethodFSReadDir   = "_contenox/fs/read_dir"
	extMethodFSReadFile  = "_contenox/fs/read_file"
	extMethodFSWriteFile = "_contenox/fs/write_file"
	extMethodFSCreateDir = "_contenox/fs/create_directory"
	extMethodFSRename    = "_contenox/fs/rename"
	extMethodFSDelete    = "_contenox/fs/delete"
)

type fsEntry struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"modTime,omitempty"`
}

// hostFiles returns the vfs-rooted file service the fs/git families serve.
// ok=false is the editor-driven shape, which owns its filesystem client-side,
// so those methods answer MethodNotFound.
func (t *Transport) hostFiles() (localfileservice.Service, bool) {
	if t.deps.Files == nil {
		return nil, false
	}
	return t.deps.Files, true
}

// workspaceRel converts a client path — absolute inside the host root, or
// relative to it — into the file service's relative form; "" and "/" name the
// workspace root and become ".".
func workspaceRel(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." || raw == "/" {
		return ".", nil
	}
	if strings.HasPrefix(raw, "/") {
		abs := filepath.Clean(raw)
		if !vfs.Within(root, abs) {
			return "", vfs.ErrEscape
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return "", vfs.ErrEscape
		}
		return filepath.ToSlash(rel), nil
	}
	return raw, nil
}

// fsServiceErr maps a file-service error to the wire error a surface can
// classify: a missing path becomes ErrResourceNotFound (IsNotFound and
// AsNotExist recognize it), a path-shaped problem stays InvalidParams, and
// everything else is an internal error.
func fsServiceErr(op string, err error) *libacp.Error {
	switch {
	case errors.Is(err, libdb.ErrNotFound):
		return libacp.NewError(libacp.ErrResourceNotFound, op+": "+err.Error())
	case errors.Is(err, localfileservice.ErrInvalidPath):
		return libacp.InvalidParams(op + ": " + err.Error())
	default:
		return libacp.InternalError(op + ": " + err.Error())
	}
}

func (t *Transport) handleFSStat(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/stat: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSStat + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/stat: " + err.Error())
	}
	if rel == "." {
		out, _ := json.Marshal(fsEntry{Name: filepath.Base(svc.Root()), IsDir: true})
		return out, nil
	}
	entry, err := svc.Stat(ctx, rel)
	if err != nil {
		return nil, fsServiceErr("fs/stat", err)
	}
	out, _ := json.Marshal(fsEntry{Name: entry.Name, IsDir: entry.IsDirectory, Size: entry.Size, ModTime: entry.UpdatedAt})
	return out, nil
}

func (t *Transport) handleFSReadDir(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/read_dir: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSReadDir + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/read_dir: " + err.Error())
	}
	entries, err := svc.List(ctx, rel)
	if err != nil {
		return nil, fsServiceErr("fs/read_dir", err)
	}
	rows := make([]fsEntry, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, fsEntry{Name: e.Name, IsDir: e.IsDirectory, Size: e.Size, ModTime: e.UpdatedAt})
	}
	out, _ := json.Marshal(struct {
		Entries []fsEntry `json:"entries"`
	}{Entries: rows})
	return out, nil
}

func (t *Transport) handleFSReadFile(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/read_file: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSReadFile + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/read_file: " + err.Error())
	}
	data, entry, err := svc.Read(ctx, rel)
	if err != nil {
		return nil, fsServiceErr("fs/read_file", err)
	}
	out, _ := json.Marshal(struct {
		Content string `json:"content"`
		IsDir   bool   `json:"isDir"`
	}{Content: base64.StdEncoding.EncodeToString(data), IsDir: entry.IsDirectory})
	return out, nil
}

func (t *Transport) handleFSWriteFile(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Create    bool   `json:"create"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/write_file: " + err.Error())
	}
	if !p.Create && !p.Overwrite {
		return nil, libacp.InvalidParams("fs/write_file: create or overwrite must be set")
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSWriteFile + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/write_file: " + err.Error())
	}
	if rel == "." {
		return nil, libacp.InvalidParams("fs/write_file: the workspace root is not writable")
	}
	raw, err := base64.StdEncoding.DecodeString(p.Content)
	if err != nil {
		return nil, libacp.InvalidParams("fs/write_file: content is not base64")
	}
	_, statErr := svc.Stat(ctx, rel)
	switch {
	case errors.Is(statErr, libdb.ErrNotFound):
		if !p.Create {
			return nil, libacp.InvalidParams("fs/write_file: file does not exist and create is false")
		}
	case statErr != nil:
		return nil, fsServiceErr("fs/write_file", statErr)
	default:
		if !p.Overwrite {
			return nil, libacp.InvalidParams("fs/write_file: file exists and overwrite is false")
		}
	}
	if _, err := svc.Write(ctx, rel, raw, false); err != nil {
		return nil, fsServiceErr("fs/write_file", err)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (t *Transport) handleFSCreateDir(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/create_directory: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSCreateDir + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/create_directory: " + err.Error())
	}
	if rel == "." {
		return nil, libacp.InvalidParams("fs/create_directory: the workspace root already exists")
	}
	if _, err := svc.Mkdir(ctx, rel); err != nil {
		return nil, fsServiceErr("fs/create_directory", err)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (t *Transport) handleFSRename(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/rename: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSRename + " requires the host workspace")
	}
	oldRel, err := workspaceRel(svc.Root(), p.OldPath)
	if err != nil {
		return nil, libacp.InvalidParams("fs/rename: " + err.Error())
	}
	if oldRel == "." {
		return nil, libacp.InvalidParams("fs/rename: the workspace root cannot be renamed")
	}
	newRel, err := workspaceRel(svc.Root(), p.NewPath)
	if err != nil {
		return nil, libacp.InvalidParams("fs/rename: " + err.Error())
	}
	if newRel == "." {
		return nil, libacp.InvalidParams("fs/rename: the workspace root cannot be a rename target")
	}
	if _, err := svc.Move(ctx, oldRel, newRel); err != nil {
		return nil, fsServiceErr("fs/rename", err)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (t *Transport) handleFSDelete(ctx context.Context, params json.RawMessage) (json.RawMessage, *libacp.Error) {
	var p struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, libacp.InvalidParams("fs/delete: " + err.Error())
	}
	svc, ok := t.hostFiles()
	if !ok {
		return nil, libacp.MethodNotFound(extMethodFSDelete + " requires the host workspace")
	}
	rel, err := workspaceRel(svc.Root(), p.Path)
	if err != nil {
		return nil, libacp.InvalidParams("fs/delete: " + err.Error())
	}
	if rel == "." {
		return nil, libacp.InvalidParams("fs/delete: the workspace root cannot be deleted")
	}
	entry, err := svc.Stat(ctx, rel)
	if err != nil {
		return nil, fsServiceErr("fs/delete", err)
	}
	if entry.IsDirectory && !p.Recursive {
		children, listErr := svc.List(ctx, rel)
		if listErr != nil {
			return nil, fsServiceErr("fs/delete", listErr)
		}
		if len(children) > 0 {
			return nil, libacp.InvalidParams("fs/delete: directory is not empty")
		}
	}
	if err := svc.Delete(ctx, rel); err != nil {
		return nil, fsServiceErr("fs/delete", err)
	}
	return json.RawMessage(`{"ok":true}`), nil
}
