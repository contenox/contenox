package vfsapi

import "time"

// fileResp is a file's metadata as the wire carries it. Content is served by
// the download route, never inline in a listing.
type fileResp struct {
	ID          string            `json:"id"`
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	ContentType string            `json:"contentType,omitempty"`
	Size        int64             `json:"size"`
	ParentID    string            `json:"parentId,omitempty"`
	IsDirectory bool              `json:"isDirectory,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type folderResp struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parentId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type nameBody struct {
	Name string `json:"name"`
}

type moveBody struct {
	NewParentID string `json:"newParentId"`
}

type folderCreateBody struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
}

type accessEntryBody struct {
	Identity     string `json:"identity"`
	Resource     string `json:"resource"`
	ResourceType string `json:"resourceType"`
	Permission   string `json:"permission"`
}

type accessEntryResp struct {
	ID           string    `json:"id"`
	Identity     string    `json:"identity"`
	Resource     string    `json:"resource"`
	ResourceType string    `json:"resourceType"`
	Permission   string    `json:"permission"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type permissionsResp struct {
	Permissions []string `json:"permissions"`
}

type accessEntriesListResp struct {
	Entries []accessEntryResp `json:"entries"`
}

type messageResp struct {
	Message string `json:"message"`
}
