package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnableCORS_DefaultDoesNotAllowArbitraryOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := EnableCORS(nil, next)

	req := httptest.NewRequest(http.MethodOptions, "/api/chats", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := rr.Header().Values("Vary"); len(got) == 0 {
		t.Fatal("expected Vary: Origin for origin-bearing request")
	}
}

func TestEnableCORS_ExplicitWildcardStillWorks(t *testing.T) {
	handler := EnableCORS(&CORSConfig{
		AllowedAPIOrigins: "*",
		AllowedMethods:    DefaultAllowedMethods,
		AllowedHeaders:    DefaultAllowedHeaders,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/chats", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestEnableCORS_ExplicitOriginAllowsOnlyThatOrigin(t *testing.T) {
	handler := EnableCORS(&CORSConfig{
		AllowedAPIOrigins: "http://127.0.0.1:5173",
		AllowedMethods:    DefaultAllowedMethods,
		AllowedHeaders:    DefaultAllowedHeaders,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRequest(http.MethodOptions, "/api/chats", nil)
	allowed.Header.Set("Origin", "http://127.0.0.1:5173")
	allowedRR := httptest.NewRecorder()
	handler.ServeHTTP(allowedRR, allowed)
	if got := allowedRR.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allowed origin header = %q", got)
	}

	blocked := httptest.NewRequest(http.MethodOptions, "/api/chats", nil)
	blocked.Header.Set("Origin", "https://evil.com")
	blockedRR := httptest.NewRecorder()
	handler.ServeHTTP(blockedRR, blocked)
	if got := blockedRR.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("blocked origin header = %q, want empty", got)
	}
}
