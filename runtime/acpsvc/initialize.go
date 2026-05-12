package acpsvc

import (
	"context"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/version"
)

func (t *Transport) Initialize(_ context.Context, req libacp.InitializeRequest) (libacp.InitializeResponse, error) {
	t.initMu.Lock()
	t.clientInfo = req.ClientInfo
	t.clientCaps = req.ClientCapabilities
	t.initMu.Unlock()

	return libacp.InitializeResponse{
		ProtocolVersion: libacp.ProtocolVersion,
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
				EmbeddedContext: req.ClientCapabilities.FS.ReadTextFile,
			},
			McpCapabilities: libacp.McpCapabilities{
				HTTP: true,
				SSE:  false,
			},
			SessionCapabilities: libacp.SessionCapabilities{
				List: &struct{}{},
			},
		},
	}, nil
}
