package web

import (
	"net/http"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
)

func (s *Server) pageAccountPassword(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "account_password", r, nil)
}

func (s *Server) postAccountPassword(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	if lang == "" {
		lang = "fa"
	}
	_ = r.ParseForm()
	current := r.FormValue("current_password")
	newPW := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")
	fresh, err := s.Store.GetAdmin(r.Context(), admin.ID)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	if !CheckPassword(fresh.PasswordHash, current) {
		s.setFlash(w, "err", i18n.Admin(lang, "account.password_wrong"))
		http.Redirect(w, r, "/account/password", http.StatusSeeOther)
		return
	}
	if len(newPW) < 8 {
		s.setFlash(w, "err", i18n.Admin(lang, "account.password_short"))
		http.Redirect(w, r, "/account/password", http.StatusSeeOther)
		return
	}
	if newPW != confirm {
		s.setFlash(w, "err", i18n.Admin(lang, "account.password_mismatch"))
		http.Redirect(w, r, "/account/password", http.StatusSeeOther)
		return
	}
	hash, err := HashPassword(newPW)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	fresh.PasswordHash = hash
	if err := s.Store.UpdateAdmin(r.Context(), fresh); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	s.audit(r, &admin.ID, "change_password", "admin", admin.ID, nil)
	c, _ := r.Cookie(SessionCookieName)
	keep := ""
	if c != nil {
		keep = c.Value
	}
	_ = s.Store.DeleteSessionsByAdminExcept(r.Context(), admin.ID, keep)
	s.setFlash(w, "ok", i18n.Admin(lang, "account.password_changed"))
	http.Redirect(w, r, "/account/password", http.StatusSeeOther)
}

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
