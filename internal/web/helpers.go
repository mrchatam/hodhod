package web

import (
	"net/http"

	"github.com/mrchatam/hodhod/internal/db"
)

func (s *Server) baseData(r *http.Request) map[string]any {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	perms, _ := s.permsFor(r, admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	if lang == "" {
		lang = "en"
	}
	return map[string]any{
		"Admin":    admin,
		"CSRF":     r.Context().Value(ctxCSRF),
		"Perms":    perms,
		"Flash":    s.popFlash(r),
		"IsMaster": admin.Role == db.RoleMaster,
		"Lang":     lang,
		"IsRTL":    lang == "fa",
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
	return p.Has(perm)
}
