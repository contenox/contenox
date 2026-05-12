package libacp

import "encoding/json"

type SessionID string

type NewSessionRequest struct {
	Cwd        string          `json:"cwd"`
	McpServers []McpServer     `json:"mcpServers"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type NewSessionResponse struct {
	SessionID SessionID       `json:"sessionId"`
	Modes     []SessionMode   `json:"modes,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type LoadSessionRequest struct {
	SessionID  SessionID       `json:"sessionId"`
	Cwd        string          `json:"cwd"`
	McpServers []McpServer     `json:"mcpServers"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type LoadSessionResponse struct {
	Modes []SessionMode   `json:"modes,omitempty"`
	Meta  json.RawMessage `json:"_meta,omitempty"`
}

type SessionMode struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

type CancelNotification struct {
	SessionID SessionID       `json:"sessionId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type ListSessionsRequest struct {
	Cwd    string          `json:"cwd,omitempty"`
	Cursor string          `json:"cursor,omitempty"`
	Meta   json.RawMessage `json:"_meta,omitempty"`
}

type SessionInfo struct {
	SessionID SessionID       `json:"sessionId"`
	Cwd       string          `json:"cwd,omitempty"`
	Title     string          `json:"title,omitempty"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type ListSessionsResponse struct {
	Sessions   []SessionInfo   `json:"sessions"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}
