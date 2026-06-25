package web

import (
	"io"
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

func TestDevProxyHandler_ForwardsRequestsAndCookies(t *testing.T) {
	var gotPath, gotHost, gotForwardedHost, gotForwardedProto, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotHost = r.Host
		gotForwardedHost = r.Header.Get("X-Forwarded-Host")
		gotForwardedProto = r.Header.Get("X-Forwarded-Proto")
		gotCookie = r.Header.Get("Cookie")
		http.SetCookie(w, &http.Cookie{Name: "beam", Value: "ok", Path: "/"})
		w.Header().Set("X-Upstream", "vite")
		_, _ = io.WriteString(w, "proxied")
	}))
	defer upstream.Close()

	handler, err := DevProxyHandler(upstream.URL)
	if err != nil {
		t.Fatalf("DevProxyHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://contenox.test/chat/session?x=1", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "123"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "proxied" {
		t.Fatalf("body = %q", rr.Body.String())
	}
	if gotPath != "/chat/session?x=1" {
		t.Fatalf("proxied path = %q", gotPath)
	}
	if gotHost != "contenox.test" {
		t.Fatalf("proxied host = %q", gotHost)
	}
	if gotForwardedHost != "contenox.test" {
		t.Fatalf("X-Forwarded-Host = %q", gotForwardedHost)
	}
	if gotForwardedProto != "http" {
		t.Fatalf("X-Forwarded-Proto = %q", gotForwardedProto)
	}
	if gotCookie != "sid=123" {
		t.Fatalf("Cookie = %q", gotCookie)
	}
	if rr.Header().Get("X-Upstream") != "vite" {
		t.Fatalf("X-Upstream = %q", rr.Header().Get("X-Upstream"))
	}
	if got := rr.Header().Values("Set-Cookie"); len(got) != 1 || !strings.Contains(got[0], "beam=ok") {
		t.Fatalf("Set-Cookie = %#v", got)
	}
}

func TestDevProxyHandler_RejectsInvalidTarget(t *testing.T) {
	for _, target := range []string{"", "localhost:5173", "ftp://localhost:5173", "http://"} {
		if _, err := DevProxyHandler(target); err == nil {
			t.Fatalf("DevProxyHandler(%q) returned nil error", target)
		}
	}
}
