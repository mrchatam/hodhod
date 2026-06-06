package web

import (
	"net/http"

	"github.com/mrchatam/hodhod/internal/db"
)

func (s *Server) postAccountLang(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	_ = r.ParseForm()
	lang := r.FormValue("lang")
	if lang != "fa" && lang != "en" {
		lang = "fa"
	}
	_ = s.Store.SetSetting(r.Context(), "admin", admin.ID, "lang", lang)
	http.SetCookie(w, &http.Cookie{Name: "hodhod_lang", Value: lang, Path: "/", MaxAge: 86400 * 365})
	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

func (s *Server) postAccountTheme(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	_ = r.ParseForm()
	theme := r.FormValue("theme")
	switch theme {
	case "light", "dark", "system":
	default:
		theme = "system"
	}
	_ = s.Store.SetSetting(r.Context(), "admin", admin.ID, "theme", theme)
	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
