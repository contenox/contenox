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
