package libacp

import "encoding/json"

type ToolKind string

const (
	ToolKindRead       ToolKind = "read"
	ToolKindEdit       ToolKind = "edit"
	ToolKindDelete     ToolKind = "delete"
	ToolKindMove       ToolKind = "move"
	ToolKindSearch     ToolKind = "search"
	ToolKindExecute    ToolKind = "execute"
	ToolKindThink      ToolKind = "think"
	ToolKindFetch      ToolKind = "fetch"
	ToolKindSwitchMode ToolKind = "switch_mode"
	ToolKindOther      ToolKind = "other"
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

// MarshalJSON forces path/newText onto the wire for the "diff" variant even
// when empty — e.g. newText:"" is the correct (and spec-required) shape for a
// diff that clears a file's content, but plain omitempty can't distinguish
// that from "absent" for a string. Every other field, and every other
// ToolCallContent kind, keeps its normal omitempty behavior.
func (c ToolCallContent) MarshalJSON() ([]byte, error) {
	w := struct {
		Type       ToolCallContentKind `json:"type"`
		Content    *ContentBlock       `json:"content,omitempty"`
		Path       *string             `json:"path,omitempty"`
		OldText    string              `json:"oldText,omitempty"`
		NewText    *string             `json:"newText,omitempty"`
		TerminalID string              `json:"terminalId,omitempty"`
		Meta       json.RawMessage     `json:"_meta,omitempty"`
	}{
		Type:       c.Type,
		Content:    c.Content,
		OldText:    c.OldText,
		TerminalID: c.TerminalID,
		Meta:       c.Meta,
	}
	if c.Type == ToolCallContentDiff {
		path, newText := c.Path, c.NewText
		w.Path, w.NewText = &path, &newText
	}
	return json.Marshal(w)
}
