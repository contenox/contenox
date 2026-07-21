package terminalapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/terminalservice"
	"golang.org/x/net/websocket"
)

const localTerminalPrincipal = "local-user"

// AddRoutes registers interactive terminal endpoints. If enabled is false, this is a no-op.
//
// authenticate is the credential gate for every terminal route INCLUDING the
// websocket handshake. It is a FUNCTION, not a raw token, for two load-bearing
// reasons: (1) the terminal must accept exactly the credentials every other
// serve surface accepts — the raw shared secret AND the browser's session JWT
// cookie minted by /ui/login (serverapi.AuthenticateCredential is the one
// gate; a raw-only ConstantTimeCompare here once rejected Beam's cookie with a
// 401 while /acp accepted it — one login must satisfy every surface); and
// (2) serverapi imports this package, so importing the gate from there would
// cycle — the caller injects it instead. nil means "no token configured":
// open, matching the loopback-only serving mode that permits an empty TOKEN.
func AddRoutes(mux *http.ServeMux, svc terminalservice.Service, auth middleware.AuthZReader, enabled bool, authenticate func(credential string) bool) {
	if !enabled {
		return
	}
	if svc == nil {
		svc = terminalservice.NewDisabled()
	}
	h := &handler{svc: svc, auth: auth, authenticate: authenticate}
	mux.HandleFunc("GET /terminal/sessions", h.listSessions)
	mux.HandleFunc("POST /terminal/sessions", h.createSession)
	mux.HandleFunc("GET /terminal/sessions/{id}", h.getSession)
	mux.HandleFunc("DELETE /terminal/sessions/{id}", h.deleteSession)
	mux.Handle("GET /terminal/sessions/{id}/ws", h.wsHandler())
}

type handler struct {
	svc          terminalservice.Service
	auth         middleware.AuthZReader
	authenticate func(credential string) bool
}

