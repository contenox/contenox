package contenoxcli

import (
	"errors"
	"net/http"
	"strings"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/serverapi"
	"golang.org/x/net/websocket"
)

// errACPUnauthorized rejects the WebSocket handshake before the 101 upgrade when
// the caller presents no valid credential. Returning it from the Server's
// Handshake callback makes golang.org/x/net/websocket answer 403 and never
// switch protocols — the credential must be proven before, not after, the
// upgrade. (Checking inside the Handler would still close an unauthenticated
// socket with zero data served, but only after a misleading 101.)
var errACPUnauthorized = errors.New("acp: unauthorized websocket handshake")

// acpWSConn adapts a golang.org/x/net/websocket text-frame connection to the
// io.ReadWriteCloser libacp expects: one NDJSON line (a JSON value + "\n")
// per logical message. libacp's ndjsonWriter emits each message as two
// underlying Write calls — the JSON bytes, then a lone "\n" — so Write
// buffers until it observes the trailing newline before flushing a single
// WebSocket TEXT frame. Read does the inverse: it pulls one TEXT frame at a
// time and hands it back newline-terminated, buffering any remainder for
// subsequent calls (libacp's bufio reader may ask for less than a full frame).
type acpWSConn struct {
	ws       *websocket.Conn
	readBuf  []byte
	writeBuf []byte
}

func (c *acpWSConn) Read(p []byte) (int, error) {
	if len(c.readBuf) == 0 {
		var frame string
		if err := websocket.Message.Receive(c.ws, &frame); err != nil {
			return 0, err
		}
		c.readBuf = append([]byte(frame), '\n')
	}
	n := copy(p, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

func (c *acpWSConn) Write(p []byte) (int, error) {
	c.writeBuf = append(c.writeBuf, p...)
	if len(c.writeBuf) > 0 && c.writeBuf[len(c.writeBuf)-1] == '\n' {
		frame := c.writeBuf[:len(c.writeBuf)-1]
		if err := websocket.Message.Send(c.ws, string(frame)); err != nil {
			return 0, err
		}
		c.writeBuf = c.writeBuf[:0]
	}
	return len(p), nil
}

func (c *acpWSConn) Close() error { return c.ws.Close() }

// acpWebSocketHandler serves the ACP JSON-RPC stream over a WebSocket at
// /acp: one text frame per message, backed by a fresh acpsvc.Transport per
// connection (factory is called once per WS connect by
// libacp.NewAgentSideConnection). It mirrors terminalapi's wsHandler
// (golang.org/x/net/websocket.Server, same bearer-token extraction) so /acp
// inherits serve's auth exactly like the terminal WebSocket does.
func acpWebSocketHandler(factory libacp.AgentFactory, token string) http.Handler {
	token = strings.TrimSpace(token)
	s := &websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			cfg.Origin, _ = websocket.Origin(cfg, req)
			// When a TOKEN is configured, the upgrade must present a valid credential:
			// either the session-cookie JWT a logged-in browser carries, or the raw
			// TOKEN via bearer header / ?token= (programmatic clients). Both are checked
			// by the same gate the /api/* middleware uses. Rejected here, pre-101.
			if token != "" && !serverapi.AuthenticateCredential(token, extractACPToken(req)) {
				return errACPUnauthorized
			}
			return nil
		},
	}
	s.Handler = func(ws *websocket.Conn) {
		req := ws.Request()
		ws.PayloadType = websocket.TextFrame
		adapter := &acpWSConn{ws: ws}
		conn := libacp.NewAgentSideConnection(adapter, factory)
		_ = conn.Run(req.Context())
	}
	return s
}

// extractACPToken mirrors terminalapi.extractTerminalToken: Authorization
// bearer header, X-API-Key, the "token" query parameter (WebSocket clients
// cannot always set headers), then the auth_token cookie.
func extractACPToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
		return strings.TrimSpace(apiKey[7:])
	}
	if apiKey != "" {
		return apiKey
	}
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		return tok
	}
	if cookie, err := r.Cookie("auth_token"); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
