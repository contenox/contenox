package serverapi

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/contenox/runtime/apiframework"
)

func ValidateLocalServeSecurity(addr, token string) error {
	if IsLoopbackAddress(addr) {
		return nil
	}
	if strings.TrimSpace(token) != "" {
		return nil
	}
	return fmt.Errorf("TOKEN is required when ADDR is not loopback; use ADDR=127.0.0.1 for local-only serving")
}

func IsLoopbackAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	addr = strings.TrimPrefix(strings.TrimSuffix(addr, "]"), "[")
	if strings.EqualFold(addr, "localhost") {
		return true
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.IsLoopback()
}

// ProtectAPI is the primary /api/* gate. When a TOKEN is configured, EVERY
// request — all methods including GET/HEAD, same-origin or cross-site — must
// present a valid credential (a session-cookie JWT from /ui/login, or the raw
// TOKEN as a Bearer / X-API-Key header for programmatic clients); anything else
// is 401. This closes the same-origin-read hole where a LAN attacker's browser,
// or any local script, could read /api/state, /api/backends, /api/mcp-servers,
// etc. with no credential because only mutations were gated.
//
// When NO token is configured (the loopback zero-friction case), the historical
// CSRF stance is preserved: reads pass, and browser-originated mutations must be
// same-origin (or an explicitly allowed origin) — a cross-site browser mutation
// is 403. Non-loopback binds already require a TOKEN (ValidateLocalServeSecurity),
// so the no-token branch only ever serves local development.
func ProtectAPI(token, allowedOrigins string, next http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	allowedOrigins = strings.TrimSpace(allowedOrigins)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			if !requireCredential(w, r, token) {
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if isCrossSiteBrowserMutation(r, allowedOrigins) {
			_ = apiframework.Error(w, r, apiframework.Forbidden("cross-origin mutating requests require TOKEN"), apiframework.AuthorizeOperation)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ProtectMutatingAPI wraps next with ProtectAPI and no explicit allowed origins.
func ProtectMutatingAPI(token string, next http.Handler) http.Handler {
	return ProtectAPI(token, "", next)
}

// ProtectMutatingAPIWithAllowedOrigins is retained as an alias for ProtectAPI so
// existing callers keep compiling; the name is historical (the wrapper no longer
// gates only mutations — see ProtectAPI).
func ProtectMutatingAPIWithAllowedOrigins(token, allowedOrigins string, next http.Handler) http.Handler {
	return ProtectAPI(token, allowedOrigins, next)
}

// requireCredential enforces that the request carries a valid credential (cookie
// JWT or raw token) when a TOKEN is configured, writing a 401 and returning false
// otherwise.
func requireCredential(w http.ResponseWriter, r *http.Request, token string) bool {
	if !AuthenticateCredential(token, extractRequestToken(r)) {
		_ = apiframework.Error(w, r, apiframework.ErrUnauthorized, apiframework.GetOperation)
		return false
	}
	return true
}

// sessionCookieName is the HttpOnly cookie the Beam login flow (see ui_auth.go)
// sets, carrying a signed session JWT (see session_auth.go) that browser
// requests present as their credential. It matches the cookie name terminalapi
// and the /acp WebSocket handler already read, so one login satisfies the API,
// the terminal, and the ACP upgrade uniformly.
const sessionCookieName = "auth_token"

// extractRequestToken returns the caller's raw credential string from, in order:
// the Authorization/X-API-Key bearer header (programmatic clients present the
// raw TOKEN), then the HttpOnly session cookie (browser clients present the
// session JWT minted by /ui/login). The value is handed to AuthenticateCredential,
// which accepts either form. The cookie rides automatically on same-origin
// requests — including the /acp WebSocket upgrade — so a logged-in browser needs
// no explicit token plumbing, while Bearer/X-API-Key keep working for scripts.
func extractRequestToken(r *http.Request) string {
	if t := extractBearerToken(r); t != "" {
		return t
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func extractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
		return strings.TrimSpace(apiKey[7:])
	}
	return apiKey
}

func isCrossSiteBrowserMutation(r *http.Request, allowedOrigins string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return !originMatchesRequest(origin, r) && !originExplicitlyAllowed(origin, allowedOrigins)
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site")
}

func originMatchesRequest(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	reqScheme := "http"
	if r.TLS != nil {
		reqScheme = "https"
	}
	return strings.EqualFold(u.Scheme, reqScheme) && strings.EqualFold(u.Host, r.Host)
}

func originExplicitlyAllowed(origin, allowedOrigins string) bool {
	for _, allowed := range strings.Split(allowedOrigins, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" || allowed == "*" {
			continue
		}
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}
