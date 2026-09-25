package contenoxcli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestUnit_GatewayOwnership_CanceledRequestDoesNotLoseLease(t *testing.T) {
	owner, err := acquireGatewayOwnership(context.Background(), filepath.Join(t.TempDir(), "gateway.lease"), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.release()
	called := false
	handler := owner.guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/version", nil).WithContext(ctx))
	if !called || response.Code != http.StatusNoContent || owner.ctx.Err() != nil {
		t.Fatalf("request cancellation cost gateway ownership: called=%t status=%d owner_err=%v", called, response.Code, owner.ctx.Err())
	}
}
