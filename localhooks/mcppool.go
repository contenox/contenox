package localhooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/contenox/contenox/libtracker"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPTransport identifies how to connect to an MCP server.
type MCPTransport string

const (
	MCPTransportStdio MCPTransport = "stdio"
	MCPTransportSSE   MCPTransport = "sse"
	MCPTransportHTTP  MCPTransport = "http"
)

// MCPAuthType identifies the auth mechanism for remote MCP servers.
type MCPAuthType string

const (
	MCPAuthNone   MCPAuthType = ""
	MCPAuthBearer MCPAuthType = "bearer"
)

// MCPAuthConfig holds auth parameters for connecting to an MCP server.
type MCPAuthConfig struct {
	// Type is "bearer" or "" (none).
	Type MCPAuthType

	// Token is a literal bearer token. Prefer APIKeyFromEnv for security.
	Token string

	// APIKeyFromEnv is the name of an environment variable holding the bearer token.
	APIKeyFromEnv string
}

// ResolveToken returns the bearer token from literal value or env var.
func (a *MCPAuthConfig) ResolveToken() string {
	if a == nil {
		return ""
	}
	if a.Token != "" {
		return a.Token
	}
	if a.APIKeyFromEnv != "" {
		return os.Getenv(a.APIKeyFromEnv)
	}
	return ""
}

// MCPServerConfig describes a single MCP server connection.
type MCPServerConfig struct {
	// Name is the hook name used in chain JSON, e.g. "filesystem".
	Name string

	// Transport: "stdio" (default), "sse", or "http".
	Transport MCPTransport

	// Stdio transport: Command + Args to spawn.
	Command string
	Args    []string

	// Remote transport: URL of the SSE MCP endpoint.
	URL string

	// Auth for remote transports (optional).
	Auth *MCPAuthConfig

	// ConnectTimeout for the initial handshake (default 30s).
	ConnectTimeout time.Duration

	// MCPSessionID is the persisted session ID to resume (for HTTP/SSE transports).
	MCPSessionID string

	// OnSessionID is a callback fired when the server issues a new session ID.
	OnSessionID func(string)

	// Tracker is the activity tracker for observing MCP pool operations.
	Tracker libtracker.ActivityTracker
}

// MCPSessionPool manages a single MCP client session with reconnect support.
// Mirrors the SSHClientCache pattern: mutex-protected, reconnects on failure.
type MCPSessionPool struct {
	mu      sync.RWMutex
	session *mcp.ClientSession
	cfg     MCPServerConfig
	tracker libtracker.ActivityTracker

	// sidMu guards mcpSessionID independently of mu so the sessionRoundTripper
	// can update the live session ID (e.g. on 404 auto-heal) without deadlocking
	// against the pool-level read/write lock.
	sidMu        sync.RWMutex
	mcpSessionID string
}

// NewMCPSessionPool creates (but does not connect) a session pool for the given config.
func NewMCPSessionPool(cfg MCPServerConfig) *MCPSessionPool {
	t := cfg.Tracker
	if t == nil {
		t = libtracker.NoopTracker{}
	}
	return &MCPSessionPool{
		cfg:          cfg,
		tracker:      t,
		mcpSessionID: cfg.MCPSessionID, // seed from persisted KV value
	}
}

// Connect establishes the MCP session. Safe to call multiple times (idempotent).
func (p *MCPSessionPool) Connect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		return nil // already connected
	}
	return p.connectLocked(ctx)
}

