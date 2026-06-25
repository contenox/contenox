package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandler_ServesIndexAndDeepLinks(t *testing.T) {
	handler := SPAHandler()

	for _, path := range []string{"/", "/chat/some-session"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "<!doctype html>") {
			t.Fatalf("%s did not return index.html", path)
		}
	}
}
