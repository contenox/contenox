package libacp

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type SessionID string

type NewSessionRequest struct {
	Cwd string `json:"cwd"`
	// AdditionalDirectories are extra workspace roots for this session, on top
	// of Cwd. Each path must be absolute. Omitted or empty means none.
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	McpServers            []McpServer     `json:"mcpServers"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type NewSessionResponse struct {
	SessionID     SessionID             `json:"sessionId"`
	Modes         *SessionModeState     `json:"modes,omitempty"`
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
	Meta          json.RawMessage       `json:"_meta,omitempty"`
}

type LoadSessionRequest struct {
	SessionID SessionID `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	// AdditionalDirectories are extra workspace roots to activate for this
	// session, on top of Cwd. Each path must be absolute. Omitted or empty
	// means none.
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	McpServers            []McpServer     `json:"mcpServers"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type LoadSessionResponse struct {
	Modes         *SessionModeState     `json:"modes,omitempty"`
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
	Meta          json.RawMessage       `json:"_meta,omitempty"`
}

// SessionModeState is the spec's wire shape for `modes` in session/new and
// session/load responses: an object carrying the current mode id plus the
// available modes, not a bare array.
type SessionModeState struct {
	CurrentModeID  string          `json:"currentModeId"`
	AvailableModes []SessionMode   `json:"availableModes"`
	Meta           json.RawMessage `json:"_meta,omitempty"`
}

type SessionMode struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

// SetSessionModeRequest is session/set_mode's params: switch a session to a
// different SessionMode.ID, one of the ids SessionModeState.AvailableModes
// advertised.
type SetSessionModeRequest struct {
	SessionID SessionID       `json:"sessionId"`
	ModeID    string          `json:"modeId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type SetSessionModeResponse struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

// Session configuration option type discriminators (SessionConfigOption.Type).
const (
	SessionConfigOptionTypeSelect  = "select"
	SessionConfigOptionTypeBoolean = "boolean"
)

type SessionConfigOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Type        string `json:"type"`
	// CurrentValue is always the Go-side string form: for
	// SessionConfigOptionTypeSelect it is the selected SessionConfigValue.Value
	// id; for SessionConfigOptionTypeBoolean it is "true"/"false" (mirroring
	// SessionConfigOptionValue.AsString). MarshalJSON renders it as a JSON
	// boolean on the wire for the boolean type, as SessionConfigBoolean
	// requires; UnmarshalJSON accepts either wire shape back into this string.
	CurrentValue string              `json:"currentValue"`
	Options      SessionConfigValues `json:"options"`
	Meta         json.RawMessage     `json:"_meta,omitempty"`
}

// sessionConfigOptionWire is SessionConfigOption's wire shape: CurrentValue is
// deferred as raw JSON (string for "select", boolean for "boolean"), and
// Options is a pointer so the "boolean" variant — which the spec's
// SessionConfigBoolean has no options field for — can omit it entirely.
type sessionConfigOptionWire struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Description  string               `json:"description,omitempty"`
	Category     string               `json:"category,omitempty"`
	Type         string               `json:"type"`
	CurrentValue json.RawMessage      `json:"currentValue"`
	Options      *SessionConfigValues `json:"options,omitempty"`
	Meta         json.RawMessage      `json:"_meta,omitempty"`
}

func (o SessionConfigOption) MarshalJSON() ([]byte, error) {
	w := sessionConfigOptionWire{
		ID:          o.ID,
		Name:        o.Name,
		Description: o.Description,
		Category:    o.Category,
		Type:        o.Type,
		Meta:        o.Meta,
	}
	if o.Type == SessionConfigOptionTypeBoolean {
		raw, err := json.Marshal(o.CurrentValue == "true")
		if err != nil {
			return nil, err
		}
		w.CurrentValue = raw
	} else {
		raw, err := json.Marshal(o.CurrentValue)
		if err != nil {
			return nil, err
		}
		w.CurrentValue = raw
		options := o.Options
		w.Options = &options
	}
	return json.Marshal(w)
}

func (o *SessionConfigOption) UnmarshalJSON(data []byte) error {
	var w sessionConfigOptionWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*o = SessionConfigOption{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
		Category:    w.Category,
		Type:        w.Type,
		Meta:        w.Meta,
	}
	if w.Options != nil {
		o.Options = *w.Options
	}
	if len(w.CurrentValue) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(w.CurrentValue, &s); err == nil {
		o.CurrentValue = s
		return nil
	}
	var b bool
	if err := json.Unmarshal(w.CurrentValue, &b); err == nil {
		o.CurrentValue = strconv.FormatBool(b)
		return nil
	}
	return fmt.Errorf("libacp: session config option %q currentValue must be a string or a boolean, got %s", w.ID, w.CurrentValue)
}

type SessionConfigValues struct {
	Values []SessionConfigValue
	Groups []SessionConfigGroup
}

