package serverapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/version"
)

func TestHealthRoute(t *testing.T) {
	mux := http.NewServeMux()
	AddHealthRoutes(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("status body = %q, want ok", got.Status)
	}
}

func TestVersionRoute(t *testing.T) {
	mux := http.NewServeMux()
	AddVersionRoutes(mux, "v-test", "node-1", "local")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/version", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got apiframework.AboutServer
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Version != "v-test" || got.NodeInstanceID != "node-1" || got.Tenancy != "local" {
		t.Fatalf("body = %+v", got)
	}
}

func TestNewRegistersFoundationRoutes(t *testing.T) {
	mux := http.NewServeMux()
	cleanup, err := New(context.Background(), mux, "node-1", "local", &Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	health := httptest.NewRecorder()
	mux.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	versionResp := httptest.NewRecorder()
	mux.ServeHTTP(versionResp, httptest.NewRequest(http.MethodGet, "/version", nil))
	if versionResp.Code != http.StatusOK {
		t.Fatalf("version status = %d, want %d", versionResp.Code, http.StatusOK)
	}
	var about apiframework.AboutServer
	if err := json.NewDecoder(versionResp.Body).Decode(&about); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if about.Version != version.Get() || about.NodeInstanceID != "node-1" || about.Tenancy != "local" {
		t.Fatalf("about = %+v", about)
	}

	notFound := httptest.NewRecorder()
	mux.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("root status = %d, want %d", notFound.Code, http.StatusNotFound)
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(notFound.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode not found: %v", err)
	}
	if errBody.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want not_found", errBody.Error.Code)
	}
}

func TestUnit_ValidateLocalServeSecurity(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "::1", "localhost", ""} {
		if err := ValidateLocalServeSecurity(addr, ""); err != nil {
			t.Fatalf("addr %q should be allowed without token: %v", addr, err)
		}
	}
	if err := ValidateLocalServeSecurity("0.0.0.0", ""); err == nil {
		t.Fatal("non-loopback ADDR without TOKEN must fail")
	}
	if err := ValidateLocalServeSecurity("0.0.0.0", "secret"); err != nil {
		t.Fatalf("non-loopback ADDR with TOKEN should be allowed: %v", err)
	}
}

func TestUnit_ProtectMutatingAPI(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	protected := ProtectMutatingAPI("secret", next)

	get := httptest.NewRecorder()
	protected.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/providers/configs", nil))
	if get.Code != http.StatusNoContent {
		t.Fatalf("GET status = %d, want %d", get.Code, http.StatusNoContent)
	}

	postNoToken := httptest.NewRecorder()
	protected.ServeHTTP(postNoToken, httptest.NewRequest(http.MethodPost, "/api/providers/openai/configure", nil))
	if postNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("POST without token status = %d, want %d", postNoToken.Code, http.StatusUnauthorized)
	}

	postBearer := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers/openai/configure", nil)
	req.Header.Set("Authorization", "Bearer secret")
	protected.ServeHTTP(postBearer, req)
	if postBearer.Code != http.StatusNoContent {
		t.Fatalf("POST with bearer status = %d, want %d", postBearer.Code, http.StatusNoContent)
	}
}
