package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func (s *Server) pagePanelUsers(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	panel, err := s.Store.GetPanel(r.Context(), panelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f := s.panelUserFiltersFromRequest(r)
	pag := paginationFromRequest(r, 0, "q", "inbound", "status", "source", "agent_id")
	agents := s.agentNameMap(r.Context())
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)
	allAgents, _ := s.Store.ListAgents(r.Context())

	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.renderPage(w, "panel_users", r, s.panelUsersPageData(r, panel, nil, nil, err, map[string]any{
			"Stats": panelUserStats{}, "Filters": f, "Templates": templates, "Agents": allAgents,
			"Pagination": pag, "PanelListErr": err,
		}))
		return
	}
	inbounds, _ := client.ListInbounds(r.Context())

	if f.Source == "hodhod" {
		s.renderPanelUsersHodhodPage(w, r, panel, inbounds, f, pag, agents, templates, allAgents)
		return
	}

	statusFilter := f.Status
	if statusFilter == "all" {
		statusFilter = ""
	}
	page, err := client.ListUsersPaged(r.Context(), panels.UserListOptions{
		Page: pag.Page, PageSize: pag.PerPage, Search: f.Query, Status: statusFilter,
	})
	var panelErr error
	var rows []PanelUserRow
	stats := panelUserStats{}
	if err != nil {
		panelErr = err
	} else {
		usernames := make([]string, len(page.Users))
		for i, u := range page.Users {
			usernames[i] = u.Username
		}
		services, _ := s.Store.ListServicesForPanelUsernames(r.Context(), panelID, usernames)
		rows, stats = mergePanelUsersPage(page.Users, services, agents, inbounds, f)
		if f.Source == "both" {
			filtered := rows[:0]
			for _, row := range rows {
				if row.Source == "both" {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
			stats.Shown = len(rows)
		}
		if f.Source == "panel" {
			for i := range rows {
				if rows[i].Source == "both" {
					rows[i].Source = "panel"
				}
			}
		}
		pag.Total = page.Filtered
		if pag.Total == 0 {
			pag.Total = page.Total
		}
		stats.PanelCount = page.Total
	}

	s.renderPage(w, "panel_users", r, s.panelUsersPageData(r, panel, inbounds, rows, panelErr, map[string]any{
		"Stats": stats, "Filters": f, "Templates": templates, "Agents": allAgents,
		"PanelListErr": panelErr, "Pagination": pag,
	}))
}

func (s *Server) renderPanelUsersHodhodPage(w http.ResponseWriter, r *http.Request, panel *db.Panel, inbounds []panels.InboundInfo, f panelUserFilters, pag Pagination, agents map[int64]string, templates []UserCreateTemplate, allAgents []db.Agent) {
	allSvc, _ := s.Store.ListServicesForPanel(r.Context(), panel.ID)
	var hodhodOnly []db.Service
	for _, svc := range allSvc {
		if f.AgentID > 0 && svc.AgentID != f.AgentID {
			continue
		}
		hodhodOnly = append(hodhodOnly, svc)
	}
	var panelUsers []panels.UserInfo
	if client, err := s.Panels.Get(r.Context(), panel.ID); err == nil {
		panelUsers, _ = client.ListUsers(r.Context())
	}
	onPanel := map[string]bool{}
	for _, u := range panelUsers {
		onPanel[u.Username] = true
	}
	var filtered []db.Service
	for _, svc := range hodhodOnly {
		if onPanel[svc.PanelUsername] {
			continue
		}
		row := panelUserRowFromService(svc, agents)
		if matchPanelUserFilters(row, f) {
			filtered = append(filtered, svc)
		}
	}
	pag.Total = len(filtered)
	start := pag.Offset()
	end := start + pag.PerPage
	if start > len(filtered) {
		start = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	pageSvc := filtered[start:end]
	var rows []PanelUserRow
	for _, svc := range pageSvc {
		row := panelUserRowFromService(svc, agents)
		row.Source = "hodhod"
		row.HodhodOnly = true
		rows = append(rows, row)
	}
	stats := panelUserStats{HodhodCount: len(hodhodOnly), Shown: len(rows), PanelCount: len(panelUsers)}
	s.renderPage(w, "panel_users", r, s.panelUsersPageData(r, panel, inbounds, rows, nil, map[string]any{
		"Stats": stats, "Filters": f, "Templates": templates, "Agents": allAgents,
		"Pagination": pag,
	}))
}

func (s *Server) panelUsersPageData(r *http.Request, panel *db.Panel, inbounds []panels.InboundInfo, rows []PanelUserRow, panelErr error, extra ...map[string]any) map[string]any {
	data := map[string]any{
		"Panel": panel, "Inbounds": inbounds, "Rows": rows,
		"PanelListErr": panelErr,
	}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			data[k] = v
		}
	}
	if _, ok := data["Stats"]; !ok {
		data["Stats"] = panelUserStats{}
	}
	if _, ok := data["Filters"]; !ok {
		data["Filters"] = s.panelUserFiltersFromRequest(r)
	}
	if _, ok := data["Templates"]; !ok {
		templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panel.ID)
		data["Templates"] = templates
	}
	if _, ok := data["Agents"]; !ok {
		data["Agents"], _ = s.Store.ListAgents(r.Context())
	}
	tagMap := map[int]string{}
	for _, inb := range inbounds {
		tagMap[inb.ID] = inb.Tag
	}
	data["InboundTagMap"] = tagMap
	return data
}