type SessionConfigValue struct {
	Value       string          `json:"value"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

type SessionConfigGroup struct {
	Group   string               `json:"group"`
	Name    string               `json:"name"`
	Options []SessionConfigValue `json:"options"`
	Meta    json.RawMessage      `json:"_meta,omitempty"`
}

func NewSessionConfigValues(values []SessionConfigValue) SessionConfigValues {
	return SessionConfigValues{Values: values}
}

func NewGroupedSessionConfigValues(groups []SessionConfigGroup) SessionConfigValues {
	return SessionConfigValues{Groups: groups}
}

func (v SessionConfigValues) AllValues() []SessionConfigValue {
	if len(v.Groups) == 0 {
		return v.Values
	}
	var out []SessionConfigValue
	for _, group := range v.Groups {
		out = append(out, group.Options...)
	}
	return out
}

func (v SessionConfigValues) MarshalJSON() ([]byte, error) {
	if len(v.Groups) > 0 {
		return json.Marshal(v.Groups)
	}
	if v.Values == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(v.Values)
}

func (v *SessionConfigValues) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		v.Values = []SessionConfigValue{}
		v.Groups = nil
		return nil
	}
	var probe struct {
		Group   string          `json:"group"`
		Options json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(raw[0], &probe); err == nil && probe.Options != nil {
		var groups []SessionConfigGroup
		if err := json.Unmarshal(data, &groups); err != nil {
			return err
		}
		v.Values = nil
		v.Groups = groups
		return nil
	}
	var values []SessionConfigValue
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	v.Values = values
	v.Groups = nil
	return nil
}

type SetSessionConfigOptionRequest struct {
	SessionID SessionID `json:"sessionId"`
	ConfigID  string    `json:"configId"`
	// Type discriminates the value variant: absent/unknown means Value is a
	// string value id (the default), "boolean" means Value is a bool.
	Type  string                   `json:"type,omitempty"`
	Value SessionConfigOptionValue `json:"value"`
	Meta  json.RawMessage          `json:"_meta,omitempty"`
}

// SessionConfigOptionValue is the value union of session/set_config_option: a
// plain string value id (default) or a boolean (request Type "boolean").
type SessionConfigOptionValue struct {
	IsBool bool
	Str    string
	Bool   bool
}

func StringConfigValue(s string) SessionConfigOptionValue {
	return SessionConfigOptionValue{Str: s}
}

func BoolConfigValue(b bool) SessionConfigOptionValue {
	return SessionConfigOptionValue{IsBool: true, Bool: b}
}

// AsString renders the value for consumers that key handling off strings;
// booleans become "true"/"false".
func (v SessionConfigOptionValue) AsString() string {
	if v.IsBool {
		if v.Bool {
			return "true"
		}
		return "false"
	}
	return v.Str
}

func (v SessionConfigOptionValue) MarshalJSON() ([]byte, error) {
	if v.IsBool {
		return json.Marshal(v.Bool)
	}
	return json.Marshal(v.Str)
}

func (v *SessionConfigOptionValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*v = SessionConfigOptionValue{Str: s}
		return nil
	}
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*v = SessionConfigOptionValue{IsBool: true, Bool: b}
		return nil
	}
	return fmt.Errorf("libacp: config option value must be a string or a boolean, got %s", data)
}

type SetSessionConfigOptionResponse struct {
	ConfigOptions []SessionConfigOption `json:"configOptions"`
	Meta          json.RawMessage       `json:"_meta,omitempty"`
}

// ResumeSessionRequest reconnects to an existing session WITHOUT history
// replay (the client kept its transcript). McpServers is optional here,
// unlike session/new and session/load.
type ResumeSessionRequest struct {
	SessionID SessionID `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	// AdditionalDirectories are extra workspace roots to activate for this
	// session, on top of Cwd. Each path must be absolute. Omitted or empty
	// means none.
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	McpServers            []McpServer     `json:"mcpServers,omitempty"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type ResumeSessionResponse struct {
	Modes         *SessionModeState     `json:"modes,omitempty"`
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
	Meta          json.RawMessage       `json:"_meta,omitempty"`
}

type CloseSessionRequest struct {
	SessionID SessionID       `json:"sessionId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type CloseSessionResponse struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

type DeleteSessionRequest struct {
	SessionID SessionID       `json:"sessionId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type DeleteSessionResponse struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

type CancelNotification struct {
	SessionID SessionID       `json:"sessionId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// CancelRequestNotification is the payload of "$/cancel_request": the JSON-RPC
// id of the request whose response is no longer awaited.
type CancelRequestNotification struct {
	RequestID RequestID       `json:"requestId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type ListSessionsRequest struct {
	Cwd    string          `json:"cwd,omitempty"`
	Cursor string          `json:"cursor,omitempty"`
	Meta   json.RawMessage `json:"_meta,omitempty"`
}

type SessionInfo struct {
	SessionID SessionID `json:"sessionId"`
	Cwd       string    `json:"cwd,omitempty"`
	// AdditionalDirectories is the complete ordered additional-root list
	// associated with this session, when the agent tracks and reports it.
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	Title                 string          `json:"title,omitempty"`
	UpdatedAt             string          `json:"updatedAt,omitempty"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type ListSessionsResponse struct {
	Sessions   []SessionInfo   `json:"sessions"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}
