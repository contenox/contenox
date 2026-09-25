package vfs

import "time"

// File represents a file in the VFS.
type File struct {
	// ID is the stable handle for the file. The database backend uses a UUID;
	// the local-filesystem backend uses the tenant-relative path.
	ID string `json:"id"`
	// Path is the resolved slash-separated location from the tenant root.
	Path string `json:"path"`
	// Name is the final path segment.
	Name string `json:"name"`
	// ParentID is the containing folder's ID, or "" for the tenant root.
	ParentID string `json:"parentId"`
	// Size is the content length in bytes.
	Size int64 `json:"size"`
	// ContentType is the caller-supplied MIME type.
	ContentType string `json:"contentType"`
	// Data is the file content; it is populated only by reads that request it.
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// IsDirectory is set for directory entries returned by GetFilesByPath; omitted for normal files.
	IsDirectory bool `json:"isDirectory,omitempty"`
	// Metadata is caller-supplied key/value metadata stored alongside the file,
	// independent of its content. CreateFile and UpdateFile persist it as-is
	// (UpdateFile replaces it wholesale); reads — including GetFilesByPath
	// listings — surface it back. Connectors use it to stash a source-version
	// token so idempotency survives source→stored content transforms. Nil when
	// none was set. Persisted by the DB-backed Service (New); the
	// local-filesystem backend, like ContentType, does not retain it.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Folder represents a directory in the VFS.
type Folder struct {
	// ID is the stable handle for the folder; see File.ID for backend semantics.
	ID string `json:"id"`
	// Path is the resolved slash-separated location from the tenant root.
	Path string `json:"path"`
	// Name is the final path segment.
	Name string `json:"name"`
	// ParentID is the containing folder's ID, or "" for the tenant root.
	ParentID  string    `json:"parentId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Metadata holds file content metadata stored alongside blob data.
type Metadata struct {
	SpecVersion string `json:"specVersion"`
	Path        string `json:"path"`
	Hash        string `json:"hash"`
	Size        int64  `json:"size"`
	FileID      string `json:"fileId"`
	// KV is caller-supplied key/value metadata (File.Metadata). Omitted from the
	// stored JSON when empty, so existing rows decode to a nil map.
	KV map[string]string `json:"kv,omitempty"`
}
