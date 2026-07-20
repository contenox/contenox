package serverapi

// End-to-end gate tests that mirror how `contenox serve` wires the HTTP surface
// (see runtime/contenoxcli/serve_cmd.go): a rootMux with open /health and /ui/*
// routes plus an /api/ subtree wrapped by ProtectAPI over a StripPrefix'd apiMux.
// These are the exact regression tests for the same-origin-read vulnerability:
// with a TOKEN set, an uncredentialed GET to any /api/* route must 401.
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libauth"
)

// expiredSessionJWT mints a session JWT for token that is already expired, to
// prove the gate rejects expired cookies.
func expiredSessionJWT(t *testing.T, token string) string {
	t.Helper()
	cfg := libauth.CreateTokenArgs{JWTSecret: deriveSessionSecret(token), JWTExpiry: -time.Hour}
	s, _, err := libauth.CreateToken(cfg, localOperatorIdentity, localOperatorAuthz{})
	if err != nil {
		t.Fatalf("mint expired: %v", err)
	}
	return s
}

// serveLikeMux builds a router shaped like serve's rootMux for the given token.
// The apiMux registers a few representative GET routes (/state, /backends,
// /mcp-servers) that echo 200 so the test observes whether the gate let the
// request reach them.
func serveLikeMux(token string) http.Handler {
	apiMux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	apiMux.HandleFunc("GET /state", ok)
	apiMux.HandleFunc("GET /backends", ok)
	apiMux.HandleFunc("GET /mcp-servers", ok)

	rootMux := http.NewServeMux()
	AddHealthRoutes(rootMux)
	AddUIAuthRoutes(rootMux, token)
	rootMux.Handle("/api/", http.StripPrefix("/api", ProtectAPI(token, "", apiMux)))
	return rootMux
}

// cookieJWTFor logs in through the real /ui/login handler and returns the
// session-cookie JWT, exactly as a browser would obtain it.
func cookieJWTFor(t *testing.T, token string) *http.Cookie {
	t.Helper()
	return sessionCookieFromLogin(t, token)
}

var protectedGETs = []string{"/api/state", "/api/backends", "/api/mcp-servers"}

// TestServe_TokenSet_UncredentialedGETsAre401 is THE regression test for the
// reported hole: every /api/* GET with no credential must be rejected 401.
func TestServe_TokenSet_UncredentialedGETsAre401(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)

	for _, path := range protectedGETs {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s with no credential: status = %d, want 401", path, resp.StatusCode)
		}
	}
}

// TestServe_TokenSet_CookieJWTGetsThrough proves a logged-in browser's session
// cookie authorizes the same reads.
func TestServe_TokenSet_CookieJWTGetsThrough(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)
	cookie := cookieJWTFor(t, testToken)

	for _, path := range protectedGETs {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.AddCookie(cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("GET %s with cookie JWT: got 401, want non-401", path)
		}
	}
}

// TestServe_TokenSet_RawBearerGetsThrough proves programmatic clients keep
// working with the raw token as a bearer.
func TestServe_TokenSet_RawBearerGetsThrough(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)

	for _, path := range protectedGETs {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("GET %s with raw bearer: got 401, want non-401", path)
		}
	}
}

// TestServe_TokenSet_TamperedCookieIs401 proves a forged/tampered session cookie
// is rejected — only a JWT signed with the TOKEN-derived secret passes.
func TestServe_TokenSet_TamperedCookieIs401(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)
	good := cookieJWTFor(t, testToken)

	// Flip a SIGNIFICANT bit of the (base64url) signature's final character. In
	// the final quantum only the top 4 of its 6 bits are signature bits and the
	// encoder emits zero padding bits, so 'A'→'B' would change padding bits only
	// and still verify; 'A'↔'Q' always flips a real signature bit.
	tampered := good.Value
	if len(tampered) == 0 {
		t.Fatal("empty cookie JWT")
	}
	b := []byte(tampered)
	if b[len(b)-1] == 'A' {
		b[len(b)-1] = 'Q'
	} else {
		b[len(b)-1] = 'A'
	}
	tampered = string(b)

	for _, path := range protectedGETs {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tampered})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s with tampered cookie: status = %d, want 401", path, resp.StatusCode)
		}
	}
}

// TestServe_TokenSet_WrongSecretJWTIs401 proves a JWT minted with a DIFFERENT
// token (well-formed but signed with the wrong derived secret) is rejected.
func TestServe_TokenSet_WrongSecretJWTIs401(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)

	otherJWT, _, err := mintSessionToken("a-completely-different-token")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/state", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: otherJWT})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-secret JWT: status = %d, want 401", resp.StatusCode)
	}
}

// TestServe_TokenSet_ExpiredJWTIs401 proves an expired session JWT is rejected.
func TestServe_TokenSet_ExpiredJWTIs401(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)

	expired := expiredSessionJWT(t, testToken)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/state", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: expired})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired JWT: status = %d, want 401", resp.StatusCode)
	}
}

// TestServe_TokenSet_OpenRoutesNeedNoCredential proves health and the auth
// endpoints stay reachable without any credential so the login page can work.
func TestServe_TokenSet_OpenRoutesNeedNoCredential(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(testToken))
	t.Cleanup(srv.Close)

	cases := []struct {
		method, path string
		body         string
	}{
		{http.MethodGet, "/health", ""},
		{http.MethodGet, "/ui/auth-status", ""},
		// Correct token so login returns 200, proving the route is reachable with
		// no pre-existing credential (a wrong token would 401 from login itself,
		// which is the endpoint's own response, not the API gate).
		{http.MethodPost, "/ui/login", `{"token":"` + testToken + `"}`},
	}
	for _, c := range cases {
		var req *http.Request
		if c.body != "" {
			req, _ = http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(c.body))
		} else {
			req, _ = http.NewRequest(c.method, srv.URL+c.path, nil)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("%s %s must be reachable without a credential, got 401", c.method, c.path)
		}
	}
}

// TestServe_NoToken_LoopbackGETsPassWithoutCredential proves the zero-friction
// local-dev path is not regressed: with no TOKEN, reads pass with no credential.
func TestServe_NoToken_LoopbackGETsPassWithoutCredential(t *testing.T) {
	srv := httptest.NewServer(serveLikeMux(""))
	t.Cleanup(srv.Close)

	for _, path := range protectedGETs {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("no-token GET %s: status = %d, want 200", path, resp.StatusCode)
		}
	}
}
