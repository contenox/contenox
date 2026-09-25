package acpsvc

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	libacp "github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

type extDuplex struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (p *extDuplex) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *extDuplex) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *extDuplex) Close() error {
	_ = p.r.Close()
	return p.w.Close()
}

type extNotification struct {
	Method string
	Params json.RawMessage
}

// startExtWire drives a real Transport through a real client connection over an
// in-memory duplex pipe, the production arrangement beam uses. The client
// registers no core Client behavior (UnimplementedClient) and captures every
// inbound extension notification in order.
func startExtWire(t *testing.T, deps Deps) (*Transport, *libacp.ClientSideConnection, chan extNotification) {
	t.Helper()
	agentR, clientW := io.Pipe()
	clientR, agentW := io.Pipe()
	agentSide := &extDuplex{r: agentR, w: agentW}
	clientSide := &extDuplex{r: clientR, w: clientW}

	ctx, cancel := context.WithCancel(context.Background())
	var clientConn *libacp.ClientSideConnection
	t.Cleanup(func() {
		// Cancel first, then wait for the client run loop to shut down so the
		// cleanup never blocks on a connection the test already abandoned.
		cancel()
		if clientConn != nil {
			select {
			case <-clientConn.Closed():
			case <-time.After(5 * time.Second):
			}
		}
	})

	var transport *Transport
	agentConn := libacp.NewAgentSideConnection(agentSide, func(c *libacp.AgentSideConnection) libacp.Agent {
		transport = New(deps)(c).(*Transport)
		return transport
	})
	go func() { _ = agentConn.Run(ctx) }()

	notifs := make(chan extNotification, 64)
	clientConn = libacp.NewClientSideConnection(clientSide, func(c *libacp.ClientSideConnection) libacp.Client {
		c.SetExtNotificationHandler(func(_ context.Context, method string, params json.RawMessage) {
			notifs <- extNotification{Method: method, Params: append(json.RawMessage(nil), params...)}
		})
		return &libacp.UnimplementedClient{}
	})
	go func() { _ = clientConn.Run(ctx) }()

	require.Eventually(t, func() bool { return transport != nil }, time.Second, time.Millisecond)
	return transport, clientConn, notifs
}

// extCall sends one extension request and returns the raw result or the wire
// error (a *libacp.Error when the agent answered an error object).
func extCall(t *testing.T, client *libacp.ClientSideConnection, method string, params any) (json.RawMessage, error) {
	t.Helper()
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		require.NoError(t, err)
		raw = b
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.CallExtMethod(ctx, method, raw)
}

// extWireError asserts err is a wire *libacp.Error with code and returns it.
func extWireError(t *testing.T, err error, code int) *libacp.Error {
	t.Helper()
	require.Error(t, err)
	var rpcErr *libacp.Error
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, code, rpcErr.Code)
	return rpcErr
}

func TestExtWire_UnknownExtensionMethodIsMethodNotFound(t *testing.T) {
	_, client, _ := startExtWire(t, Deps{})
	_, err := extCall(t, client, "_contenox/nope", nil)
	extWireError(t, err, libacp.ErrMethodNotFound)
}

func TestExtWire_ErrorCodesSurviveTheWire(t *testing.T) {
	svc := newLocalFileSvc(t)
	_, client, _ := startExtWire(t, Deps{Files: svc})

	// A missing path must arrive as a typed resource-not-found, so the client
	// side can classify with IsNotFound/AsNotExist.
	_, err := extCall(t, client, extMethodFSStat, map[string]any{"path": "absent.txt"})
	rpcErr := extWireError(t, err, libacp.ErrResourceNotFound)
	require.True(t, libacp.IsNotFound(rpcErr))

	// A malformed param stays InvalidParams.
	_, err = extCall(t, client, extMethodFSStat, map[string]any{"path": "../../etc/passwd"})
	extWireError(t, err, libacp.ErrInvalidParams)
}