func (p *MCPSessionPool) connectLocked(ctx context.Context) error {
	reportErr, reportChange, end := p.tracker.Start(ctx, "connect", "mcp_server", "name", p.cfg.Name, "transport", string(p.cfg.Transport))
	defer end()

	timeout := p.cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	transport, err := p.buildTransport()
	if err != nil {
		reportErr(err)
		return fmt.Errorf("mcp: build transport for %q: %w", p.cfg.Name, err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "contenox",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		reportErr(err)
		return fmt.Errorf("mcp: connect to %q: %w", p.cfg.Name, err)
	}

	p.session = session
	reportChange(p.cfg.Name, map[string]any{"transport": string(p.cfg.Transport), "url": p.cfg.URL})
	return nil
}

func (p *MCPSessionPool) buildTransport() (mcp.Transport, error) {
	// Closures let sessionRoundTripper safely read/write the live session ID
	// without holding the pool-level lock (which would deadlock).
	getSessionID := func() string {
		p.sidMu.RLock()
		defer p.sidMu.RUnlock()
		return p.mcpSessionID
	}
	setSessionID := func(id string) {
		p.sidMu.Lock()
		changed := p.mcpSessionID != id
		if changed {
			p.mcpSessionID = id
		}
		p.sidMu.Unlock()
		if changed && p.cfg.OnSessionID != nil {
			p.cfg.OnSessionID(id)
		}
	}

	switch p.cfg.Transport {
	case MCPTransportStdio, "":
		if p.cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires a command")
		}
		cmd := exec.Command(p.cfg.Command, p.cfg.Args...)
		return &mcp.CommandTransport{Command: cmd}, nil

	case MCPTransportSSE:
		if p.cfg.URL == "" {
			return nil, fmt.Errorf("sse transport requires a url")
		}
		var rt http.RoundTripper = http.DefaultTransport
		if token := p.cfg.Auth.ResolveToken(); token != "" {
			rt = &bearerRoundTripper{base: rt, token: token}
		}
		rt = &sessionRoundTripper{base: rt, getSessionID: getSessionID, setSessionID: setSessionID}
		t := &mcp.SSEClientTransport{Endpoint: p.cfg.URL}
		t.HTTPClient = &http.Client{Transport: rt}
		return t, nil

	case MCPTransportHTTP:
		if p.cfg.URL == "" {
			return nil, fmt.Errorf("http transport requires a url")
		}
		var rt http.RoundTripper = http.DefaultTransport
		if token := p.cfg.Auth.ResolveToken(); token != "" {
			rt = &bearerRoundTripper{base: rt, token: token}
		}
		rt = &sessionRoundTripper{base: rt, getSessionID: getSessionID, setSessionID: setSessionID}
		t := &mcp.StreamableClientTransport{Endpoint: p.cfg.URL}
		t.HTTPClient = &http.Client{Transport: rt}
		return t, nil

	default:
		return nil, fmt.Errorf("unknown MCP transport: %q", p.cfg.Transport)
	}
}

// Session returns the active session, connecting lazily if not yet established.
func (p *MCPSessionPool) Session(ctx context.Context) (*mcp.ClientSession, error) {
	p.mu.RLock()
	s := p.session
	p.mu.RUnlock()
	if s != nil {
		return s, nil
	}
	// Not yet connected; connect now.
	if err := p.Connect(ctx); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.session, nil
}

// Reconnect tears down the current session and re-connects.
// Called automatically by MCPHookRepo.Exec on transport errors.
func (p *MCPSessionPool) Reconnect(ctx context.Context) error {
	reportErr, reportChange, end := p.tracker.Start(ctx, "reconnect", "mcp_server", "name", p.cfg.Name)
	defer end()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		_ = p.session.Close()
		p.session = nil
	}
	reportChange(p.cfg.Name, "reconnecting")
	if err := p.connectLocked(ctx); err != nil {
		reportErr(err)
		return err
	}
	return nil
}

// Close terminates the session cleanly.
func (p *MCPSessionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		err := p.session.Close()
		p.session = nil
		return err
	}
	return nil
}

// bearerRoundTripper injects Authorization: Bearer <token> on every request.
type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (b *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(req2)
}

// sessionRoundTripper intercepts and injects the Mcp-Session-Id header.
// It auto-heals stale sessions: if we injected a token and the server responds
// with HTTP 404, the dead token is wiped and the request is replayed fresh.
type sessionRoundTripper struct {
	base         http.RoundTripper
	getSessionID func() string
	setSessionID func(string)
}

func (srt *sessionRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	sid := srt.getSessionID()

	// Only inject if the go-sdk didn't already supply it.
	// Track whether we added it so we only auto-heal errors we caused.
	injected := false
	if sid != "" && req2.Header.Get("Mcp-Session-Id") == "" {
		req2.Header.Set("Mcp-Session-Id", sid)
		injected = true
	}

	resp, err := srt.base.RoundTrip(req2)

	// AUTO-HEAL: server returned 404 because it no longer knows this session
	// (e.g. it was restarted). Wipe the token, drain the response body to free
	// the TCP connection, and replay the original request without the header.
	if err == nil && resp != nil && resp.StatusCode == http.StatusNotFound && injected {
		// Only replay if the body is replayable (nil body or GetBody available).
		if req.Body == nil || req.GetBody != nil {
			srt.setSessionID("") // wipe in-memory + notify KV callback

			// Drain and close the 404 body to reuse the connection.
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}

			// Retry without the stale header.
			req3 := req.Clone(req.Context())
			req3.Header.Del("Mcp-Session-Id")
			if req.Body != nil && req.GetBody != nil {
				if body, bodyErr := req.GetBody(); bodyErr == nil {
					req3.Body = body
				}
			}
			resp, err = srt.base.RoundTrip(req3)
		}
	}

	// Capture server-issued session ID from any successful response.
	if err == nil && resp != nil {
		if newID := resp.Header.Get("Mcp-Session-Id"); newID != "" {
			srt.setSessionID(newID)
		}
	}
	return resp, err
}

