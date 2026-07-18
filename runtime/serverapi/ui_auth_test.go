package serverapi

// These tests exercise the Beam remote-access login endpoints and the session
// cookie's acceptance across the mutation-protection middleware. They hit the
// handlers directly (no real product routes required) and assert the cookie is
// interchangeable with the bearer token everywhere the bearer is honored.
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "s3cr3t-shared-token"

func decodeAuthStatus(t *testing.T, body []byte) authStatusResponse {
	t.Helper()
	var out authStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode auth status: %v (body=%s)", err, body)
	}
	return out
}

// sessionCookieFromLogin performs a successful login and returns the session
// cookie the server set, so downstream requests can present it.
func sessionCookieFromLogin(t *testing.T, token string) *http.Cookie {
	t.Helper()
	h := uiLoginHandler(token, newLoginLimiter(defaultLoginMaxAttempts, defaultLoginWindow))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader(`{"token":"`+token+`"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("login response set no %q cookie", sessionCookieName)
	return nil
}

func TestUILogin_CorrectTokenSetsHttpOnlySessionCookie(t *testing.T) {
	c := sessionCookieFromLogin(t, testToken)
	// The cookie must carry a signed session JWT, NOT the raw shared secret.
	if c.Value == testToken {
		t.Fatal("session cookie must not store the raw TOKEN verbatim")
	}
	if !validateSessionToken(testToken, c.Value) {
		t.Fatalf("session cookie must be a valid session JWT, got %q", c.Value)
	}
	if !c.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie SameSite: want Strict, got %v", c.SameSite)
	}
	if c.Path != "/" {
		t.Fatalf("session cookie Path: want /, got %q", c.Path)
	}
	// Over plain HTTP (httptest is non-TLS) Secure must be off, or the browser
	// would drop the cookie and login would silently fail.
	if c.Secure {
		t.Fatal("session cookie must not be Secure over plain HTTP")
	}
}

func TestUILogin_SecureCookieUnderForwardedHTTPS(t *testing.T) {
	h := uiLoginHandler(testToken, newLoginLimiter(defaultLoginMaxAttempts, defaultLoginWindow))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader(`{"token":"`+testToken+`"}`))
	req.Header.Set("X-Forwarded-Proto", "https")
	h.ServeHTTP(rr, req)
	var got *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName {
			got = c
		}
	}
	if got == nil {
		t.Fatal("no session cookie set")
	}
	if !got.Secure {
		t.Fatal("session cookie must be Secure when X-Forwarded-Proto=https")
	}
}

func TestUILogin_WrongTokenIsUnauthorizedAndSetsNoCookie(t *testing.T) {
	h := uiLoginHandler(testToken, newLoginLimiter(defaultLoginMaxAttempts, defaultLoginWindow))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader(`{"token":"wrong"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" && c.MaxAge >= 0 {
			t.Fatal("wrong token must not set a live session cookie")
		}
	}
	got := decodeAuthStatus(t, rr.Body.Bytes())
	if got.Authenticated {
		t.Fatal("wrong token must report authenticated=false")
	}
}

func TestUILogin_RateLimitReturns429(t *testing.T) {
	// A tiny window/budget so exhausting it is deterministic without sleeps.
	limiter := newLoginLimiter(2, time.Minute)
	h := uiLoginHandler(testToken, limiter)
	do := func() int {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader(`{"token":"wrong"}`))
		req.RemoteAddr = "203.0.113.7:44444"
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if code := do(); code != http.StatusUnauthorized {
		t.Fatalf("attempt 1: want 401, got %d", code)
	}
	if code := do(); code != http.StatusUnauthorized {
		t.Fatalf("attempt 2: want 401, got %d", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("attempt 3: want 429, got %d", code)
	}
}

func TestUILimiter_SeparateIPsHaveSeparateBudgets(t *testing.T) {
	limiter := newLoginLimiter(1, time.Minute)
	if !limiter.allow("1.1.1.1") {
		t.Fatal("first attempt from IP A should be allowed")
	}
	if limiter.allow("1.1.1.1") {
		t.Fatal("second attempt from IP A should be throttled")
	}
	if !limiter.allow("2.2.2.2") {
		t.Fatal("IP B must have its own budget")
	}
}

func TestUILimiter_WindowResets(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newLoginLimiter(1, time.Minute)
	limiter.now = func() time.Time { return now }
	if !limiter.allow("x") {
		t.Fatal("first allowed")
	}
	if limiter.allow("x") {
		t.Fatal("second throttled within window")
	}
	now = now.Add(2 * time.Minute)
	if !limiter.allow("x") {
		t.Fatal("allowed again after window elapsed")
	}
}

func TestUIAuthStatus_NoTokenConfigured(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/auth-status", nil)
	uiAuthStatusHandler("").ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth-status must always be readable; got %d", rr.Code)
	}
	got := decodeAuthStatus(t, rr.Body.Bytes())
	if got.Required || !got.Authenticated {
		t.Fatalf("no token: want {required:false, authenticated:true}, got %+v", got)
	}
}

