package acpsvc

import (
	"context"
	"encoding/json"
	"os"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/version"
)

func (t *Transport) Initialize(_ context.Context, req libacp.InitializeRequest) (libacp.InitializeResponse, error) {
	t.initMu.Lock()
	t.clientInfo = req.ClientInfo
	t.clientCaps = req.ClientCapabilities
	t.initMu.Unlock()

	var authMethods []libacp.AuthMethod
	if clientSupportsTerminalAuth(req.ClientCapabilities) {
		command := os.Args[0]
		authMethods = append(authMethods, libacp.AuthMethod{
			ID:          terminalAuthMethodID,
			Name:        "Setup Contenox",
			Description: "Opens an interactive terminal to configure your LLM provider and model.",
			Type:        "terminal",
			Meta: mustJSON(map[string]any{
				"terminal-auth": map[string]any{
					"command": command,
					"args":    []string{"acp", "--setup"},
					"label":   "Contenox Setup",
				},
			}),
		})
	}

	return libacp.InitializeResponse{
		ProtocolVersion: negotiateProtocolVersion(req.ProtocolVersion),
		AgentInfo: &libacp.Implementation{
			Name:    "contenox",
			Title:   "Contenox ACP Agent",
			Version: version.Get(),
		},
		AgentCapabilities: libacp.AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: libacp.PromptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: true,
			},
			McpCapabilities: libacp.McpCapabilities{
				HTTP: true,
				SSE:  false,
			},
			SessionCapabilities: libacp.SessionCapabilities{
				List: &struct{}{},
			},
		},
		AuthMethods: authMethods,
	}, nil
}

func negotiateProtocolVersion(client int) int {
	if client >= 1 && client <= libacp.ProtocolVersion {
		return client
	}
	return libacp.ProtocolVersion
}

func clientSupportsTerminalAuth(caps libacp.ClientCapabilities) bool {
	if caps.Meta == nil {
		return false
	}
	var meta map[string]any
	if err := json.Unmarshal(caps.Meta, &meta); err != nil {
		return false
	}
	v, ok := meta["terminal-auth"]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
