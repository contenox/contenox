package apiframework

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type ContextKey string

const (
	ContextTokenKey ContextKey = "token"
)

func TokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		authHeader := r.Header.Get("X-API-Key")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			ctx = context.WithValue(ctx, ContextTokenKey, token)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func EnforceToken(expectedToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestToken, ok := r.Context().Value(ContextTokenKey).(string)
		if !ok || subtle.ConstantTimeCompare([]byte(requestToken), []byte(expectedToken)) != 1 {
			_ = Error(w, r, ErrUnauthorized, AuthorizeOperation)
			return
		}
		next.ServeHTTP(w, r)
	})
}
