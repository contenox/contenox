//go:build !windows

package acpsvc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/services/shellsession"
	"github.com/contenox/contenox/internal/services/vfs"
	libacp "github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

// terminalUpdate captures one session notification as delivered to the client
// side of the wire, so a test can assert the _contenox.terminalOutput stream
// arrives in the shape clients consume.
type terminalUpdate struct {
	Notif libacp.SessionNotification
}

type terminalCaptureClient struct {
	libacp.UnimplementedClient
	updates chan terminalUpdate
}

func (c *terminalCaptureClient) SessionUpdate(_ context.Context, n libacp.SessionNotification) error {
	c.updates <- terminalUpdate{Notif: n}
	return nil
}

// startTerminalWire mirrors startExtWire with a client that captures session
// updates (the channel startExtWire exposes only carries extension
// notifications, and terminal output travels as a session update).
func startTerminalWire(t *testing.T, deps Deps) (*Transport, *libacp.ClientSideConnection, chan terminalUpdate) {
	t.Helper()
	agentR, clientW := io.Pipe()
	clientR, agentW := io.Pipe()
	agentSide := &extDuplex{r: agentR, w: agentW}
	clientSide := &extDuplex{r: clientR, w: clientW}

	ctx, cancel := context.WithCancel(context.Background())
	var transport *Transport
	agentConn := libacp.NewAgentSideConnection(agentSide, func(c *libacp.AgentSideConnection) libacp.Agent {
		transport = New(deps)(c).(*Transport)
		return transport
	})
	go func() { _ = agentConn.Run(ctx) }()

	updates := make(chan terminalUpdate, 64)
	clientConn := libacp.NewClientSideConnection(clientSide, func(c *libacp.ClientSideConnection) libacp.Client {
		return &terminalCaptureClient{updates: updates}
	})
	go func() { _ = clientConn.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
	})

	require.Eventually(t, func() bool { return transport != nil }, time.Second, time.Millisecond)
	return transport, clientConn, updates
}

func newTerminalShellManager(t *testing.T) shellsession.Manager {
	t.Helper()
	root := t.TempDir()
	roots, err := vfs.NewFactory(root)
	require.NoError(t, err)
	m := shellsession.NewManager(shellsession.Config{
		CwdResolver: func(context.Context) string { return root },
		Workspace:   roots,
		Shell:       "/bin/sh",
	})
	t.Cleanup(m.Shutdown)
	return m
}

func registerTerminalSession(t *testing.T, transport *Transport, sid libacp.SessionID) {
	t.Helper()
	transport.sessionMu.Lock()
	transport.sessions[sid] = &sessionEntry{InternalSessionID: string(sid) + "-internal", Cwd: "/"}
	transport.sessionMu.Unlock()
}

func TestExtTerminal_RunStreamsOutputAsTerminalOutputUpdates(t *testing.T) {
	transport, client, updates := startTerminalWire(t, Deps{ShellSessions: newTerminalShellManager(t)})
	sid := libacp.SessionID("acp-term-1")
	registerTerminalSession(t, transport, sid)

	// A client consumes the stream continuously, never only between polls, so
	// the test drains into a slice from its own goroutine the way real clients
	// do.
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	go func() {
		for {
			select {
			case u := <-updates:
				mu.Lock()
				got = append(got, string(u.Notif.Update.Meta))
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(done) })

	raw, err := extCall(t, client, extMethodTerminalRun, map[string]any{
		"sessionId": string(sid),
		"command":   "echo hallo-welt",
	})
	require.NoError(t, err)
	var res struct {
		Offset  int64  `json:"offset"`
		Started bool   `json:"started"`
		Output  string `json:"output"`
	}
	require.NoError(t, json.Unmarshal(raw, &res))
	require.Greater(t, res.Offset, int64(0))

	// The streamed terminalOutput updates must carry the echo in the
	// client-consumed shape: kind _contenox.terminalOutput, payload under the
	// contenox.terminalOutput _meta key, Reset first then chunks.
	var buf []string
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for len(got) > 0 {
			meta := got[0]
			got = got[1:]
			var envelope struct {
				Payload struct {
					SessionID string `json:"sessionId"`
					Chunk     string `json:"chunk"`
					Reset     bool   `json:"reset"`
				} `json:"contenox.terminalOutput"`
			}
			if err := json.Unmarshal([]byte(meta), &envelope); err != nil {
				continue
			}
			p := envelope.Payload
			if p.SessionID != string(sid) {
				continue
			}
			if p.Reset {
				buf = buf[:0]
				continue
			}
			buf = append(buf, p.Chunk)
			if strings.Contains(strings.Join(buf, ""), "hallo-welt") {
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
}

func TestExtTerminal_ShellSessionsNilIsMethodNotFound(t *testing.T) {
	_, client, _ := startTerminalWire(t, Deps{})
	_, err := extCall(t, client, extMethodTerminalRun, map[string]any{
		"sessionId": "acp-x",
		"command":   "echo hi",
	})
	extWireError(t, err, libacp.ErrMethodNotFound)
}

func TestExtTerminal_UnknownSessionIsInvalidParams(t *testing.T) {
	_, client, _ := startTerminalWire(t, Deps{ShellSessions: newTerminalShellManager(t)})
	_, err := extCall(t, client, extMethodTerminalRun, map[string]any{
		"sessionId": "acp-never",
		"command":   "echo hi",
	})
	extWireError(t, err, libacp.ErrInvalidParams)
}

func TestExtTerminal_MissingCommandIsInvalidParams(t *testing.T) {
	transport, client, _ := startTerminalWire(t, Deps{ShellSessions: newTerminalShellManager(t)})
	sid := libacp.SessionID("acp-empty")
	registerTerminalSession(t, transport, sid)
	_, err := extCall(t, client, extMethodTerminalRun, map[string]any{"sessionId": string(sid)})
	extWireError(t, err, libacp.ErrInvalidParams)
}