func (s *Server) panelUserFiltersFromRequest(r *http.Request) panelUserFilters {
	f := panelUserFilters{
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		Inbound: strings.TrimSpace(r.URL.Query().Get("inbound")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Source: strings.TrimSpace(r.URL.Query().Get("source")),
	}
	if f.Status == "" {
		f.Status = "all"
	}
	if v := strings.TrimSpace(r.URL.Query().Get("agent_id")); v != "" {
		f.AgentID, _ = strconv.ParseInt(v, 10, 64)
	}
	return f
}

func (s *Server) agentNameMap(ctx context.Context) map[int64]string {
	agents, _ := s.Store.ListAgents(ctx)
	m := make(map[int64]string, len(agents))
	for _, a := range agents {
		m[a.ID] = a.Name
	}
	return m
}

func (s *Server) postPanelUser(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.panelUserCreateResponse(w, r, panelID, err, "Could not connect to panel")
		return
	}
	inbounds, _ := client.ListInbounds(r.Context())
	inboundIDs := inboundIDsFromForm(r.Form["inbound_ids"], r.FormValue("inbound_ids"), 0)
	if len(inboundIDs) == 0 {
		id, _ := strconv.Atoi(r.FormValue("inbound_id"))
		inboundIDs = inboundIDsFromForm(nil, "", id)
	}
	if len(inbounds) > 0 && len(inboundIDs) == 0 {
		s.panelUserCreateResponse(w, r, panelID, fmt.Errorf("select at least one inbound"), "Select at least one inbound")
		return
	}
	vol, _ := strconv.Atoi(r.FormValue("volume_gb"))
	if vol <= 0 {
		vol = 30
	}
	days, _ := strconv.Atoi(r.FormValue("duration_days"))
	if days <= 0 {
		days = 30
	}
	ipLimit, _ := strconv.Atoi(r.FormValue("ip_limit"))
	expire := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	req := panels.CreateUserRequest{
		Username:       strings.TrimSpace(r.FormValue("username")),
		DataLimitBytes: int64(vol) * 1024 * 1024 * 1024,
		ExpireAt:       expire,
		Note:           r.FormValue("note"),
		LimitIP:        ipLimit,
		Scope:          panels.Scope{InboundIDs: inboundIDs},
	}
	if req.Username == "" {
		s.panelUserCreateResponse(w, r, panelID, fmt.Errorf("username required"), "Username is required")
		return
	}
	_, err = client.CreateUser(r.Context(), req)
	if err != nil {
		s.panelUserCreateResponse(w, r, panelID, err, err.Error())
		return
	}
	if v := r.FormValue("agent_id"); v != "" {
		agentID, _ := strconv.ParseInt(v, 10, 64)
		if agentID > 0 {
			_ = s.Store.UpsertAgentUserGrant(r.Context(), &db.AgentUserGrant{
				AgentID: agentID, PanelID: panelID, PanelUsername: req.Username,
				AllowView: true, AllowModify: true,
			})
			if _, err := s.Store.GetServiceByPanelUsername(r.Context(), panelID, req.Username); err != nil {
				sub, _ := client.SubscriptionURL(r.Context(), req.Username)
				admin := r.Context().Value(ctxAdmin).(*db.Admin)
				adminID := admin.ID
				_ = s.Store.CreateService(r.Context(), &db.Service{
					AgentID: agentID, Source: db.ServiceSourcePanel, PanelID: panelID,
					PanelUsername: req.Username, SubLink: sub, DataLimitBytes: req.DataLimitBytes,
					ExpireAt: &expire, Status: "active", CreatedByAdminID: &adminID,
				})
			}
		}
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/master/panels/%d/users", panelID))
		s.setFlash(w, "ok", "User created on panel")
		return
	}
	s.saveFlash(w, nil, "User created on panel")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func (s *Server) panelUserCreateResponse(w http.ResponseWriter, r *http.Request, panelID int64, err error, msg string) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<div class="alert-err">` + templateEscape(msg) + `</div>`))
		return
	}
	s.setFlash(w, "err", msg)
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func templateEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func (s *Server) postPanelUserTemplate(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	tpl := UserCreateTemplate{
		Name:         r.FormValue("template_name"),
		VolumeGB:     atoiDefault(r.FormValue("volume_gb"), 30),
		DurationDays: atoiDefault(r.FormValue("duration_days"), 30),
		IPLimit:      atoiDefault(r.FormValue("ip_limit"), 0),
		Note:         r.FormValue("note"),
		InboundIDs:   inboundIDsFromForm(r.Form["inbound_ids"], r.FormValue("inbound_ids"), 0),
	}
	if err := saveUserCreateTemplate(r.Context(), s.Store, panelID, tpl); err != nil {
		s.setFlash(w, "err", err.Error())
	} else {
		s.setFlash(w, "ok", "Template saved")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func (s *Server) postPanelUserTemplateDelete(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	name, _ := url.PathUnescape(chi.URLParam(r, "name"))
	if err := deleteUserCreateTemplate(r.Context(), s.Store, panelID, name); err != nil {
		s.setFlash(w, "err", err.Error())
	} else {
		s.setFlash(w, "ok", "Template deleted")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return def
	}
	return n
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
		s.panelUserHTMLError(w, err.Error())
		return
	}
	if err = fn(client, email); err != nil {
		if r.Header.Get("HX-Request") != "" {
			s.panelUserHTMLError(w, err.Error())
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	s.setFlash(w, "ok", "Done")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}

func (s *Server) panelUserHTMLError(w http.ResponseWriter, msg string) {
	if w.Header().Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="alert-err">` + templateEscape(msg) + `</div>`))
		return
	}
	http.Error(w, msg, http.StatusBadRequest)
}

