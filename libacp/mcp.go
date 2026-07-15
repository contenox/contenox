package libacp

import (
	"encoding/json"
	"fmt"
)

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HttpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type McpServerKind string

const (
	McpServerKindStdio McpServerKind = ""
	McpServerKindHTTP  McpServerKind = "http"
	McpServerKindSSE   McpServerKind = "sse"
)

type McpServer struct {
	Type    string          `json:"type,omitempty"`
	Name    string          `json:"name"`
	Command string          `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []EnvVariable   `json:"env,omitempty"`
	URL     string          `json:"url,omitempty"`
	Headers []HttpHeader    `json:"headers,omitempty"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

func (m McpServer) Kind() McpServerKind {
	switch m.Type {
	case "http":
		return McpServerKindHTTP
	case "sse":
		return McpServerKindSSE
	default:
		return McpServerKindStdio
	}
}

// mcpServerHttpWire and mcpServerStdioWire are McpServer's two wire shapes for
// MarshalJSON: unlike McpServer itself, args/env and headers have no
// omitempty, because the spec requires them always present (even as `[]`)
// for their respective transport.
type mcpServerHttpWire struct {
	Type    string          `json:"type,omitempty"`
	Name    string          `json:"name"`
	URL     string          `json:"url,omitempty"`
	Headers []HttpHeader    `json:"headers"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

type mcpServerStdioWire struct {
	Name    string          `json:"name"`
	Command string          `json:"command,omitempty"`
	Args    []string        `json:"args"`
	Env     []EnvVariable   `json:"env"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

// MarshalJSON forces args/env (McpServerStdio) and headers (McpServerHttp/
// McpServerSse) onto the wire as `[]` rather than omitting them when empty:
// the spec (and the reference Rust implementation) declares these fields as
// plain, always-serialized arrays with no default, so a strict receiver
// rejects a payload missing them. omitempty alone cannot express this on the
// flattened McpServer struct — it treats a zero-length slice as absent
// regardless of nil-ness — hence the two per-transport wire shapes below.
func (m McpServer) MarshalJSON() ([]byte, error) {
	switch m.Kind() {
	case McpServerKindHTTP, McpServerKindSSE:
		headers := m.Headers
		if headers == nil {
			headers = []HttpHeader{}
		}
		return json.Marshal(mcpServerHttpWire{
			Type:    m.Type,
			Name:    m.Name,
			URL:     m.URL,
			Headers: headers,
			Meta:    m.Meta,
		})
	default:
		args, env := m.Args, m.Env
		if args == nil {
			args = []string{}
		}
		if env == nil {
			env = []EnvVariable{}
		}
		return json.Marshal(mcpServerStdioWire{
			Name:    m.Name,
			Command: m.Command,
			Args:    args,
			Env:     env,
			Meta:    m.Meta,
		})
	}
}

func (m McpServer) Validate() error {
	switch m.Kind() {
	case McpServerKindStdio:
		if m.Command == "" {
			return fmt.Errorf("libacp: stdio mcp server %q missing command", m.Name)
		}
	case McpServerKindHTTP, McpServerKindSSE:
		if m.URL == "" {
			return fmt.Errorf("libacp: %s mcp server %q missing url", m.Type, m.Name)
		}
	}
	return nil
}
