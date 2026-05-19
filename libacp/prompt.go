package libacp

import "encoding/json"

type StopReason string

const (
	StopReasonEndTurn         StopReason = "end_turn"
	StopReasonMaxTokens       StopReason = "max_tokens"
	StopReasonMaxTurnRequests StopReason = "max_turn_requests"
	StopReasonRefusal         StopReason = "refusal"
	StopReasonCancelled       StopReason = "cancelled"
)

type PromptRequest struct {
	SessionID SessionID       `json:"sessionId"`
	Prompt    []ContentBlock  `json:"prompt"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type PromptResponse struct {
	StopReason StopReason      `json:"stopReason"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type SessionUpdateKind string

const (
	SessionUpdateUserMessageChunk  SessionUpdateKind = "user_message_chunk"
	SessionUpdateAgentMessageChunk SessionUpdateKind = "agent_message_chunk"
	SessionUpdateAgentThoughtChunk SessionUpdateKind = "agent_thought_chunk"
	SessionUpdateToolCall          SessionUpdateKind = "tool_call"
	SessionUpdateToolCallUpdate    SessionUpdateKind = "tool_call_update"
	SessionUpdatePlan              SessionUpdateKind = "plan"
	SessionUpdateAvailableCommands SessionUpdateKind = "available_commands_update"
	SessionUpdateCurrentMode       SessionUpdateKind = "current_mode_update"
)

type SessionUpdate struct {
	SessionUpdate SessionUpdateKind `json:"sessionUpdate"`

	Content *ContentBlock `json:"-"`

	ToolCallID  string             `json:"toolCallId,omitempty"`
	Title       string             `json:"title,omitempty"`
	Kind        ToolKind           `json:"kind,omitempty"`
	Status      ToolCallStatus     `json:"status,omitempty"`
	ToolContent []ToolCallContent  `json:"-"`
	Locations   []ToolCallLocation `json:"locations,omitempty"`
	RawInput    json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput   json.RawMessage    `json:"rawOutput,omitempty"`

	Entries []PlanEntry `json:"entries,omitempty"`

	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`

	CurrentModeID string `json:"currentModeId,omitempty"`

	Meta json.RawMessage `json:"_meta,omitempty"`
}

type sessionUpdateWire struct {
	SessionUpdate SessionUpdateKind `json:"sessionUpdate"`

	Content json.RawMessage `json:"content,omitempty"`

	ToolCallID string             `json:"toolCallId,omitempty"`
	Title      string             `json:"title,omitempty"`
	Kind       ToolKind           `json:"kind,omitempty"`
	Status     ToolCallStatus     `json:"status,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
	RawInput   json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage    `json:"rawOutput,omitempty"`

	Entries []PlanEntry `json:"entries,omitempty"`

	AvailableCommands []AvailableCommand `json:"availableCommands,omitempty"`

	CurrentModeID string `json:"currentModeId,omitempty"`

	Meta json.RawMessage `json:"_meta,omitempty"`
}

func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	w := sessionUpdateWire{
		SessionUpdate:     u.SessionUpdate,
		ToolCallID:        u.ToolCallID,
		Title:             u.Title,
		Kind:              u.Kind,
		Status:            u.Status,
		Locations:         u.Locations,
		RawInput:          u.RawInput,
		RawOutput:         u.RawOutput,
		Entries:           u.Entries,
		AvailableCommands: u.AvailableCommands,
		CurrentModeID:     u.CurrentModeID,
		Meta:              u.Meta,
	}
	switch u.SessionUpdate {
	case SessionUpdateToolCall, SessionUpdateToolCallUpdate:
		if len(u.ToolContent) > 0 {
			raw, err := json.Marshal(u.ToolContent)
			if err != nil {
				return nil, err
			}
			w.Content = raw
		}
	default:
		if u.Content != nil {
			raw, err := json.Marshal(u.Content)
			if err != nil {
				return nil, err
			}
			w.Content = raw
		}
	}
	return json.Marshal(w)
}

func (u *SessionUpdate) UnmarshalJSON(data []byte) error {
	var w sessionUpdateWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*u = SessionUpdate{
		SessionUpdate:     w.SessionUpdate,
		ToolCallID:        w.ToolCallID,
		Title:             w.Title,
		Kind:              w.Kind,
		Status:            w.Status,
		Locations:         w.Locations,
		RawInput:          w.RawInput,
		RawOutput:         w.RawOutput,
		Entries:           w.Entries,
		AvailableCommands: w.AvailableCommands,
		CurrentModeID:     w.CurrentModeID,
		Meta:              w.Meta,
	}
	if len(w.Content) == 0 {
		return nil
	}
	switch w.SessionUpdate {
	case SessionUpdateToolCall, SessionUpdateToolCallUpdate:
		return json.Unmarshal(w.Content, &u.ToolContent)
	default:
		var cb ContentBlock
		if err := json.Unmarshal(w.Content, &cb); err != nil {
			return err
		}
		u.Content = &cb
	}
	return nil
}

type SessionNotification struct {
	SessionID SessionID       `json:"sessionId"`
	Update    SessionUpdate   `json:"update"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

func NewAgentMessageChunk(text string) SessionUpdate {
	c := NewTextContent(text)
	return SessionUpdate{SessionUpdate: SessionUpdateAgentMessageChunk, Content: &c}
}

func NewAgentThoughtChunk(text string) SessionUpdate {
	c := NewTextContent(text)
	return SessionUpdate{SessionUpdate: SessionUpdateAgentThoughtChunk, Content: &c}
}

func NewUserMessageChunk(text string) SessionUpdate {
	c := NewTextContent(text)
	return SessionUpdate{SessionUpdate: SessionUpdateUserMessageChunk, Content: &c}
}
