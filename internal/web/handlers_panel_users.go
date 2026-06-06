package web

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/sales"
)

func panelTabURL(panelID int64, tab string) string {
	return fmt.Sprintf("/master/panels/%d?tab=%s", panelID, tab)
}

func (s *Server) pagePanelUsers(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target := panelTabURL(panelID, "users")
	if q := r.URL.RawQuery; q != "" {
		target += "&" + q
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (s *Server) loadPanelUsersViewData(r *http.Request, panel *db.Panel) map[string]any {
	panelID := panel.ID
	f := s.panelUserFiltersFromRequest(r)
	pag := paginationFromRequest(r, 0, "tab", "q", "inbound", "status", "source", "agent_id")
	if pag.Query["tab"] == "" {
		pag.Query["tab"] = "users"
	}
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)
	allAgents, _ := s.Store.ListAgents(r.Context())
	agents := s.agentNameMap(r.Context())

	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		return s.panelUsersPageData(r, panel, nil, nil, err, map[string]any{
			"Stats": panelUserStats{}, "Filters": f, "Templates": templates, "Agents": allAgents,
			"Pagination": pag, "PanelListErr": err,
		})
	}
	inbounds, _ := client.ListInbounds(r.Context())

	if f.Source == "hodhod" {
		return s.panelUsersHodhodData(r, panel, inbounds, f, pag, agents, templates, allAgents)
	}

	result := s.ListPanelAccounts(r, panel, ListPanelAccountsQuery{
		PanelID: panelID, Filters: f, Pagination: pag, EnrichOnline: true,
	})
	return s.panelUsersPageData(r, panel, inbounds, result.Rows, result.PanelErr, map[string]any{
		"Stats": result.Stats, "Filters": f, "Templates": templates, "Agents": allAgents,
		"PanelListErr": result.PanelErr, "Pagination": result.Pagination,
		"PanelEmptyHint": result.PanelErr == nil && len(result.Rows) == 0 && result.Stats.PanelCount == 0,
		"CanDelDepleted": panel.Type == db.PanelXUI,
	})
}

func (s *Server) panelUsersHodhodData(r *http.Request, panel *db.Panel, inbounds []panels.InboundInfo, f panelUserFilters, pag Pagination, agents map[int64]string, templates []UserCreateTemplate, allAgents []db.Agent) map[string]any {
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
	return s.panelUsersPageData(r, panel, inbounds, rows, nil, map[string]any{
		"Stats": stats, "Filters": f, "Templates": templates, "Agents": allAgents,
		"Pagination": pag,
	})
}

func (s *Server) renderPanelUsersHodhodPage(w http.ResponseWriter, r *http.Request, panel *db.Panel, inbounds []panels.InboundInfo, f panelUserFilters, pag Pagination, agents map[int64]string, templates []UserCreateTemplate, allAgents []db.Agent) {
	data := s.panelUsersHodhodData(r, panel, inbounds, f, pag, agents, templates, allAgents)
	s.renderPage(w, "panel_edit", r, data)
}

