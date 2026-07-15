package contenoxcli

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

// acpWSDial opens a WebSocket connection to srv's root path, optionally with
// a bearer token passed as the "token" query parameter (the same fallback
// terminalapi's extractTerminalToken and extractACPToken here both support,
// since a browser WebSocket client cannot always set custom headers).
func acpWSDial(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	if token != "" {
		wsURL += "?token=" + token
	}
	ws, err := websocket.Dial(wsURL, "", srv.URL+"/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

// acpWSInitializeFrame builds a wire-ready "initialize" JSON-RPC request
// frame using libacp's own request/type helpers, matching what a conformant
// ACP client sends.
func acpWSInitializeFrame(t *testing.T, id int64) string {
	t.Helper()
	params, err := json.Marshal(libacp.InitializeRequest{
		ProtocolVersion:    libacp.ProtocolVersion,
		ClientCapabilities: libacp.ClientCapabilities{},
		ClientInfo:         &libacp.Implementation{Name: "ws-test", Version: "1"},
	})
	require.NoError(t, err)
	req := libacp.NewRequest(libacp.NewRequestIDNumber(id), libacp.MethodInitialize, params)
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	return string(raw)
}

// TestUnit_ACPWebSocketHandler_InitializeRoundTrip proves the frame<->line
// adapter and the WebSocket handler correctly carry one ACP JSON-RPC message
// per WebSocket TEXT frame end to end, through the real libacp connection
// (NewAgentSideConnection + Run), without needing a built engine/model:
// acpsvc.Transport.Initialize works against a nil Engine by design (see
// runtime/acpsvc/setuponly_test.go).
func TestUnit_ACPWebSocketHandler_InitializeRoundTrip(t *testing.T) {
	factory := acpsvc.New(acpsvc.Deps{})
	srv := httptest.NewServer(acpWebSocketHandler(factory, ""))
	t.Cleanup(srv.Close)

	ws := acpWSDial(t, srv, "")
	require.NoError(t, websocket.Message.Send(ws, acpWSInitializeFrame(t, 1)))

	require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
	var frame string
	require.NoError(t, websocket.Message.Receive(ws, &frame))

	in, err := libacp.ParseIncoming([]byte(frame))
	require.NoError(t, err)
	require.Equal(t, libacp.IncomingKindResponse, in.Kind, "wire: %s", frame)
	require.Nil(t, in.Response.Error, "wire: %s", frame)

	var result map[string]any
	require.NoError(t, json.Unmarshal(in.Response.Result, &result))
	require.Equal(t, float64(1), result["protocolVersion"], "wire: %s", frame)

	agentCaps, ok := result["agentCapabilities"].(map[string]any)
	require.True(t, ok, "agentCapabilities must be present, wire: %s", frame)
	_, hasSessionCaps := agentCaps["sessionCapabilities"]
	require.True(t, hasSessionCaps, "agentCapabilities.sessionCapabilities must be present, wire: %s", frame)
}

// TestUnit_ACPWebSocketHandler_TokenAuth proves /acp inherits serve's bearer
// token: a connection without the configured token never gets a valid
// initialize response (the handler checks and returns before wiring up
// libacp), while the same request with the token succeeds.
func TestUnit_ACPWebSocketHandler_TokenAuth(t *testing.T) {
	const token = "s3cr3t-token"
	factory := acpsvc.New(acpsvc.Deps{})
	srv := httptest.NewServer(acpWebSocketHandler(factory, token))
	t.Cleanup(srv.Close)

	t.Run("without token gets no valid response", func(t *testing.T) {
		ws := acpWSDial(t, srv, "")
		// The server-side handler returns immediately on a token mismatch, so
		// the write may itself fail once the peer starts closing; either way no
		// valid initialize response can arrive.
		_ = websocket.Message.Send(ws, acpWSInitializeFrame(t, 1))

		require.NoError(t, ws.SetReadDeadline(time.Now().Add(2*time.Second)))
		var frame string
		err := websocket.Message.Receive(ws, &frame)
		require.Error(t, err, "unauthenticated connection must not receive a valid initialize response")
	})

	t.Run("with token succeeds", func(t *testing.T) {
		ws := acpWSDial(t, srv, token)
		require.NoError(t, websocket.Message.Send(ws, acpWSInitializeFrame(t, 1)))

		require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
		var frame string
		require.NoError(t, websocket.Message.Receive(ws, &frame))

		in, err := libacp.ParseIncoming([]byte(frame))
		require.NoError(t, err)
		require.Equal(t, libacp.IncomingKindResponse, in.Kind, "wire: %s", frame)
		require.Nil(t, in.Response.Error, "wire: %s", frame)

		var result map[string]any
		require.NoError(t, json.Unmarshal(in.Response.Result, &result))
		require.Equal(t, float64(1), result["protocolVersion"], "wire: %s", frame)
	})
}
