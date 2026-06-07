// Package web serves the embedded Beam React SPA from the contenox binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
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