func (s *Server) panelUsersPageData(r *http.Request, panel *db.Panel, inbounds []panels.InboundInfo, rows []PanelUserRow, panelErr error, extra ...map[string]any) map[string]any {
	data := map[string]any{
		"Panel": panel, "Inbounds": inbounds, "Rows": rows,
		"PanelListErr": panelErr,
	}
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	data["IsMaster"] = admin != nil && admin.Role == db.RoleMaster
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
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	if lang == "" {
		lang = "fa"
	}
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
	days, _ := strconv.Atoi(r.FormValue("duration_days"))
	ipLimit, _ := strconv.Atoi(r.FormValue("ip_limit"))
	username := strings.TrimSpace(r.FormValue("username"))
	if username == "" {
		s.panelUserCreateResponse(w, r, panelID, fmt.Errorf("username required"), "Username is required")
		return
	}
	agentID, _ := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	if agentID <= 0 {
		s.panelUserCreateResponse(w, r, panelID, fmt.Errorf("agent required"), "Select an agent for Hodhod tracking")
		return
	}
	templates, _ := loadUserCreateTemplates(r.Context(), s.Store, panelID)
	if tpl, ok := findUserCreateTemplate(templates, r.FormValue("template_name")); ok {
		if tpl.VolumeGB > 0 {
			vol = tpl.VolumeGB
		}
		if tpl.DurationDays > 0 {
			days = tpl.DurationDays
		}
		if len(tpl.InboundIDs) > 0 {
			inboundIDs = tpl.InboundIDs
		}
		if tpl.IPLimit > 0 && ipLimit == 0 {
			ipLimit = tpl.IPLimit
		}
	}
	note := r.FormValue("note")
	svc, err := s.Sales.CreatePanelAccount(r.Context(), sales.CreatePanelAccountInput{
		CreateManualInput: sales.CreateManualInput{
			AgentID: agentID, PanelID: panelID, Label: note,
			VolumeGB: vol, DurationDays: days, InboundIDs: inboundIDs,
			AdminID: admin.ID, IsMaster: true,
		},
		ManualUsername: username,
		SkipAgentQuota: true,
		SkipPermCheck:  true,
		Note:           note,
		LimitIP:        ipLimit,
	})
	if err != nil {
		s.panelUserCreateResponse(w, r, panelID, err, friendlySalesErr(lang, err))
		return
	}
	panelUsername := username
	if svc != nil {
		panelUsername = svc.PanelUsername
	}
	if agentID > 0 {
		if _, err := s.attachPanelUsersToAgent(r.Context(), agentID, panelID, []string{panelUsername}); err != nil {
			s.panelUserCreateResponse(w, r, panelID, err, err.Error())
			return
		}
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", panelTabURL(panelID, "users"))
		s.setFlash(w, "ok", "User created on panel")
		return
	}
	s.saveFlash(w, nil, "User created on panel")
	http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
}

