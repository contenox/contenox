package serverapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"strings"
	"time"

	"github.com/contenox/runtime/libauth"
	"golang.org/x/crypto/hkdf"
)

// Single-operator session auth for `contenox serve`.
//
// OSS serve has no user database: there is exactly one credential, the
// configured shared TOKEN. Rather than store that raw secret in the browser
// session cookie, /ui/login mints a short-lived signed JWT (via libauth) that
// emulates a fixed admin principal ("local-operator"). Browser requests then authenticate by presenting that
// cookie JWT; programmatic clients keep presenting the raw TOKEN as a Bearer /
// X-API-Key header (or the /acp ?token= query param). AuthenticateCredential
// accepts EITHER form, so both paths share one gate.
//
// Secret derivation: the JWT signing secret is derived from the configured
// TOKEN via HKDF-SHA256 (no separate secret to configure). This keeps issued
// cookies valid across the life of a process without any persisted key, and —
// because the salt/info are fixed and the TOKEN is stable — a restart with the
// SAME token continues to accept previously-issued, unexpired cookies. Rotating
// the TOKEN changes the derived secret, which correctly invalidates every old
// cookie. The derived secret never equals the raw TOKEN, so a JWT can never be
// confused with the raw bearer credential in AuthenticateCredential.
const (
	// localOperatorIdentity is the fixed principal every serve session JWT is
	// minted for. Serve is single-operator, so there is no user lookup — the
	// subject is a constant admin identity.
	localOperatorIdentity = "local-operator"

	// sessionTokenTTL bounds how long a minted cookie JWT stays valid. A logged-in
	// browser re-authenticates (via the stored TOKEN on the login page) at most
	// once per day.
	sessionTokenTTL = 24 * time.Hour

	// sessionSecretSalt / sessionSecretInfo domain-separate the HKDF derivation so
	// the session-signing secret can never collide with any other key derived from
	// the same TOKEN elsewhere.
	sessionSecretSalt = "contenox-serve-session-v1"
	sessionSecretInfo = "session-jwt-hs256"
)

// localOperatorAuthz is the libauth.Authz payload carried in serve session JWTs.
// Serve has a single admin operator, so authorisation is unconditionally granted
// once the JWT itself validates (the JWT signature is the actual gate).
type localOperatorAuthz struct{}

// RequireAuthorisation implements libauth.Authz: the local operator may do
// everything.
func (localOperatorAuthz) RequireAuthorisation(string, int) (bool, error) {
	return true, nil
}

// deriveSessionSecret turns the configured TOKEN into the HS256 signing secret
// used for session JWTs. HKDF-SHA256 with a fixed salt/info yields a stable,
// high-entropy key that is a deterministic function of the TOKEN — no separate
// secret config, and same-token restarts keep existing cookies valid.
func deriveSessionSecret(token string) string {
	r := hkdf.New(sha256.New, []byte(token), []byte(sessionSecretSalt), []byte(sessionSecretInfo))
	out := make([]byte, 32)
	if _, err := io.ReadFull(r, out); err != nil {
		// io.ReadFull over hkdf.New only fails if asked for more bytes than the
		// hash can supply (SHA-256 supplies up to 255*32 bytes); 32 never fails.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(out)
}

// mintSessionToken issues a signed, short-lived session JWT for the fixed local
// operator, to be stored in the HttpOnly session cookie by /ui/login. token is
// the configured shared secret; it must be non-empty (login is a no-op without
// a configured TOKEN).
func mintSessionToken(token string) (string, time.Time, error) {
	cfg := libauth.CreateTokenArgs{
		JWTSecret: deriveSessionSecret(token),
		JWTExpiry: sessionTokenTTL,
	}
	return libauth.CreateToken(cfg, localOperatorIdentity, localOperatorAuthz{})
}

// validateSessionToken reports whether jwtStr is a valid, unexpired session JWT
// signed with the secret derived from token. A tampered signature, wrong
// signing key (different TOKEN), expired token, or malformed string all yield
// false.
func validateSessionToken(token, jwtStr string) bool {
	jwtStr = strings.TrimSpace(jwtStr)
	if token == "" || jwtStr == "" {
		return false
	}
	_, err := libauth.ValidateToken[localOperatorAuthz](context.Background(), jwtStr, deriveSessionSecret(token))
	return err == nil
}

// AuthenticateCredential reports whether cred authenticates against the
// configured token. It accepts either the raw TOKEN itself (compared in
// constant time — the programmatic Bearer / X-API-Key / ?token= path) or a
// valid session JWT minted by /ui/login (the browser cookie path). Both the API
// protection middleware and the /acp WebSocket upgrade use this single gate, so
// one login satisfies every serve surface. Returns false when no token is
// configured (callers gate on that separately) or cred is empty.
func AuthenticateCredential(token, cred string) bool {
	token = strings.TrimSpace(token)
	cred = strings.TrimSpace(cred)
	if token == "" || cred == "" {
		return false
	}
	// Raw shared-secret path: constant-time so a wrong bearer leaks no timing
	// signal about how many leading bytes matched.
	if subtle.ConstantTimeCompare([]byte(cred), []byte(token)) == 1 {
		return true
	}
	// Browser cookie path: a signed session JWT.
	return validateSessionToken(token, cred)
}
