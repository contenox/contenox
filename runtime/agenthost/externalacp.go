package agenthost

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libacp/acpexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

// closeTimeout bounds how long Handle.Close waits for the underlying
// ClientSideConnection's read loop (Run) to observe the transport closing
// and return, after the transport has been asked to close. It mirrors, one
// layer up, the grace-period pattern acpexec.Process.Close already applies
// to the subprocess itself.
const closeTimeout = 5 * time.Second

// ExternalACPAgent is the runtimetypes.AgentKindExternalACP implementation
// of Agent: it connects to an external ACP agent described by an
// ExternalACPConfig, either by spawning it as a subprocess over stdio (the
// v1, implemented path — it wraps libacp/acpexec, the same subprocess
// plumbing the client-e2e tests use to drive testy) or, in the future, by
// dialing it as a network endpoint (not implemented yet; Connect returns a
// clear error for that transport instead of silently doing nothing).
type ExternalACPAgent struct {
	Config runtimetypes.ExternalACPConfig

	// Stderr, if set, receives the spawned subprocess's stderr as it is
	// written (see acpexec.WithStderr). Defaults to io.Discard.
	Stderr io.Writer

	// KillGrace, if positive, overrides how long teardown waits for the
	// spawned agent to exit after its stdin is closed before killing it
	// (see acpexec.WithKillGrace; default 5s). Persistent agents — testy,
	// most editor adapters — never exit on stdin-close, so a short grace
	// here is what keeps their teardown from stalling for the full default.
	KillGrace time.Duration
}

// NewExternalACPAgent returns an ExternalACPAgent for cfg.
func NewExternalACPAgent(cfg runtimetypes.ExternalACPConfig) *ExternalACPAgent {
	return &ExternalACPAgent{Config: cfg}
}

var _ Agent = (*ExternalACPAgent)(nil)

// Connect validates a.Config and dispatches to the transport-specific
// connect path. harness is passed straight through to the
// libacp.ClientSideConnection this establishes — see Agent.Connect's doc
// comment for why that seam matters.
func (a *ExternalACPAgent) Connect(ctx context.Context, harness libacp.Client) (*Handle, error) {
	if harness == nil {
		return nil, fmt.Errorf("agenthost: harness is required")
	}
	if err := a.Config.Validate(); err != nil {
		return nil, fmt.Errorf("agenthost: invalid external_acp config: %w", err)
	}

	switch a.Config.Transport {
	case runtimetypes.ExternalACPTransportStdio:
		return a.connectStdio(ctx, harness)
	case runtimetypes.ExternalACPTransportEndpoint:
		return nil, fmt.Errorf("agenthost: endpoint transport is not implemented yet (agent %q)", a.Config.URL)
	default:
		// a.Config.Validate() above already rejects any transport other
		// than stdio/endpoint, so this is unreachable in practice — kept as
		// defense in depth against that invariant changing underneath us.
		return nil, fmt.Errorf("agenthost: unknown transport %q", a.Config.Transport)
	}
}

// connectStdio spawns a.Config.Command as a subprocess (acpexec.Spawn) and
// wires a libacp.ClientSideConnection to it over its stdin/stdout, exactly
// as libacp/acpexec's own client-e2e tests wire one to testy.
//
// ctx governs the spawned subprocess's entire lifetime, not just the connect
// step: acpexec.Spawn closes the process down (the same way Handle.Close
// would) the moment ctx is cancelled. A caller that wants a long-lived agent
// independent of whatever short-lived ctx it happened to call Connect with
// should pass one it controls directly (e.g. context.Background()) and rely
// on Handle.Close, not ctx cancellation, for teardown.
func (a *ExternalACPAgent) connectStdio(ctx context.Context, harness libacp.Client) (*Handle, error) {
	cmd := exec.Command(a.Config.Command, a.Config.Args...)
	if a.Config.Cwd != "" {
		cmd.Dir = a.Config.Cwd
	}
	if len(a.Config.Env) > 0 {
		cmd.Env = append(os.Environ(), envPairs(a.Config.Env)...)
	}

	var opts []acpexec.Option
	if a.Stderr != nil {
		opts = append(opts, acpexec.WithStderr(a.Stderr))
	}
	if a.KillGrace > 0 {
		opts = append(opts, acpexec.WithKillGrace(a.KillGrace))
	}

	proc, err := acpexec.Spawn(ctx, cmd, opts...)
	if err != nil {
		return nil, fmt.Errorf("agenthost: spawn external ACP agent %q: %w", a.Config.Command, err)
	}

	conn := libacp.NewClientSideConnection(proc, func(*libacp.ClientSideConnection) libacp.Client {
		return harness
	})

	runDone := make(chan error, 1)
	go func() { runDone <- conn.Run(ctx) }()

	closeFn := func() error {
		procErr := proc.Close()
		select {
		case runErr := <-runDone:
			if procErr != nil {
				return procErr
			}
			return runErr
		case <-time.After(closeTimeout):
			if procErr != nil {
				return procErr
			}
			return fmt.Errorf("agenthost: ClientSideConnection.Run did not exit within %s of Close", closeTimeout)
		}
	}

	return &Handle{Conn: conn, closeFn: closeFn}, nil
}

// envPairs renders env as "KEY=VALUE" pairs suitable for appending to
// exec.Cmd.Env.
func envPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for k, v := range env {
		pairs = append(pairs, k+"="+v)
	}
	return pairs
}
