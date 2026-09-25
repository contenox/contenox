package vfs

import "time"

type fileRow struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Meta      []byte    `json:"meta"`
	BlobsID   string    `json:"blobsId,omitempty"`
	IsFolder  bool      `json:"isFolder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type blobRow struct {
	ID        string    `json:"id"`
	Meta      []byte    `json:"meta"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type fileTreeEntry struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
