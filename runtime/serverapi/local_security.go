package serverapi

import (
	"crypto/subtle"
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

func ProtectMutatingAPI(token string, next http.Handler) http.Handler {
	return ProtectMutatingAPIWithAllowedOrigins(token, "", next)
}

func ProtectMutatingAPIWithAllowedOrigins(token, allowedOrigins string, next http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	allowedOrigins = strings.TrimSpace(allowedOrigins)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			if isMutatingMethod(r.Method) || isCrossSiteBrowserRead(r) {
				if !requireBearerToken(w, r, token) {
					return
				}
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

func requireBearerToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if subtle.ConstantTimeCompare([]byte(extractBearerToken(r)), []byte(token)) != 1 {
		_ = apiframework.Error(w, r, apiframework.ErrUnauthorized, apiframework.GetOperation)
		return false
	}
	return true
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

func isCrossSiteBrowserRead(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return !originMatchesRequest(origin, r)
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
