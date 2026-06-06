package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/app.css static/app.js
var staticFS embed.FS

// cssAssetVersion busts browser cache after static asset rebuilds.
var cssAssetVersion string

func init() {
	h := sha256.New()
	for _, name := range []string{"static/app.css", "static/app.js"} {
		b, err := staticFS.ReadFile(name)
		if err != nil || len(b) == 0 {
			continue
		}
		h.Write(b)
	}
	sum := h.Sum(nil)
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
	switch {
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
