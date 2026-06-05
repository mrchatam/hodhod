package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/panels"
)

func (s *Server) pagePanelUsers(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	panel, err := s.Store.GetPanel(r.Context(), panelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.setFlash(w, "err", "Could not connect to panel")
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d", panelID), http.StatusSeeOther)
		return
	}
	users, err := client.ListUsers(r.Context())
	if err != nil {
		s.setFlash(w, "err", "Failed to list panel users: "+err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d", panelID), http.StatusSeeOther)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	inboundFilter := strings.TrimSpace(r.URL.Query().Get("inbound"))
	var filtered []panels.UserInfo
	for _, u := range users {
		if q != "" && !strings.Contains(strings.ToLower(u.Username), q) {
			continue
		}
		if inboundFilter != "" && inboundFilter != fmt.Sprintf("%d", u.InboundID) && inboundFilter != u.InboundTag {
			continue
		}
		filtered = append(filtered, u)
	}
	inbounds, _ := client.ListInbounds(r.Context())
	s.renderPage(w, "panel_users", r, map[string]any{
		"Panel": panel, "Users": filtered, "Inbounds": inbounds,
		"Query": q, "InboundFilter": inboundFilter,
	})
}

func (s *Server) postPanelUser(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
		return
	}
	vol, _ := strconv.Atoi(r.FormValue("volume_gb"))
	days, _ := strconv.Atoi(r.FormValue("duration_days"))
	ipLimit, _ := strconv.Atoi(r.FormValue("ip_limit"))
	inboundID, _ := strconv.Atoi(r.FormValue("inbound_id"))
	expire := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	req := panels.CreateUserRequest{
		Username:       r.FormValue("username"),
		DataLimitBytes: int64(vol) * 1024 * 1024 * 1024,
		ExpireAt:       expire,
		Note:           r.FormValue("note"),
		LimitIP:        ipLimit,
		Scope:          panels.Scope{InboundID: inboundID},
	}
	if ids := parseInboundIDs(r.FormValue("inbound_ids")); len(ids) > 0 {
		req.Scope.InboundIDs = ids
	}
	_, err = client.CreateUser(r.Context(), req)
	s.saveFlash(w, err, "User created on panel")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func (s *Server) panelUserEmail(r *http.Request) string {
	email, _ := url.PathUnescape(chi.URLParam(r, "email"))
	return email
}

func (s *Server) panelUserAction(w http.ResponseWriter, r *http.Request, fn func(client panels.Client, email string) error) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		if r.Header.Get("HX-Request") != "" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
		return
	}
	err = fn(client, email)
	if err != nil {
		if r.Header.Get("HX-Request") != "" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.setFlash(w, "err", err.Error())
	} else if r.Header.Get("HX-Request") == "" {
		s.setFlash(w, "ok", "Done")
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func (s *Server) renderPanelUserRow(w http.ResponseWriter, r *http.Request, panelID int64, email string) {
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		http.Error(w, "panel error", http.StatusInternalServerError)
		return
	}
	u, err := client.GetUser(r.Context(), email)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	panel, _ := s.Store.GetPanel(r.Context(), panelID)
	data := map[string]any{
		"CSRF": r.Context().Value(ctxCSRF), "Panel": panel, "User": u,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.pages["panel_users"]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "panel_user_row", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) postPanelUserModify(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	req := panels.UpdateUserRequest{}
	if v := r.FormValue("volume_gb"); v != "" {
		gb, _ := strconv.Atoi(v)
		bytes := int64(gb) * 1024 * 1024 * 1024
		req.DataLimitBytes = &bytes
	}
	if v := r.FormValue("duration_days"); v != "" {
		d, _ := strconv.Atoi(v)
		t := time.Now().Add(time.Duration(d) * 24 * time.Hour)
		req.ExpireAt = &t
	}
	if v := r.FormValue("expire_at"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			req.ExpireAt = &t
		}
	}
	s.panelUserAction(w, r, func(client panels.Client, email string) error {
		_, err := client.UpdateUser(r.Context(), email, req)
		return err
	})
}

func (s *Server) postPanelUserReset(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, func(client panels.Client, email string) error {
		return client.ResetUsage(r.Context(), email)
	})
}

func (s *Server) postPanelUserDisable(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, func(client panels.Client, email string) error {
		return client.Disable(r.Context(), email)
	})
}

func (s *Server) postPanelUserEnable(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, func(client panels.Client, email string) error {
		return client.Enable(r.Context(), email)
	})
}

func (s *Server) postPanelUserDelete(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		if r.Header.Get("HX-Request") != "" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
		return
	}
	err = client.DeleteUser(r.Context(), email)
	if err != nil {
		if r.Header.Get("HX-Request") != "" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.setFlash(w, "ok", "User deleted")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}
