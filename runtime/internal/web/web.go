// Package web serves the embedded Beam React SPA from the contenox binary.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

//go:embed all:beam/dist
var beamFiles embed.FS

// BeamFS returns the embedded Beam dist as an io/fs.FS rooted at the dist dir.
func BeamFS() fs.FS {
	sub, err := fs.Sub(beamFiles, "beam/dist")
	if err != nil {
		panic("web: failed to sub-FS beam/dist: " + err.Error())
	}
	return sub
}

// SPAHandler serves Beam with an index.html fallback for client-side routes.
func SPAHandler() http.Handler {
	beamFS := BeamFS()
	fileServer := http.FileServer(http.FS(beamFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := beamFS.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// DevProxyHandler proxies Beam UI requests to a Vite dev server while the Go
// server remains the browser origin for /api and auth cookies.
func DevProxyHandler(target string) (http.Handler, error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return nil, fmt.Errorf("target URL is required")
	}
	targetURL, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse target URL: %w", err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return nil, fmt.Errorf("target URL must use http or https")
	}
	if targetURL.Host == "" {
		return nil, fmt.Errorf("target URL must include a host")
	}

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "beam dev proxy unavailable", http.StatusBadGateway)
		},
	}, nil
}
