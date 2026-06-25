package serverapi

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
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
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(extractBearerToken(r)), []byte(token)) != 1 {
			_ = apiframework.Error(w, r, apiframework.ErrUnauthorized, apiframework.GetOperation)
			return
		}
		next.ServeHTTP(w, r)
	})
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