func (s *Server) renderPanelUserRow(w http.ResponseWriter, r *http.Request, panelID int64, email string) {
	panel, _ := s.Store.GetPanel(r.Context(), panelID)
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		http.Error(w, "panel error", http.StatusInternalServerError)
		return
	}
	inbounds, _ := client.ListInbounds(r.Context())
	tagMap := map[int]string{}
	for _, inb := range inbounds {
		tagMap[inb.ID] = inb.Tag
	}
	u, err := client.GetUser(r.Context(), email)
	row := PanelUserRow{Username: email, HodhodOnly: false, Source: "panel"}
	if err == nil {
		row = panelUserRowFromPanel(*u, tagMap)
		row.Source = "panel"
	}
	if svc, err := s.Store.GetServiceByPanelUsername(r.Context(), panelID, email); err == nil {
		agents := s.agentNameMap(r.Context())
		row = mergePanelUserWithService(row, *svc, agents)
		if row.Source == "panel" {
			row.Source = "both"
		} else {
			row.Source = "hodhod"
		}
	}
	data := map[string]any{
		"CSRF": r.Context().Value(ctxCSRF), "Panel": panel, "Row": row, "InboundTagMap": tagMap,
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
		s.panelUserHTMLError(w, err.Error())
		return
	}
	if err = client.DeleteUser(r.Context(), email); err != nil {
		s.panelUserHTMLError(w, err.Error())
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.setFlash(w, "ok", "User deleted")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d/users", panelID), http.StatusSeeOther)
}
