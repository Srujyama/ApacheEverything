// Package web embeds the built frontend (apps/web/dist) into the server binary.
//
// The build pipeline copies apps/web/dist/ into apps/server/internal/web/dist/
// before `go build` runs. A placeholder dist/index.html is checked in so the
// package compiles before the first frontend build.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

//go:embed all:dist
var distFS embed.FS

// Mount registers the SPA handler on the given router.
// Any non-API request that doesn't match a static asset falls back to index.html
// so client-side routing (react-router) keeps working.
func Mount(r chi.Router) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			serveIndex(w, req, sub)
			return
		}
		if _, err := fs.Stat(sub, path); err == nil {
			fileServer.ServeHTTP(w, req)
			return
		}
		serveIndex(w, req, sub)
	})
}

func serveIndex(w http.ResponseWriter, req *http.Request, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "frontend not built — run `task build:web` first", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
	_ = req
}
