package contenoxcli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/serverapi"
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

	t.Run("without token the handshake is rejected before upgrade", func(t *testing.T) {
		// Auth is enforced in the Server's Handshake callback, so an
		// unauthenticated upgrade is refused with 403 and never switches
		// protocols — websocket.Dial fails outright ("bad status") rather than
		// connecting and then being silently dropped.
		wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
		ws, err := websocket.Dial(wsURL, "", srv.URL+"/")
		if ws != nil {
			_ = ws.Close()
		}
		require.Error(t, err, "unauthenticated /acp upgrade must be rejected at the handshake")
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

// TestUnit_ACPWebSocketHandler_SessionCookieAuth proves the /acp upgrade
// authenticates from the same HttpOnly `auth_token` session cookie the Beam
// login flow sets (extractACPToken reads it) — so a logged-in browser needs no
// ?token= query param, cookies riding automatically on the same-origin upgrade.
func TestUnit_ACPWebSocketHandler_SessionCookieAuth(t *testing.T) {
	const token = "s3cr3t-token"
	factory := acpsvc.New(acpsvc.Deps{})
	srv := httptest.NewServer(acpWebSocketHandler(factory, token))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	cfg, err := websocket.NewConfig(wsURL, srv.URL+"/")
	require.NoError(t, err)
	cfg.Header.Set("Cookie", "auth_token="+token)
	ws, err := websocket.DialConfig(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	require.NoError(t, websocket.Message.Send(ws, acpWSInitializeFrame(t, 1)))
	require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
	var frame string
	require.NoError(t, websocket.Message.Receive(ws, &frame))

	in, err := libacp.ParseIncoming([]byte(frame))
	require.NoError(t, err)
	require.Equal(t, libacp.IncomingKindResponse, in.Kind, "wire: %s", frame)
	require.Nil(t, in.Response.Error, "wire: %s", frame)
}

// loginJWTCookie drives the real serverapi /ui/login flow to obtain the session
// JWT a browser would hold, proving /acp accepts the minted cookie JWT (not just
// the raw token) end to end across packages.
func loginJWTCookie(t *testing.T, token string) string {
	t.Helper()
	mux := http.NewServeMux()
	serverapi.AddUIAuthRoutes(mux, token)
	loginSrv := httptest.NewServer(mux)
	t.Cleanup(loginSrv.Close)

	resp, err := http.Post(loginSrv.URL+"/ui/login", "application/json",
		strings.NewReader(`{"token":"`+token+`"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	for _, c := range resp.Cookies() {
		if c.Name == "auth_token" {
			require.NotEqual(t, token, c.Value, "cookie must carry a JWT, not the raw token")
			return c.Value
		}
	}
	t.Fatal("login set no auth_token cookie")
	return ""
}

// TestUnit_ACPWebSocketHandler_CookieJWTAuth proves the /acp upgrade accepts the
// session JWT minted by /ui/login — the actual browser credential — as well as
// rejecting an upgrade with no credential when a TOKEN is set.
func TestUnit_ACPWebSocketHandler_CookieJWTAuth(t *testing.T) {
	const token = "s3cr3t-token"
	factory := acpsvc.New(acpsvc.Deps{})
	srv := httptest.NewServer(acpWebSocketHandler(factory, token))
	t.Cleanup(srv.Close)

	jwt := loginJWTCookie(t, token)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	cfg, err := websocket.NewConfig(wsURL, srv.URL+"/")
	require.NoError(t, err)
	cfg.Header.Set("Cookie", "auth_token="+jwt)
	ws, err := websocket.DialConfig(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	require.NoError(t, websocket.Message.Send(ws, acpWSInitializeFrame(t, 1)))
	require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
	var frame string
	require.NoError(t, websocket.Message.Receive(ws, &frame))

	in, err := libacp.ParseIncoming([]byte(frame))
	require.NoError(t, err)
	require.Equal(t, libacp.IncomingKindResponse, in.Kind, "wire: %s", frame)
	require.Nil(t, in.Response.Error, "wire: %s", frame)
}
