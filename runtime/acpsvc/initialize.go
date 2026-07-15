package acpsvc

import (
	"context"
	"encoding/json"
	"os"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/version"
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
			Type:        libacp.AuthMethodTypeTerminal,
			Args:        []string{"acp", "--setup"},
			Meta: mustJSON(map[string]any{
				"terminal-auth": map[string]any{
					"command": command,
					"args":    []string{"acp", "--setup"},
					"label":   "Contenox Setup",
				},
			}),
		})
		// The browser sibling: same terminal-auth launch mechanics, but the
		// command serves the Beam onboarding UI, opens the browser, and exits
		// once setup is complete.
		authMethods = append(authMethods, libacp.AuthMethod{
			ID:          browserAuthMethodID,
			Name:        "Setup Contenox in browser",
			Description: "Opens the Beam web UI to configure your LLM provider and model, then exits when setup is complete.",
			Type:        libacp.AuthMethodTypeTerminal,
			Args:        []string{"acp", "--setup-web"},
			Meta: mustJSON(map[string]any{
				"terminal-auth": map[string]any{
					"command": command,
					"args":    []string{"acp", "--setup-web"},
					"label":   "Contenox Setup (browser)",
				},
			}),
		})
	}
	// The env_var method is the non-interactive setup route: the client
	// collects the listed variables and relaunches the agent with them set (or
	// they are already set, and authenticate completes setup in place). Only
	// meaningful while unconfigured.
	if t.deps.Engine == nil && t.deps.EnvSetup != nil {
		authMethods = append(authMethods, libacp.AuthMethod{
			ID:          envAuthMethodID,
			Name:        "Configure from environment",
			Description: "Set the CONTENOX_DEFAULT_* variables (plus a provider API key for cloud providers); contenox completes setup non-interactively.",
			Type:        libacp.AuthMethodTypeEnvVar,
			Vars:        t.deps.EnvSetup.Vars,
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
				List:   &struct{}{},
				Resume: &struct{}{},
				Close:  &struct{}{},
				Delete: &struct{}{},
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
	// The spec's (unstable) capability field, and Zed's earlier _meta
	// convention — honor both.
	if caps.Auth.Terminal {
		return true
	}
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
