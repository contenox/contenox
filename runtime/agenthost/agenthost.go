// Package agenthost is the runtime's client/host-role primitive for driving
// another ACP agent, the way an editor like Zed spawns an agent binary over
// stdio and drives it as a client. It is the counterpart to this repo's
// existing ACP *agent* role (`contenox acp`, runtime/acpsvc): where acpsvc
// makes contenox speak Agent, agenthost makes the runtime speak Client
// against something else.
//
// The harness — the libacp.Client callback surface an agent calls back into
// (session/request_permission, fs/*, terminal/*, session/update) — is always
// a parameter supplied by the caller, never assembled inside this package.
// That seam is deliberate: this package builds the low-level plumbing to
// spawn/connect an external ACP agent kind (runtimetypes.AgentKindExternalACP)
// plus minimal single-turn session driving on top of it (DriveTurn, drive.go);
// a real harness registry/service and a "chain" agent kind
// (runtimetypes.AgentKindChain, reserved in the schema) are later work.
package agenthost

import (
	"context"
	"sync"

	"github.com/contenox/runtime/libacp"
)

// Agent is the runtime-side primitive for driving another ACP agent: given a
// harness, Connect establishes a live connection to that agent and returns a
// Handle wired to it. ExternalACPAgent is the only implementation in this
// slice (a spawned/attached external ACP peer); the interface is named
// generically, not "ExternalACPAgent-specific", so a future "chain" agent
// kind — an in-runtime task chain addressed the same way — is a drop-in
// second implementation without changing this seam.
type Agent interface {
	// Connect spawns or attaches to the agent and returns a live Handle
	// wired to harness. harness is supplied by the caller: a production
	// harness assembling real permission/fs/terminal handling, a minimal
	// test harness, or libacp.UnimplementedClient{} as a no-op, can all be
	// passed here unchanged — Connect never builds one itself.
	Connect(ctx context.Context, harness libacp.Client) (*Handle, error)
}

// Handle is a live connection to an agent, returned by Agent.Connect. It
// owns that connection's lifecycle: Close tears down the underlying
// transport (e.g. the spawned subprocess) and waits for the connection's
// read loop to exit before returning, so a caller that has called Close
// knows the agent is fully torn down, not just "asked to stop".
type Handle struct {
	// Conn is the live ACP client-side connection to the agent. Callers
	// issue ACP calls against it directly: Initialize, NewSession, Prompt,
	// and so on (see libacp/clientconn.go for the full outbound surface).
	Conn *libacp.ClientSideConnection

	closeFn   func() error
	closeOnce sync.Once
	closeErr  error
}

// Close tears down the Handle's transport and waits for Conn's read loop
// (Conn.Run) to return. It is idempotent: every call, including ones after
// the first, returns the same result.
func (h *Handle) Close() error {
	h.closeOnce.Do(func() {
		h.closeErr = h.closeFn()
	})
	return h.closeErr
}