func (s *Server) panelUserCreateResponse(w http.ResponseWriter, r *http.Request, panelID int64, err error, msg string) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<div class="alert-err">` + templateEscape(msg) + `</div>`))
		return
	}
	s.setFlash(w, "err", msg)
	http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
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
	http.Redirect(w, r, panelTabURL(panelID, "templates"), http.StatusSeeOther)
}

func (s *Server) postPanelUserTemplateDelete(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	name, _ := url.PathUnescape(chi.URLParam(r, "name"))
	if err := deleteUserCreateTemplate(r.Context(), s.Store, panelID, name); err != nil {
		s.setFlash(w, "err", err.Error())
	} else {
		s.setFlash(w, "ok", "Template deleted")
	}
	http.Redirect(w, r, panelTabURL(panelID, "templates"), http.StatusSeeOther)
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

func (s *Server) panelUserAction(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64, string) error) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	if err := fn(r.Context(), panelID, email); err != nil {
		if r.Header.Get("HX-Request") != "" {
			s.panelUserHTMLError(w, err.Error())
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	s.setFlash(w, "ok", "Done")
	http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
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
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	agentScope := admin != nil && admin.Role == db.RoleAgent && admin.AgentID != nil

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
	row := PanelUserRow{Username: email, HodhodOnly: false, Source: "panel", PanelID: panelID}
	if users, err := client.ListUsers(r.Context()); err == nil {
		for _, u := range dedupePanelUsersByUsername(users) {
			if u.Username == email {
				row = panelUserRowFromPanel(u, tagMap)
				row.Source = "panel"
				row.PanelID = panelID
				break
			}
		}
	}
	if row.InboundIDs == nil && row.InboundTags == nil {
		if u, err := client.GetUser(r.Context(), email); err == nil {
			row = panelUserRowFromPanel(*u, tagMap)
			row.Source = "panel"
			row.PanelID = panelID
		}
	}
	agents := s.agentNameMap(r.Context())
	if svc, err := s.Store.GetServiceByPanelUsername(r.Context(), panelID, email); err == nil {
		row = mergePanelUserWithService(row, *svc, agents)
		if row.Source == "panel" {
			row.Source = "both"
		} else {
			row.Source = "hodhod"
		}
		row.PanelID = panelID
	}

	pageName := "panel_edit"
	data := s.baseData(r)
	data["Panel"] = panel
	data["Row"] = row
	data["InboundTagMap"] = tagMap
	if agentScope {
		agentID := *admin.AgentID
		perms, _ := s.permsFor(r, admin)
		data["Perms"] = perms
		if visible, err := s.buildAgentVisibleUsers(r.Context(), agentID, panelID); err == nil {
			if v, ok := visible[agentUserKeyStr(panelID, email)]; ok {
				row.CanModify = agentCanModifyUser(v, s.agentHasModifyPerm(perms))
				data["Row"] = row
			}
		}
		data["IsMaster"] = false
		data["Scope"] = "agent-panel"
		data["ShowOnline"] = false
		data["ShowSource"] = false
		data["ShowInbound"] = false
		pageName = "agent_panel_customers"
	} else {
		data["Agents"], _ = s.Store.ListAgents(r.Context())
		data["IsMaster"] = true
		data["Scope"] = "master-panel"
		data["ShowOnline"] = true
		data["ShowSource"] = true
		data["ShowInbound"] = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.pages[pageName]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "user_row", data); err != nil {
		slog.Error("template error", "partial", "user_row", "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) postPanelUserModify(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	_ = r.ParseForm()
	in := modifyInputFromForm(r)
	_, err := s.Sales.ModifyPanelAccount(r.Context(), sales.ModifyPanelAccountInput{
		ModifyInput: in,
		PanelID:     panelID,
		Username:    email,
		IsMaster:    true,
	})
	if err != nil {
		if r.Header.Get("HX-Request") != "" {
			s.panelUserHTMLError(w, err.Error())
			return
		}
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	s.setFlash(w, "ok", "Done")
	http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
}

func (s *Server) postPanelUserReset(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, s.Sales.ResetPanelUsage)
}

func (s *Server) postPanelUserDisable(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, func(ctx context.Context, panelID int64, email string) error {
		return s.Sales.SetPanelAccountEnabled(ctx, panelID, email, false)
	})
}

func (s *Server) postPanelUserEnable(w http.ResponseWriter, r *http.Request) {
	s.panelUserAction(w, r, func(ctx context.Context, panelID int64, email string) error {
		return s.Sales.SetPanelAccountEnabled(ctx, panelID, email, true)
	})
}

func (s *Server) postPanelUserAttachAgent(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	email := s.panelUserEmail(r)
	_ = r.ParseForm()
	agentID, _ := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	redirect := panelTabURL(panelID, "users")
	if agentID <= 0 || email == "" {
		s.setFlash(w, "err", "Select an agent")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	ok, err := s.Store.AgentHasPanel(r.Context(), agentID, panelID)
	if err != nil || !ok {
		s.setFlash(w, "err", "Panel not assigned to agent")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	n, err := s.attachPanelUsersToAgent(r.Context(), agentID, panelID, []string{email})
	if err != nil {
		s.saveFlash(w, err, "")
	} else if n == 0 {
		s.setFlash(w, "err", "Could not attach user")
	} else {
		s.setFlash(w, "ok", "User attached to agent")
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPanelUserRow(w, r, panelID, email)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) postPanelUsersDelDepleted(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	cleaner, ok := client.(panels.DepletedCleaner)
	if !ok {
		http.Error(w, "not supported", http.StatusBadRequest)
		return
	}
	n, err := cleaner.DeleteDepletedClients(r.Context())
	if err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
		return
	}
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	s.audit(r, &admin.ID, "del_depleted_clients", "panel", panelID, map[string]any{"deleted": n})
	s.setFlash(w, "ok", fmt.Sprintf("Removed %d depleted clients", n))
	http.Redirect(w, r, panelTabURL(panelID, "users"), http.StatusSeeOther)
}