func TestUIAuthStatus_TokenConfigured(t *testing.T) {
	h := uiAuthStatusHandler(testToken)

	// Anonymous: required, not authenticated.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui/auth-status", nil))
	got := decodeAuthStatus(t, rr.Body.Bytes())
	if !got.Required || got.Authenticated {
		t.Fatalf("anon: want {required:true, authenticated:false}, got %+v", got)
	}

	// With the session cookie: authenticated.
	cookie := sessionCookieFromLogin(t, testToken)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/auth-status", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rr, req)
	got = decodeAuthStatus(t, rr.Body.Bytes())
	if !got.Required || !got.Authenticated {
		t.Fatalf("cookie: want {required:true, authenticated:true}, got %+v", got)
	}

	// With the bearer token: also authenticated (programmatic path unchanged).
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/auth-status", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(rr, req)
	got = decodeAuthStatus(t, rr.Body.Bytes())
	if !got.Authenticated {
		t.Fatal("bearer token must satisfy auth-status")
	}
}

func TestUILogout_ClearsCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/logout", nil)
	uiLogoutHandler(testToken).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", rr.Code)
	}
	var cleared bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout must expire the session cookie")
	}
}

// The session cookie must satisfy the mutation-protection middleware exactly as
// a bearer token does: a cookie'd mutation passes, an anonymous one 401s.
func TestProtectMutatingAPI_SessionCookieAuthorizesMutation(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI(testToken, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	cookie := sessionCookieFromLogin(t, testToken)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backends", strings.NewReader("{}"))
	req.AddCookie(cookie)
	handler.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("valid session cookie must authorize a mutation: called=%v code=%d", called, rr.Code)
	}

	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/backends", strings.NewReader("{}"))
	handler.ServeHTTP(rr, req)
	if called || rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing cookie must 401 a mutation: called=%v code=%d", called, rr.Code)
	}

	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/backends", strings.NewReader("{}"))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-the-token"})
	handler.ServeHTTP(rr, req)
	if called || rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong cookie must 401 a mutation: called=%v code=%d", called, rr.Code)
	}
}

// Loopback with no TOKEN: login is not required, and mutations pass without any
// credential (zero prompts for local dev).
func TestUILogin_NoTokenLoopbackNeedsNoCredential(t *testing.T) {
	rr := httptest.NewRecorder()
	uiLoginHandler("", newLoginLimiter(defaultLoginMaxAttempts, defaultLoginWindow)).
		ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader("{}")))
	if rr.Code != http.StatusOK {
		t.Fatalf("no-token login: want 200, got %d", rr.Code)
	}
	got := decodeAuthStatus(t, rr.Body.Bytes())
	if got.Required {
		t.Fatal("no token configured must report required=false")
	}

	called := false
	handler := ProtectMutatingAPI("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	mr := httptest.NewRecorder()
	// Same-origin mutation (no Origin header, not cross-site) passes with no token.
	handler.ServeHTTP(mr, httptest.NewRequest(http.MethodPost, "/api/backends", strings.NewReader("{}")))
	if !called {
		t.Fatal("no-token same-origin mutation must pass without a credential")
	}
}
