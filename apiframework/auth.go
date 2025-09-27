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

		// Extract token from Authorization header
		authHeader := r.Header.Get("X-API-Key")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			ctx = context.WithValue(ctx, ContextTokenKey, token)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func EnforceToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if token, ok := ctx.Value(ContextTokenKey).(string); !ok ||
			subtle.ConstantTimeCompare([]byte(token), []byte(token)) != 1 {
			err := ErrUnauthorized
			Error(w, r, err, AuthorizeOperation)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
