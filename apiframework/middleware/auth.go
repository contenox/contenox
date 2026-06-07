package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/libauth"
)

// ExtractAndSetTokenMiddleware extracts a token from Authorization header or
// auth_token cookie and injects it into the context under libauth.ContextTokenKey.
func ExtractAndSetTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			token = strings.TrimSpace(authHeader[7:])
		}

		if token == "" {
			if cookie, err := r.Cookie("auth_token"); err == nil && cookie != nil {
				token = cookie.Value
			}
		}

		ctx := r.Context()
		if token != "" {
			ctx = context.WithValue(ctx, libauth.ContextTokenKey, token)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// JWTAuthMiddleware validates the token and enriches context with identity and
// permissions. Missing tokens are passed through so route-level auth can decide
// whether the endpoint is public.
func JWTAuthMiddleware(tokenManager AuthzManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if _, ok := ctx.Value(libauth.ContextTokenKey).(string); !ok {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		validatedCtx, err := tokenManager.ValidateAuthToken(ctx)
		if err != nil {
			_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
			return
		}

		next.ServeHTTP(w, r.WithContext(validatedCtx))
	})
}

// JWTRefreshMiddleware attempts to refresh browser-client tokens.
func JWTRefreshMiddleware(tokenManager AuthzManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userAgent := r.Header.Get("User-Agent")
		isBrowser := userAgent != "" && !strings.Contains(strings.ToLower(r.Header.Get("X-Requested-With")), "xmlhttprequest")
		if !isBrowser {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		tokenStr, err := tokenManager.GetTokenString(ctx)
		if err != nil {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		gracePeriod := 20 * time.Minute
		newToken, wasReplaced, expiresAt, err := tokenManager.RefreshToken(ctx, tokenStr, &gracePeriod)
		if err != nil {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if wasReplaced {
			cookie := &http.Cookie{
				Name:     "auth_token",
				Value:    newToken,
				Path:     "/",
				Expires:  expiresAt.UTC(),
				SameSite: http.SameSiteStrictMode,
				HttpOnly: true,
				Secure:   r.TLS != nil,
			}
			http.SetCookie(w, cookie)

			ctx = context.WithValue(ctx, libauth.ContextTokenKey, newToken)
			ctx, err = tokenManager.ValidateAuthToken(ctx)
			if err != nil {
				_ = apiframework.Error(w, r, err, apiframework.AuthorizeOperation)
				return
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type AuthzManager interface {
	RefreshToken(ctx context.Context, tokenString string, withGracePeriod *time.Duration) (string, bool, time.Time, error)
	CreateAuthToken(ctx context.Context, subject string, permissions libauth.Authz) (string, time.Time, error)
	ValidateAuthToken(ctx context.Context) (context.Context, error)
	SetToken(ctx context.Context, tokenString string) (context.Context, error)
	AuthZReader
}

type AuthZReader interface {
	GetIdentity(ctx context.Context) (string, error)
	GetUsername(ctx context.Context) (string, error)
	GetPermissions(ctx context.Context) (libauth.Authz, error)
	GetTokenString(ctx context.Context) (string, error)
	GetExpiresAt(ctx context.Context) (time.Time, error)
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(libauth.ContextTokenKey).(string); !ok {
			_ = apiframework.Error(w, r, apiframework.ErrUnauthorized, apiframework.AuthorizeOperation)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type LoginManager interface {
	Login(ctx context.Context, username, password string) (LoginResponse, error)
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
}

func GetLoginResponse(ctx context.Context, auth AuthZReader) (LoginResponse, error) {
	tokenString, err := auth.GetTokenString(ctx)
	if err != nil {
		return LoginResponse{}, err
	}
	expiresAt, err := auth.GetExpiresAt(ctx)
	if err != nil {
		return LoginResponse{}, err
	}
	userID, err := auth.GetIdentity(ctx)
	if err != nil {
		return LoginResponse{}, err
	}
	username, err := auth.GetUsername(ctx)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{
		Token:     tokenString,
		ExpiresAt: expiresAt,
		UserID:    userID,
		Username:  username,
	}, nil
}
