package serverapi

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Beam remote-access login.
//
// These three routes back the Beam login page: a browser POSTs the configured
// shared secret to /ui/login, the server verifies it and hands back an HttpOnly
// session cookie, and every subsequent request (API mutations, the terminal, the
// /acp WebSocket upgrade) authenticates from that cookie automatically — no
// URL-param or localStorage token plumbing on the UI path.
//
// Cookie value choice: the cookie carries a short-lived signed session JWT (see
// session_auth.go), NOT the raw TOKEN. On a correct login the server mints a JWT
// emulating a fixed admin principal ("local-operator"), signed with a secret
// HKDF-derived from the configured TOKEN. Every request then authenticates via
// AuthenticateCredential, which accepts either that cookie JWT (browser path) or
// the raw TOKEN as a Bearer/X-API-Key header (programmatic path). The raw secret
// therefore never lands in the browser: the cookie is HttpOnly (script cannot
// read it), SameSite=Strict (not sent cross-site), Secure over TLS, and expires
// on its own within 24h even if exfiltrated. terminalapi and the /acp handler
// read the same `auth_token` cookie, so one login satisfies all three surfaces.

const (
	defaultLoginMaxAttempts = 20
	defaultLoginWindow      = time.Minute
)

// AddUIAuthRoutes registers the Beam login endpoints on mux. Wire it on the
// serve root mux only (outside the /api protection wrapper, so /ui/login is
// reachable without already holding the cookie it issues). token is the
// configured shared secret; when empty, login is not required and the endpoints
// report so while remaining harmless no-ops.
func AddUIAuthRoutes(mux *http.ServeMux, token string) {
	token = strings.TrimSpace(token)
	limiter := newLoginLimiter(defaultLoginMaxAttempts, defaultLoginWindow)
	mux.Handle("POST /ui/login", uiLoginHandler(token, limiter))
	mux.Handle("POST /ui/logout", uiLogoutHandler(token))
	mux.Handle("GET /ui/auth-status", uiAuthStatusHandler(token))
}

type authStatusResponse struct {
	Required      bool `json:"required"`
	Authenticated bool `json:"authenticated"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// requestIsSecure reports whether the browser reached us over TLS, so the
// session cookie is marked Secure only when it will actually be sent back.
// Direct TLS sets r.TLS; a TLS-terminating reverse proxy (the common remote
// deployment) forwards plain HTTP but sets X-Forwarded-Proto=https. Marking a
// cookie Secure over plain HTTP would silently drop it and break login, so we
// gate strictly on observed HTTPS.
func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		// No Expires/MaxAge: a session cookie, cleared when the browser closes or
		// on explicit /ui/logout.
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func uiLoginHandler(token string, limiter *loginLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			// No secret configured: nothing to authenticate against. Report the
			// state truthfully rather than issuing a meaningless cookie.
			writeJSON(w, http.StatusOK, authStatusResponse{Required: false, Authenticated: true})
			return
		}
		if !limiter.allow(clientIP(r)) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many login attempts"})
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		// Constant-time compare so a wrong token leaks no timing signal about how
		// many leading bytes matched.
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Token)), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, authStatusResponse{Required: true, Authenticated: false})
			return
		}
		// Mint a short-lived session JWT (fixed admin principal) instead of storing
		// the raw secret in the browser — see session_auth.go.
		jwtStr, _, err := mintSessionToken(token)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
			return
		}
		setSessionCookie(w, r, jwtStr)
		writeJSON(w, http.StatusOK, authStatusResponse{Required: true, Authenticated: true})
	})
}

func uiLogoutHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusOK, authStatusResponse{Required: token != "", Authenticated: false})
	})
}

func uiAuthStatusHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		required := token != ""
		authenticated := !required || AuthenticateCredential(token, extractRequestToken(r))
		writeJSON(w, http.StatusOK, authStatusResponse{Required: required, Authenticated: authenticated})
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// loginLimiter is a fixed-window per-client-IP throttle on /ui/login. It bounds
// online brute-force attempts against the shared secret (the constant-time
// compare already defeats timing side channels; this caps raw guess volume).
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]*loginWindow
}

type loginWindow struct {
	start time.Time
	count int
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		max:    max,
		window: window,
		now:    time.Now,
		hits:   map[string]*loginWindow{},
	}
}

// allow records an attempt for key and reports whether it is within the current
// window's budget. Both successful and failed logins consume budget, so a flood
// of guesses from one IP is capped regardless of outcome.
func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	wc := l.hits[key]
	if wc == nil || now.Sub(wc.start) >= l.window {
		l.hits[key] = &loginWindow{start: now, count: 1}
		return true
	}
	if wc.count >= l.max {
		return false
	}
	wc.count++
	return true
}
