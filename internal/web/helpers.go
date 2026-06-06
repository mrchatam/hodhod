package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
	"github.com/mrchatam/hodhod/internal/sales"
)

func (s *Server) baseData(r *http.Request) map[string]any {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	perms, _ := s.permsFor(r, admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	if lang == "" {
		lang = "fa"
	}
	theme, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "theme")
	if theme == "" {
		theme = "system"
	}
	path := r.URL.Path
	return map[string]any{
		"Admin":       admin,
		"CSRF":        r.Context().Value(ctxCSRF),
		"Perms":       perms,
		"Flash":       s.popFlash(r),
		"IsMaster":    admin.Role == db.RoleMaster,
		"Lang":        lang,
		"IsRTL":       lang == "fa",
		"Theme":       theme,
		"CurrentPath": path,
	}
}

func (s *Server) renderPage(w http.ResponseWriter, page string, r *http.Request, extra map[string]any) {
	data := s.baseData(r)
	for k, v := range extra {
		data[k] = v
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "unknown page", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, page, partial string, data map[string]any) {
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "unknown page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, partial, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func panelTestMessage(lang string, err error) (ok bool, msg string) {
	if err == nil {
		return true, i18n.Admin(lang, "panels.test_ok")
	}
	return false, i18n.Admin(lang, "panels.test_fail")
}

func (s *Server) permsFor(r *http.Request, admin *db.Admin) (*db.AgentPermissions, error) {
	if admin.Role == db.RoleMaster {
		return &db.AgentPermissions{CreateUser: true, ModifyUser: true, AddTime: true, AddVolume: true,
			ResetUsage: true, DisableEnable: true, DeleteUser: true, ManageBot: true, ManagePlans: true}, nil
	}
	if admin.AgentID == nil {
		return &db.AgentPermissions{ViewOnly: true}, nil
	}
	return s.Store.GetAgentPermissions(r.Context(), *admin.AgentID)
}

func (s *Server) agentID(admin *db.Admin) (int64, bool) {
	if admin.Role == db.RoleMaster {
		return 0, false
	}
	if admin.AgentID == nil {
		return 0, false
	}
	return *admin.AgentID, true
}

func (s *Server) setFlash(w http.ResponseWriter, kind, msg string) {
	http.SetCookie(w, &http.Cookie{Name: "hodhod_flash", Value: kind + ":" + msg, Path: "/", MaxAge: 60})
}

func (s *Server) saveFlash(w http.ResponseWriter, err error, okMsg string) {
	if err != nil {
		slog.Error("save failed", "err", err)
		s.setFlash(w, "err", friendlySaveErr(err))
		return
	}
	s.setFlash(w, "ok", okMsg)
}

func friendlySaveErr(err error) string {
	if err == nil {
		return "Save failed"
	}
	msg := err.Error()
	if strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") {
		return "Save failed — duplicate value (check custom domain or username)"
	}
	return "Save failed — please try again or check server logs"
}

func (s *Server) popFlash(r *http.Request) map[string]string {
	c, err := r.Cookie("hodhod_flash")
	if err != nil {
		return nil
	}
	parts := splitOnce(c.Value, ":")
	if len(parts) != 2 {
		return nil
	}
	return map[string]string{"Kind": parts[0], "Msg": parts[1]}
}

func splitOnce(s, sep string) []string {
	i := indexOf(s, sep)
	if i < 0 {
		return []string{s}
	}
	return []string{s[:i], s[i+len(sep):]}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (s *Server) canPerm(r *http.Request, admin *db.Admin, perm db.Perm) bool {
	if admin.Role == db.RoleMaster {
		return true
	}
	p, err := s.permsFor(r, admin)
	if err != nil {
		return false
	}
	if p.ViewOnly && perm != "" {
		return false
	}
	return perm == "" || p.Has(perm)
}

func friendlySalesErr(lang string, err error) string {
	if err == nil {
		return i18n.Admin(lang, "flash.error")
	}
	switch {
	case errors.Is(err, sales.ErrViewOnly):
		return i18n.Admin(lang, "err.view_only")
	case errors.Is(err, sales.ErrPermDenied):
		return i18n.Admin(lang, "err.perm_denied")
	case errors.Is(err, sales.ErrPanelNotAssigned):
		return i18n.Admin(lang, "err.panel_not_assigned")
	case errors.Is(err, sales.ErrNoCreateInbound):
		return i18n.Admin(lang, "err.no_create_inbound")
	case errors.Is(err, sales.ErrQuotaExceeded):
		return i18n.Admin(lang, "err.quota_exceeded")
	default:
		return err.Error()
	}
}