type createSessionRequest struct {
	CWD   string `json:"cwd"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Shell string `json:"shell,omitempty"`
}

type createSessionResponse struct {
	ID     string `json:"id"`
	WSPath string `json:"wsPath"`
}

type wsErrorFrame struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (h *handler) principal(ctx context.Context) (string, error) {
	if h.auth == nil {
		return localTerminalPrincipal, nil
	}
	principal, err := h.auth.GetIdentity(ctx)
	if err != nil {
		return "", err
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return localTerminalPrincipal, nil
	}
	return principal, nil
}

func (h *handler) requireToken(r *http.Request) error {
	if h.authenticate == nil {
		return nil
	}
	if !h.authenticate(extractTerminalToken(r)) {
		return apiframework.ErrUnauthorized
	}
	return nil
}

func (h *handler) principalFromRequest(r *http.Request) (string, error) {
	if err := h.requireToken(r); err != nil {
		return "", err
	}
	return h.principal(r.Context())
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	op := apiframework.AuthorizeOperation
	if errors.Is(err, apiframework.ErrUnauthorized) {
		op = apiframework.GetOperation
	}
	_ = apiframework.Error(w, r, err, op)
}

func extractTerminalToken(r *http.Request) string {
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
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	if cookie, err := r.Cookie("auth_token"); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func (h *handler) createSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, err := h.principalFromRequest(r)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	req, err := apiframework.Decode[createSessionRequest](r) // @request terminalapi.createSessionRequest
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}

	out, err := h.svc.Create(ctx, principal, terminalservice.CreateRequest{
		CWD:   strings.TrimSpace(req.CWD),
		Cols:  req.Cols,
		Rows:  req.Rows,
		Shell: strings.TrimSpace(req.Shell),
	})
	if err != nil {
		if errors.Is(err, terminalservice.ErrTooManySessions) {
			_ = apiframework.Error(w, r, apiframework.UnprocessableEntity(err.Error()), apiframework.CreateOperation)
			return
		}
		_ = apiframework.Error(w, r, err, apiframework.CreateOperation)
		return
	}
	resp := createSessionResponse{
		ID:     out.ID,
		WSPath: "/api/terminal/sessions/" + out.ID + "/ws",
	}
	_ = apiframework.Encode(w, r, http.StatusCreated, resp) // @response terminalapi.createSessionResponse
}

func (h *handler) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, err := h.principalFromRequest(r)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	cursor, limit, err := apiframework.ListParams(r, 100)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	items, err := h.svc.List(ctx, principal, cursor, limit)
	if err != nil {
		if errors.Is(err, runtimetypes.ErrLimitParamExceeded) {
			_ = apiframework.Error(w, r, err, apiframework.ListOperation)
			return
		}
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, items) // @response []terminalstore.Session
}

func (h *handler) getSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, err := h.principalFromRequest(r)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	id := strings.TrimSpace(apiframework.GetPathParam(r, "id", "The unique identifier of the terminal session."))
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("id is required"), apiframework.GetOperation)
		return
	}
	sess, err := h.svc.Get(ctx, principal, id)
	if err != nil {
		if errors.Is(err, terminalservice.ErrSessionNotFound) {
			_ = apiframework.Error(w, r, apiframework.NotFound("terminal session not found"), apiframework.GetOperation)
			return
		}
		_ = apiframework.Error(w, r, err, apiframework.GetOperation)
		return
	}
	_ = apiframework.Encode(w, r, http.StatusOK, sess) // @response terminalstore.Session
}

func (h *handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	principal, err := h.principalFromRequest(r)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	id := strings.TrimSpace(apiframework.GetPathParam(r, "id", "The unique identifier of the terminal session."))
	if id == "" {
		_ = apiframework.Error(w, r, apiframework.BadRequest("id is required"), apiframework.DeleteOperation)
		return
	}
	if err := h.svc.Close(r.Context(), principal, id); err != nil {
		if errors.Is(err, terminalservice.ErrSessionNotFound) {
			_ = apiframework.Error(w, r, apiframework.NotFound("terminal session not found"), apiframework.DeleteOperation)
			return
		}
		_ = apiframework.Error(w, r, err, apiframework.DeleteOperation)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) wsHandler() http.Handler {
	s := &websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			cfg.Origin, _ = websocket.Origin(cfg, req)
			// Enforce the token at the handshake so an unauthenticated upgrade is
			// rejected with 403 before the 101 switch, not accepted and then
			// silently dropped inside the handler.
			return h.requireToken(req)
		},
	}
	s.Handler = func(ws *websocket.Conn) {
		req := ws.Request()
		principal, err := h.principal(req.Context())
		if err != nil {
			return
		}
		id := req.PathValue("id")
		if id == "" {
			parts := strings.Split(req.URL.Path, "/")
			for i, p := range parts {
				if p == "sessions" && i+1 < len(parts) && parts[i+1] != "ws" {
					id = parts[i+1]
					break
				}
			}
		}
		if id == "" {
			return
		}

		ws.PayloadType = websocket.BinaryFrame
		resizeCh := make(chan terminalservice.ResizeMsg, 4)
		defer close(resizeCh)

		rw := &termConn{ws: ws, resizeCh: resizeCh}
		if err := h.svc.Attach(context.Background(), principal, id, rw, resizeCh); err != nil {
			slog.Error("terminal attach error", "session", id, "error", err)
			_ = writeWSError(ws, attachErrorMessage(err))
		}
	}
	return s
}

type termConn struct {
	ws       *websocket.Conn
	resizeCh chan<- terminalservice.ResizeMsg
	buf      []byte
}

func (c *termConn) Read(p []byte) (int, error) {
	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}
	for {
		buf := make([]byte, 32*1024)
		n, err := c.ws.Read(buf)
		if err != nil {
			return 0, err
		}
		data := buf[:n]

		var msg struct {
			Type string `json:"type"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
			select {
			case c.resizeCh <- terminalservice.ResizeMsg{Cols: msg.Cols, Rows: msg.Rows}:
			default:
			}
			continue
		}

		copied := copy(p, data)
		if copied < len(data) {
			c.buf = data[copied:]
		}
		return copied, nil
	}
}

func (c *termConn) Write(p []byte) (int, error) { return c.ws.Write(p) }
func (c *termConn) Close() error                { return c.ws.Close() }

func attachErrorMessage(err error) string {
	if errors.Is(err, terminalservice.ErrSessionNotFound) {
		return "session not found"
	}
	var apiErr *apiframework.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Error()
	}
	if err != nil {
		return err.Error()
	}
	return "terminal attach failed"
}

func writeWSError(ws *websocket.Conn, message string) error {
	frame, err := json.Marshal(wsErrorFrame{Type: "error", Message: message})
	if err != nil {
		return err
	}
	_, err = ws.Write(frame)
	return err
}
