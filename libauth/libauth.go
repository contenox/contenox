// Package libauth provides secure authentication and authorization services using JWT tokens.
// It includes functionality for token creation, validation, refresh, password hashing,
// and permission checks. The package is designed to be extensible with custom permission
// systems through the AA interface.
package libauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Common error variables for libauth
var (
	ErrNotAuthorized           = errors.New("libauth: not authorized")
	ErrTokenExpired            = errors.New("libauth: token has expired")
	ErrIssuedAtMissing         = errors.New("libauth: issued at claim is missing")
	ErrIssuedAtInFuture        = errors.New("libauth: token is malformed: issued-at time is in the future")
	ErrIdentityMissing         = errors.New("libauth: identity claim is missing")
	ErrInvalidTokenClaims      = errors.New("libauth: invalid token claims")
	ErrTokenMissing            = errors.New("libauth: token is missing")
	ErrUnexpectedSigningMethod = errors.New("libauth: unexpected signing method")
	ErrTokenParsingFailed      = errors.New("libauth: token parsing failed")
	ErrTokenSigningFailed      = errors.New("libauth: token signing failed")
	ErrEmptyJWTSecret          = errors.New("libauth: JWT secret cannot be empty")
	ErrEmptyIdentity           = errors.New("libauth: identity cannot be empty")
)

// CreateTokenArgs holds the required settings for JWT creation.
// Example:
//
//	cfg := CreateTokenArgs{
//	    JWTSecret: "your-strong-secret",
//	    JWTExpiry: 2 * time.Hour, // token valid for 2 hours
//	}
type CreateTokenArgs struct {
	JWTSecret string
	JWTExpiry time.Duration
}

// AuthClaims defines the JWT claims structure.
// Generic type T must implement the AA interface (custom permission system)
// which requires implementations for both authentication and authorization.
type AuthClaims[T Authz] struct {
	jwt.RegisteredClaims
	Identity    string `json:"identity"`
	Permissions T      `json:"permissions"`
}

// Authz: authentication and authorization the interface defines methods for permission and authentication checks.
//
// Example usage:
//
//	type MyPermissions struct { /* custom permission fields */ }
//	func (p MyPermissions) RequireAuthorisation(forResource string, permission int) (bool, error) {
//	    // Your permission logic here.
//	}
//	func (p MyPermissions) RequireAuthentication(forIdentity string) (bool, error) {
//	    // Your authentication logic here.
//	}
type Authz interface {
	// RequireAuthorisation checks if the user has the required permission for the resource.
	// params:
	// - forResource (string) - the resource to check permission for
	// - permission (int) - the permission level required
	// returns: (bool, error) - true if the user has the required permission, false otherwise, and an error if any
	RequireAuthorisation(forResource string, permission int) (bool, error)
}

// Valid implements the jwt.Claims interface, checking the validity of the claims.
// It validates the expiration, issued-at, and identity claims.
func (c AuthClaims[T]) Valid() error {
	exp, err := c.GetExpirationTime()
	if err != nil {
		return fmt.Errorf("failed to parse expiration time: %w: %w", ErrTokenParsingFailed, err)
	}
	if exp == nil || time.Now().UTC().After(exp.Time) {
		return ErrTokenExpired
	}

	iat, err := c.GetIssuedAt()
	if err != nil {
		return fmt.Errorf("failed to parse issued at: %w: %w", ErrTokenParsingFailed, err)
	}
	if iat == nil {
		return ErrIssuedAtMissing
	}
	if time.Now().UTC().Before(iat.Time) {
		return ErrIssuedAtInFuture
	}

	if c.Identity == "" {
		return ErrIdentityMissing
	}

	return nil
}

// contextKey is a private type for storing values in context.
type contextKey string

// ContextTokenKey is the key used to store the JWT token string in the context.
// Use context.WithValue(ctx, ContextTokenKey, tokenStr) to inject the token.
const ContextTokenKey = contextKey("jwtToken")

// ValidateToken parses and validates a JWT token string.
// params:
// - ctx (context.Context) - the context that may hold the JWT token for downstream use
// - tokenStr (string) - the JWT token string to validate
// - jwtSecret (string) - the JWT secret to use for validation
//
// Example:
//
//	ctx := r.Context()
//	claims, err := ValidateToken[MyPermissions](ctx, tokenStr, "your-strong-secret")
//	if err != nil {
//	    // Handle invalid token (expired, malformed, etc.)
//	}
//	fmt.Printf("Token is valid for user: %s\n", claims.Identity)
func ValidateToken[T Authz](ctx context.Context, tokenStr string, jwtSecret string) (*AuthClaims[T], error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AuthClaims[T]{}, func(t *jwt.Token) (any, error) {
		// Ensure the token is signed with an HMAC-based method.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnexpectedSigningMethod
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenParsingFailed, err)
	}
	claims, ok := token.Claims.(*AuthClaims[T])
	if !ok || !token.Valid {
		return nil, ErrInvalidTokenClaims
	}
	if !token.Valid {
		// If we get here and the token isn't valid, check if it's due to expiration
		if time.Now().UTC().After(claims.ExpiresAt.Time) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidTokenClaims
	}

	return claims, nil
}

