package libacp

import "encoding/json"

type ToolKind string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindDelete  ToolKind = "delete"
	ToolKindMove    ToolKind = "move"
	ToolKindSearch  ToolKind = "search"
	ToolKindExecute ToolKind = "execute"
	ToolKindThink   ToolKind = "think"
	ToolKindFetch   ToolKind = "fetch"
	ToolKindOther   ToolKind = "other"
)

type ToolCallStatus string

const (
	ToolCallStatusPending    ToolCallStatus = "pending"
	ToolCallStatusInProgress ToolCallStatus = "in_progress"
	ToolCallStatusCompleted  ToolCallStatus = "completed"
	ToolCallStatusFailed     ToolCallStatus = "failed"
)

type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

type ToolCallContentKind string

const (
	ToolCallContentRegular  ToolCallContentKind = "content"
	ToolCallContentDiff     ToolCallContentKind = "diff"
	ToolCallContentTerminal ToolCallContentKind = "terminal"
)

type ToolCallContent struct {
	Type       ToolCallContentKind `json:"type"`
	Content    *ContentBlock       `json:"content,omitempty"`
	Path       string              `json:"path,omitempty"`
	OldText    string              `json:"oldText,omitempty"`
	NewText    string              `json:"newText,omitempty"`
	TerminalID string              `json:"terminalId,omitempty"`
	Meta       json.RawMessage     `json:"_meta,omitempty"`
}
