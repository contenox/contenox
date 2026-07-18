package serverapi

// These tests exercise ProtectMutatingAPI(WithAllowedOrigins) directly against
// a bare stub handler — the request never reaches the real mux — so the path
// below is only a stand-in for "some mutating product route" and carries no
// dependency on any specific handler being registered. It uses /api/backends
// (a route that both accepts POST and survives; see backendapi) rather than
// the retired /api/chats, which internalchatapi (deleted, Stage 6 of the beam
// ACP unification) used to serve.
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProtectMutatingAPI_NoTokenRejectsCrossOriginBrowserMutation(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for cross-origin mutation")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestProtectMutatingAPI_NoTokenAllowsSameOriginBrowserMutation(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "http://127.0.0.1:32123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler was not called for same-origin mutation")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}

func TestProtectMutatingAPI_NoTokenAllowsNonBrowserMutation(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler was not called for request without browser cross-site headers")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}

func TestProtectMutatingAPI_NoTokenRejectsSecFetchCrossSiteMutation(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for Sec-Fetch-Site cross-site mutation")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestProtectMutatingAPIWithAllowedOrigins_NoTokenAllowsExplicitOrigin(t *testing.T) {
	called := false
	handler := ProtectMutatingAPIWithAllowedOrigins("", "http://127.0.0.1:5173", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler was not called for explicitly allowed origin")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}

func TestProtectMutatingAPIWithAllowedOrigins_NoTokenWildcardDoesNotBypassMutationGuard(t *testing.T) {
	called := false
	handler := ProtectMutatingAPIWithAllowedOrigins("", "*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for wildcard-only cross-origin mutation")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestProtectMutatingAPI_TokenStillRequiredWhenConfigured(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "http://127.0.0.1:32123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called without required token")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32123/api/backends", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler was not called with valid token")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}

func TestProtectMutatingAPI_TokenRejectsAllowedCrossOriginBrowserReadWithoutToken(t *testing.T) {
	called := false
	handler := ProtectMutatingAPIWithAllowedOrigins("secret", "https://evil.com", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32123/api/backends", nil)
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for cross-origin browser read without token")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rr.Code, rr.Body.String())
	}
}

func TestProtectMutatingAPI_TokenRejectsSecFetchCrossSiteReadWithoutToken(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32123/api/backends", nil)
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for Sec-Fetch-Site cross-site read without token")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rr.Code, rr.Body.String())
	}
}

// SECURITY REGRESSION: a same-origin browser GET with NO credential used to be
// served (the mutation-only gate never checked reads). With a TOKEN configured,
// every read must now require a credential — same-origin is not enough.
func TestProtectMutatingAPI_TokenRejectsSameOriginBrowserReadWithoutToken(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32123/api/backends", nil)
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "http://127.0.0.1:32123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for same-origin browser read without credential")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

// SECURITY REGRESSION: a non-browser GET with no credential (e.g. a LAN attacker's
// curl) used to be served. With a TOKEN configured it must now 401.
func TestProtectMutatingAPI_TokenRejectsNonBrowserReadWithoutToken(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32123/api/backends", nil)
	req.Host = "127.0.0.1:32123"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("handler was called for non-browser read without credential")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestProtectMutatingAPI_TokenAllowsCrossOriginBrowserReadWithToken(t *testing.T) {
	called := false
	handler := ProtectMutatingAPI("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32123/api/backends", nil)
	req.Host = "127.0.0.1:32123"
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("handler was not called for tokened cross-origin browser read")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}
