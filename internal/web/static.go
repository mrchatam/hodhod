package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/app.css
var staticFS embed.FS

func (s *Server) staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

func staticContentType(path string) string {
	if strings.HasSuffix(path, ".css") {
		return "text/css; charset=utf-8"
	}
	return "application/octet-stream"
}