// RefreshTokenWithGracePeriod refreshes a JWT token only if it's within a given grace period before expiry,
//
//	if not, returns the old token.
func RefreshTokenWithGracePeriod[T Authz](cfg CreateTokenArgs, oldToken string, gracePeriod time.Duration) (string, bool, time.Time, error) {
	// Parse the old token
	token, err := jwt.ParseWithClaims(oldToken, &AuthClaims[T]{}, func(token *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return "", false, time.Time{}, fmt.Errorf("%w: %w", ErrTokenParsingFailed, err)
	}

	// Extract claims
	claims, ok := token.Claims.(*AuthClaims[T])
	if !ok || !token.Valid {
		return "", false, time.Time{}, ErrInvalidTokenClaims
	}

	// Check if the token is within the grace period before expiry
	expiryTime, err := claims.GetExpirationTime()
	if err != nil || expiryTime == nil {
		return "", false, time.Time{}, ErrTokenParsingFailed
	}
	if time.Until(expiryTime.Time) > gracePeriod {
		return oldToken, false, time.Time{}, nil
	}
	newToken, expiresAt, err := RefreshToken[T](cfg, oldToken)
	// Refresh token normally
	return newToken, true, expiresAt, err
}

// RefreshToken generates a new JWT from an existing token.
// params:
// - cfg (CreateTokenArgs) - the configuration for creating the new token
// - oldToken (string) - the existing token to refresh
// returns: (string, error) - the new token string and an error if any
// Note: RefreshToken only works for non-expired tokens.
func RefreshToken[T Authz](cfg CreateTokenArgs, oldToken string) (string, time.Time, error) {
	// Parse the old token
	token, err := jwt.ParseWithClaims(oldToken, &AuthClaims[T]{}, func(token *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", ErrTokenParsingFailed, err)
	}

	// Extract claims
	claims, ok := token.Claims.(*AuthClaims[T])
	if !ok || !token.Valid {
		return "", time.Time{}, ErrInvalidTokenClaims
	}

	// Generate a new expiry time
	expiresAt := time.Now().Add(cfg.JWTExpiry).UTC()

	// Create new token with refreshed expiry
	newClaims := AuthClaims[T]{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		Identity:    claims.Identity,
		Permissions: claims.Permissions,
	}

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	t, err := newToken.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", expiresAt, fmt.Errorf("%w: %w", ErrTokenSigningFailed, err)
	}
	return t, expiresAt, nil
}

// CreateToken creates a new JWT for a given identity and permission payload.
// params:
// - cfg (CreateTokenArgs) - the configuration for creating the new token
// - identity (string) - the identity for which to create the token
// - payload (T) - the permission payload for the token
// returns: (string, error) - the new token string and an error if any
//
// Example:
//
//	token, err := CreateToken(cfg, "user123", myPermissions)
//	if err != nil {
//	    // Handle error: check for empty secret or identity.
//	}
//	fmt.Println("Generated token:", token)
func CreateToken[T Authz](cfg CreateTokenArgs, identity string, payload T) (string, time.Time, error) {
	if cfg.JWTSecret == "" {
		return "", time.Time{}, ErrEmptyJWTSecret
	}
	if identity == "" {
		return "", time.Time{}, ErrEmptyIdentity
	}

	expiresAt := time.Now().UTC().Add(cfg.JWTExpiry)
	claims := AuthClaims[T]{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   identity,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		Identity:    identity,
		Permissions: payload,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	final, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return final, expiresAt, nil
}

// GetClaims retrieves the permission claims from the context.
// params:
// - ctx (context.Context) - the context containing the JWT token, injected via ContextTokenKey
// - jwtSecret (string) - the JWT secret key used to verify the token
// returns: (T, error) - the permission claims and an error if any
func GetClaims[T Authz](ctx context.Context, jwtSecret string) (T, error) {
	var permClaims T
	token, ok := ctx.Value(ContextTokenKey).(string)
	if !ok {
		return permClaims, ErrTokenMissing
	}

	permList, err := ValidateToken[T](ctx, token, jwtSecret)
	if err != nil {
		return permClaims, fmt.Errorf("%w: %w", ErrInvalidTokenClaims, err)
	}

	return permList.Permissions, nil
}

// CheckAuthorisation verifies that the JWT holds permission for a specific resource and permission.
// params:
// - ctx (context.Context) - the context containing the JWT token
// - jwtSecret (string) - the JWT secret key used to verify the token
// - forResource (string) - the resource to check permission for
// - permission (int) - the permission level required
// returns: (bool, error) - true if the JWT holds permission, false otherwise, and an error if any
// false, nil -> denied
// false, err -> validation error
func CheckAuthorisation[T Authz](ctx context.Context, jwtSecret string, forResource string, permission int) (bool, error) {
	permList, err := GetClaims[T](ctx, jwtSecret)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrNotAuthorized, err)
	}
	ok, err := permList.RequireAuthorisation(forResource, permission)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrNotAuthorized, err)
	}
	if !ok {
		return false, nil
	}

	return true, nil
}

// GetIdentity extracts the identity (subject) from the JWT token stored in the context.
// params:
// - ctx (context.Context) - the context containing the JWT token
// - jwtSecret (string) - the JWT secret key used to verify the token
// returns: (string, error) - the identity (subject) of the JWT token, and an error if any
func GetIdentity[T Authz](ctx context.Context, jwtSecret string) (string, error) {
	token, ok := ctx.Value(ContextTokenKey).(string)
	if !ok {
		return "", ErrInvalidTokenClaims
	}

	claims, err := ValidateToken[T](ctx, token, jwtSecret)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidTokenClaims, err)
	}

	return claims.Identity, nil
}