// CallTool calls a tool on the persistent session, reconnecting once if the
// session appears to have been lost. The pool must be connected first.
func (p *MCPSessionPool) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	reportErr, reportChange, end := p.tracker.Start(ctx, "call_tool", "mcp_server", "name", p.cfg.Name, "tool", toolName)
	defer end()
	if args == nil {
		args = map[string]any{}
	}
	result, err := p.callTool(ctx, toolName, args)
	if err != nil {
		// App-level errors (bad LLM arguments, method not found, context cancel)
		// must NOT trigger reconnect — the underlying session is still healthy.
		if isAppError(err) {
			reportErr(err)
			return nil, err
		}
		// Transport error: attempt one reconnect.
		if reconnectErr := p.Reconnect(ctx); reconnectErr != nil {
			mergedErr := fmt.Errorf("mcp %q.%q: call failed and reconnect failed: %w (original: %v)", p.cfg.Name, toolName, reconnectErr, err)
			reportErr(mergedErr)
			return nil, mergedErr
		}
		result, err = p.callTool(ctx, toolName, args)
	}
	if err != nil {
		reportErr(err)
		return nil, err
	}
	reportChange(p.cfg.Name, toolName)
	return result, nil
}

func (p *MCPSessionPool) callTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	session, err := p.Session(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcp %q.%q: session: %w", p.cfg.Name, toolName, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp %q.%q: call: %w", p.cfg.Name, toolName, err)
	}
	if result.IsError {
		return nil, fmt.Errorf("mcp %q.%q: tool error: %s", p.cfg.Name, toolName, mcpCollectText(result.Content))
	}
	if result.StructuredContent != nil {
		return result.StructuredContent, nil
	}
	return mcpCollectText(result.Content), nil
}

// ListTools returns all tools advertised by the MCP server, reconnecting once
// if the session has been lost.
func (p *MCPSessionPool) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	reportErr, reportChange, end := p.tracker.Start(ctx, "list_tools", "mcp_server", "name", p.cfg.Name)
	defer end()
	tools, err := p.listTools(ctx)
	if err != nil {
		if isAppError(err) {
			reportErr(err)
			return nil, err
		}
		if reconnectErr := p.Reconnect(ctx); reconnectErr != nil {
			mergedErr := fmt.Errorf("mcp %q: list-tools failed and reconnect failed: %w (original: %v)", p.cfg.Name, reconnectErr, err)
			reportErr(mergedErr)
			return nil, mergedErr
		}
		tools, err = p.listTools(ctx)
	}
	if err != nil {
		reportErr(err)
		return nil, err
	}
	reportChange(p.cfg.Name, len(tools))
	return tools, nil
}

func (p *MCPSessionPool) listTools(ctx context.Context) ([]*mcp.Tool, error) {
	session, err := p.Session(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcp %q: list-tools session: %w", p.cfg.Name, err)
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp %q: list-tools: %w", p.cfg.Name, err)
	}
	return result.Tools, nil
}

// mcpCollectText concatenates all TextContent entries from an MCP content slice.
func mcpCollectText(contents []mcp.Content) string {
	var sb []byte
	for _, c := range contents {
		if tc, ok := c.(*mcp.TextContent); ok {
			if len(sb) > 0 {
				sb = append(sb, '\n')
			}
			sb = append(sb, tc.Text...)
		}
	}
	return string(sb)
}

// isAppError determines if the error returned by the MCP SDK is an application-level
// JSON-RPC rejection (like invalid schema) or context cancellation, rather than a network drop.
func isAppError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid params") ||
		strings.Contains(msg, "tool error") ||
		strings.Contains(msg, "unexpected additional properties") ||
		strings.Contains(msg, "missing properties") ||
		strings.Contains(msg, "method not found") ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}
