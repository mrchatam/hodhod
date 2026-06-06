package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/app.css
var staticFS embed.FS

// cssAssetVersion busts browser cache after CSS rebuilds (hash prefix of embedded app.css).
var cssAssetVersion string

func init() {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil || len(b) == 0 {
		return
	}
	sum := sha256.Sum256(b)
	cssAssetVersion = hex.EncodeToString(sum[:6])
}

func CSSAssetVersion() string { return cssAssetVersion }

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
