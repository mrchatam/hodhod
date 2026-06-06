package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

func (s *Server) pageAgents(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.Store.ListAgents(r.Context())
	s.renderPage(w, "agents", r, map[string]any{"Agents": agents})
}

func (s *Server) postAgent(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	a := &db.Agent{Name: r.FormValue("name"), Status: db.AgentActive}
	if v := r.FormValue("tg_admin_id"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		a.TgAdminID = &n
	}
	if err := s.Store.CreateAgent(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Store.UpsertAgentPermissions(r.Context(), &db.AgentPermissions{
		AgentID: a.ID, ViewOnly: true,
	})
	if u := r.FormValue("login_username"); u != "" {
		pw := r.FormValue("login_password")
		if pw == "" {
			pw = "changeme123"
		}
		hash, _ := HashPassword(pw)
		aid := a.ID
		_ = s.Store.CreateAdmin(r.Context(), &db.Admin{
			Username: u, PasswordHash: hash, Role: db.RoleAgent, AgentID: &aid,
		})
	}
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	s.audit(r, &admin.ID, "create_agent", "agent", a.ID, map[string]any{"name": a.Name})
	s.setFlash(w, "ok", "Agent created")
	http.Redirect(w, r, "/master/agents", http.StatusSeeOther)
}

func (s *Server) pageAgentEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	agent, err := s.Store.GetAgent(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	perms, _ := s.Store.GetAgentPermissions(r.Context(), id)
	agentPanels, _ := s.Store.ListAgentPanels(r.Context(), id)
	allPanels, _ := s.Store.ListPanels(r.Context())
	type panelRow struct {
		Panel    db.Panel
		Inbounds []panels.InboundInfo
	}
	var panelRows []panelRow
	for _, p := range allPanels {
		row := panelRow{Panel: p}
		if client, err := s.Panels.Get(r.Context(), p.ID); err == nil {
			if inb, err := client.ListInbounds(r.Context()); err == nil {
				row.Inbounds = inb
			}
		}
		panelRows = append(panelRows, row)
	}
	admins, _ := s.Store.ListAdminsByAgent(r.Context(), id)
	var agentAdmin *db.Admin
	if len(admins) > 0 {
		agentAdmin = &admins[0]
	}
	inboundGrants, _ := s.Store.ListAgentInboundGrants(r.Context(), id, 0)
	inboundGrantsByPanel := map[int64][]db.AgentInboundGrant{}
	for _, g := range inboundGrants {
		inboundGrantsByPanel[g.PanelID] = append(inboundGrantsByPanel[g.PanelID], g)
	}
	userGrants, _ := s.Store.ListAgentUserGrants(r.Context(), id, 0)
	userGrantsByPanel := map[int64][]db.AgentUserGrant{}
	for _, g := range userGrants {
		userGrantsByPanel[g.PanelID] = append(userGrantsByPanel[g.PanelID], g)
	}
	accessPanelID, _ := strconv.ParseInt(r.URL.Query().Get("access_panel"), 10, 64)
	if accessPanelID == 0 && len(agentPanels) > 0 {
		accessPanelID = agentPanels[0].PanelID
	}
	type accessPanelUsers struct {
		PanelID int64
		Users   []panels.UserInfo
		Err     error
	}
	var accessUsers []accessPanelUsers
	var accessListErr error
	for _, ap := range agentPanels {
		if client, err := s.Panels.Get(r.Context(), ap.PanelID); err == nil {
			users, err := client.ListUsers(r.Context())
			accessUsers = append(accessUsers, accessPanelUsers{PanelID: ap.PanelID, Users: users, Err: err})
			if err != nil && accessListErr == nil {
				accessListErr = err
			}
		} else if accessListErr == nil {
			accessListErr = err
		}
	}
	type accessInboundRow struct {
		InboundID      int
		Tag            string
		AllowCreate    bool
		AllowViewUsers bool
	}
	type accessUserRow struct {
		Username    string
		AllowView   bool
		AllowModify bool
	}
	var accessInboundRows []accessInboundRow
	var accessUserRows []accessUserRow
	grantByInbound := map[int]db.AgentInboundGrant{}
	for _, g := range inboundGrantsByPanel[accessPanelID] {
		grantByInbound[g.InboundID] = g
	}
	for _, pr := range panelRows {
		if pr.Panel.ID != accessPanelID {
			continue
		}
		for _, inb := range pr.Inbounds {
			g := grantByInbound[inb.ID]
			accessInboundRows = append(accessInboundRows, accessInboundRow{
				InboundID: inb.ID, Tag: inb.Tag,
				AllowCreate: g.AllowCreate, AllowViewUsers: g.AllowViewUsers,
			})
		}
		break
	}
	grantByUser := map[string]db.AgentUserGrant{}
	for _, g := range userGrantsByPanel[accessPanelID] {
		grantByUser[g.PanelUsername] = g
	}
	for _, au := range accessUsers {
		if au.PanelID != accessPanelID {
			continue
		}
		for _, u := range au.Users {
			g := grantByUser[u.Username]
			accessUserRows = append(accessUserRows, accessUserRow{
				Username: u.Username, AllowView: g.AllowView, AllowModify: g.AllowModify,
			})
		}
		break
	}
	s.renderPage(w, "agent_edit", r, map[string]any{
		"Agent": agent, "Perms": perms, "AgentPanels": agentPanels, "PanelRows": panelRows,
		"AgentAdmin": agentAdmin, "PermFields": permFields(perms),
		"PlatformHost": s.Cfg.MainHost(), "AgentPublicURL": s.AgentPublicURL(r.Context(), agent),
		"InboundGrants": inboundGrantsByPanel, "UserGrants": userGrantsByPanel,
		"AccessPanelID": accessPanelID, "AccessInboundRows": accessInboundRows,
		"AccessUserRows": accessUserRows, "AccessListErr": accessListErr,
	})
}

func (s *Server) postAgentAccess(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	if panelID == 0 {
		http.Error(w, "panel required", http.StatusBadRequest)
		return
	}
	var inboundGrants []db.AgentInboundGrant
	inboundGrants = parseInboundGrantsFromForm(r.Form)
	if err := s.Store.ReplaceAgentInboundGrants(r.Context(), id, panelID, inboundGrants); err != nil {
		s.saveFlash(w, err, "")
		http.Redirect(w, r, fmt.Sprintf("/master/agents/%d?access_panel=%d#tab-panels", id, panelID), http.StatusSeeOther)
		return
	}
	var userGrants []db.AgentUserGrant
	for key, val := range r.Form {
		if !strings.HasPrefix(key, "user_") || val[0] != "on" {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(key, "user_"), "_", 2)
		if len(parts) != 2 {
			continue
		}
		username, err := url.PathUnescape(parts[0])
		if err != nil {
			username = parts[0]
		}
		g := db.AgentUserGrant{PanelUsername: username}
		switch parts[1] {
		case "view":
			g.AllowView = true
		case "modify":
			g.AllowModify = true
		default:
			continue
		}
		found := false
		for i := range userGrants {
			if userGrants[i].PanelUsername == username {
				if parts[1] == "view" {
					userGrants[i].AllowView = true
				} else {
					userGrants[i].AllowModify = true
				}
				found = true
				break
			}
		}
		if !found {
			userGrants = append(userGrants, g)
		}
	}
	err := s.Store.ReplaceAgentUserGrants(r.Context(), id, panelID, userGrants)
	s.saveFlash(w, err, "Access saved")
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d?access_panel=%d#tab-panels", id, panelID), http.StatusSeeOther)
}

func permFields(p *db.AgentPermissions) []map[string]any {
	type pf struct {
		Key, Label string
		val        bool
	}
	fields := []pf{
		{"create_user", "Create users", p.CreateUser},
		{"modify_user", "Modify users", p.ModifyUser},
		{"add_time", "Add time", p.AddTime},
		{"add_volume", "Add volume", p.AddVolume},
		{"reset_usage", "Reset usage", p.ResetUsage},
		{"disable_enable", "Disable / enable", p.DisableEnable},
		{"delete_user", "Delete users", p.DeleteUser},
		{"manage_bot", "Manage bot", p.ManageBot},
		{"manage_plans", "Manage plans", p.ManagePlans},
	}
	out := make([]map[string]any, len(fields))
	for i, f := range fields {
		out[i] = map[string]any{"Key": f.Key, "Label": f.Label, "Checked": f.val}
	}
	return out
}

func (s *Server) postAgentUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	if _, err := s.Store.GetAgent(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}
	name := r.FormValue("name")
	status := db.AgentStatus(r.FormValue("status"))
	maxBots, _ := strconv.Atoi(r.FormValue("max_bots"))
	floor, _ := strconv.ParseInt(r.FormValue("price_floor"), 10, 64)
	ceiling, _ := strconv.ParseInt(r.FormValue("price_ceiling"), 10, 64)
	var tgAdminID *int64
	if v := r.FormValue("tg_admin_id"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		tgAdminID = &n
	}
	err := s.Store.UpdateAgentSettings(r.Context(), id, name, status, maxBots, floor, ceiling, tgAdminID)
	s.saveFlash(w, err, "Agent saved")
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}

func (s *Server) postAgentPermissions(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	p := &db.AgentPermissions{AgentID: id}
	for _, k := range []string{"create_user", "modify_user", "add_time", "add_volume", "reset_usage", "disable_enable", "delete_user", "manage_bot", "manage_plans"} {
		val := r.FormValue(k) == "on"
		switch k {
		case "create_user":
			p.CreateUser = val
		case "modify_user":
			p.ModifyUser = val
		case "add_time":
			p.AddTime = val
		case "add_volume":
			p.AddVolume = val
		case "reset_usage":
			p.ResetUsage = val
		case "disable_enable":
			p.DisableEnable = val
		case "delete_user":
			p.DeleteUser = val
		case "manage_bot":
			p.ManageBot = val
		case "manage_plans":
			p.ManagePlans = val
		}
	}
	p.ViewOnly = !(p.CreateUser || p.ModifyUser || p.AddTime || p.AddVolume ||
		p.ResetUsage || p.DisableEnable || p.DeleteUser || p.ManageBot || p.ManagePlans)
	err := s.Store.UpsertAgentPermissions(r.Context(), p)
	s.saveFlash(w, err, "Permissions saved")
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}

func (s *Server) postAgentPanel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	if panelID <= 0 {
		s.setFlash(w, "err", "Select a panel")
		http.Redirect(w, r, fmt.Sprintf("/master/agents/%d#tab-panels", id), http.StatusSeeOther)
		return
	}
	maxUsers, _ := strconv.Atoi(r.FormValue("max_users"))
	capDays, _ := strconv.Atoi(r.FormValue("expiry_cap_days"))
	ap := &db.AgentPanel{
		AgentID: id, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(nil),
		MaxUsers: maxUsers, ExpiryCapDays: capDays,
	}
	err := s.Store.UpsertAgentPanel(r.Context(), ap)
	if err != nil {
		s.saveFlash(w, err, "")
		http.Redirect(w, r, fmt.Sprintf("/master/agents/%d#tab-panels", id), http.StatusSeeOther)
		return
	}
	inboundGrants := parseInboundGrantsFromForm(r.Form)
	if err := s.Store.ReplaceAgentInboundGrants(r.Context(), id, panelID, inboundGrants); err != nil {
		s.saveFlash(w, err, "")
	} else {
		s.setFlash(w, "ok", "Panel assigned")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d?access_panel=%d#tab-panels", id, panelID), http.StatusSeeOther)
}

func (s *Server) postAgentAttachInboundUsers(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	inboundID, _ := strconv.Atoi(r.FormValue("inbound_id"))
	redirect := fmt.Sprintf("/master/agents/%d?access_panel=%d#tab-panels", id, panelID)
	if panelID <= 0 || inboundID <= 0 {
		s.setFlash(w, "err", "Invalid panel or inbound")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	ok, err := s.Store.AgentHasPanel(r.Context(), id, panelID)
	if err != nil || !ok {
		s.setFlash(w, "err", "Panel not assigned to agent")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	client, err := s.Panels.Get(r.Context(), panelID)
	if err != nil {
		s.saveFlash(w, err, "")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	users, err := client.ListUsers(r.Context())
	if err != nil {
		s.saveFlash(w, err, "")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	inboundSet := map[int]bool{inboundID: true}
	var names []string
	for _, u := range users {
		if userOnInbound(u, inboundSet) {
			names = append(names, u.Username)
		}
	}
	n, err := s.attachPanelUsersToAgent(r.Context(), id, panelID, names)
	if err != nil {
		s.saveFlash(w, err, "")
	} else {
		s.setFlash(w, "ok", fmt.Sprintf("Attached %d users", n))
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) postAgentResetPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	admins, _ := s.Store.ListAdminsByAgent(r.Context(), id)
	if len(admins) == 0 {
		http.Error(w, "no login", http.StatusBadRequest)
		return
	}
	hash, _ := HashPassword(r.FormValue("password"))
	admins[0].PasswordHash = hash
	err := s.Store.UpdateAdmin(r.Context(), &admins[0])
	s.saveFlash(w, err, "Password reset")
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}

func (s *Server) pagePanels(w http.ResponseWriter, r *http.Request) {
	panels, _ := s.Store.ListPanels(r.Context())
	type panelRow struct {
		db.Panel
		AgentCount int64
	}
	rows := make([]panelRow, 0, len(panels))
	for _, p := range panels {
		n, _ := s.Store.CountAgentPanelsByPanel(r.Context(), p.ID)
		rows = append(rows, panelRow{Panel: p, AgentCount: n})
	}
	s.renderPage(w, "panels", r, map[string]any{"PanelRows": rows})
}

func (s *Server) postPanel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	enc, _ := s.Box.Encrypt(r.FormValue("password"))
	apiTokenEnc := ""
	if token := r.FormValue("api_token"); token != "" {
		apiTokenEnc, _ = s.Box.Encrypt(token)
	}
	p := &db.Panel{
		Type: db.PanelType(r.FormValue("type")), Name: r.FormValue("name"),
		BaseURL: r.FormValue("base_url"), BasePath: r.FormValue("base_path"),
		Username: r.FormValue("username"), PasswordEnc: enc, APITokenEnc: apiTokenEnc, Status: "active",
	}
	err := s.Store.CreatePanel(r.Context(), p)
	if err == nil {
		s.Panels.Invalidate(p.ID)
	}
	s.saveFlash(w, err, "Panel added")
	http.Redirect(w, r, "/master/panels", http.StatusSeeOther)
}

func (s *Server) testPanel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	lang, _ := s.Store.GetSetting(r.Context(), "admin", admin.ID, "lang")
	err := s.Panels.TestConnection(r.Context(), id)
	ok, msg := panelTestMessage(lang, err)
	if err != nil {
		admin := r.Context().Value(ctxAdmin).(*db.Admin)
		s.audit(r, &admin.ID, "panel_test_fail", "panel", id, map[string]any{"error": err.Error()})
	}
	if r.Header.Get("HX-Request") != "" {
		s.renderPartial(w, "panels", "panel_test_result", map[string]any{"OK": ok, "Message": msg})
		return
	}
	if ok {
		s.setFlash(w, "ok", msg)
	} else {
		s.setFlash(w, "err", msg)
	}
	http.Redirect(w, r, "/master/panels", http.StatusSeeOther)
}

func (s *Server) pagePanelEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := s.Store.GetPanel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "details"
	}
	agentRows, _ := s.Store.ListPanelAgentRows(r.Context(), id)
	botRows, _ := s.Store.ListPanelBotRows(r.Context(), id)
	data := map[string]any{
		"Panel": p, "AgentRows": agentRows, "BotRows": botRows, "Tab": tab,
	}
	allAgents, _ := s.Store.ListAgents(r.Context())
	data["AllAgents"] = allAgents
	if client, err := s.Panels.Get(r.Context(), id); err == nil {
		data["Inbounds"], _ = client.ListInbounds(r.Context())
	}
	var assignBots []db.Bot
	seenBot := map[int64]bool{}
	for _, ar := range agentRows {
		bots, _ := s.Store.ListBotsByAgent(r.Context(), ar.AgentID)
		for _, b := range bots {
			if !seenBot[b.ID] {
				seenBot[b.ID] = true
				assignBots = append(assignBots, b)
			}
		}
	}
	data["AssignBots"] = assignBots
	switch tab {
	case "users":
		for k, v := range s.loadPanelUsersViewData(r, p) {
			data[k] = v
		}
	case "backups":
		for k, v := range s.loadPanelBackupsViewData(r, p) {
			data[k] = v
		}
	case "templates":
		templates, _ := loadUserCreateTemplates(r.Context(), s.Store, id)
		data["Templates"] = templates
	}
	s.renderPage(w, "panel_edit", r, data)
}

func (s *Server) postPanelAgent(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	agentID, _ := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	if agentID <= 0 {
		s.setFlash(w, "err", "Select an agent")
		http.Redirect(w, r, panelTabURL(panelID, "agents"), http.StatusSeeOther)
		return
	}
	maxUsers, _ := strconv.Atoi(r.FormValue("max_users"))
	capDays, _ := strconv.Atoi(r.FormValue("expiry_cap_days"))
	ap := &db.AgentPanel{
		AgentID: agentID, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(nil),
		MaxUsers: maxUsers, ExpiryCapDays: capDays,
	}
	if err := s.Store.UpsertAgentPanel(r.Context(), ap); err != nil {
		s.saveFlash(w, err, "")
		http.Redirect(w, r, panelTabURL(panelID, "agents"), http.StatusSeeOther)
		return
	}
	inboundGrants := parseInboundGrantsFromForm(r.Form)
	if err := s.Store.ReplaceAgentInboundGrants(r.Context(), agentID, panelID, inboundGrants); err != nil {
		s.saveFlash(w, err, "")
	} else {
		s.setFlash(w, "ok", "Agent assigned")
	}
	http.Redirect(w, r, panelTabURL(panelID, "agents"), http.StatusSeeOther)
}

func (s *Server) postPanelBot(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	botID, _ := strconv.ParseInt(r.FormValue("bot_id"), 10, 64)
	redirect := panelTabURL(panelID, "bots")
	if botID <= 0 {
		s.setFlash(w, "err", "Select a bot")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	bot, err := s.Store.GetBot(r.Context(), botID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	okAgent, err := s.Store.AgentHasPanel(r.Context(), bot.AgentID, panelID)
	if err != nil || !okAgent {
		s.setFlash(w, "err", "Bot's agent is not assigned to this panel")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	inboundIDs := inboundIDsFromForm(r.Form["inbound_ids"], r.FormValue("inbound_ids"), 0)
	if len(inboundIDs) > 0 {
		granted, _ := s.Store.ListAgentInboundCreateIDs(r.Context(), bot.AgentID, panelID)
		grantSet := map[int]bool{}
		for _, id := range granted {
			grantSet[id] = true
		}
		for _, id := range inboundIDs {
			if len(granted) > 0 && !grantSet[id] {
				s.setFlash(w, "err", "Some inbounds are not granted to the agent")
				http.Redirect(w, r, redirect, http.StatusSeeOther)
				return
			}
		}
	}
	bp := &db.BotPanel{BotID: botID, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(inboundIDs)}
	s.saveFlash(w, s.Store.UpsertBotPanel(r.Context(), bp), "Bot linked to panel")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) postPanelBotDelete(w http.ResponseWriter, r *http.Request) {
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	botID, _ := strconv.ParseInt(chi.URLParam(r, "bot_id"), 10, 64)
	s.saveFlash(w, s.Store.DeleteBotPanel(r.Context(), botID, panelID), "Bot unlinked")
	http.Redirect(w, r, panelTabURL(panelID, "bots"), http.StatusSeeOther)
}

func (s *Server) postPanelUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	p, err := s.Store.GetPanel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p.Name = r.FormValue("name")
	p.BaseURL = r.FormValue("base_url")
	p.BasePath = r.FormValue("base_path")
	p.Username = r.FormValue("username")
	if pw := r.FormValue("password"); pw != "" {
		p.PasswordEnc, _ = s.Box.Encrypt(pw)
	}
	if token := r.FormValue("api_token"); token != "" {
		p.APITokenEnc, _ = s.Box.Encrypt(token)
	}
	err = s.Store.UpdatePanel(r.Context(), p)
	if err == nil {
		s.Panels.Invalidate(id)
	}
	s.saveFlash(w, err, "Panel updated")
	http.Redirect(w, r, fmt.Sprintf("/master/panels/%d", id), http.StatusSeeOther)
}

func (s *Server) postPanelDisable(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := s.Store.GetPanel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p.Status = "disabled"
	err = s.Store.UpdatePanel(r.Context(), p)
	if err == nil {
		s.Panels.Invalidate(id)
	}
	s.saveFlash(w, err, "Panel disabled")
	http.Redirect(w, r, "/master/panels", http.StatusSeeOther)
}

func (s *Server) postAgentDomain(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	domain := r.FormValue("custom_domain")
	if err := s.Store.SetAgentDomain(r.Context(), id, domain); err != nil {
		s.setFlash(w, "err", err.Error())
	} else {
		s.setFlash(w, "ok", "Domain saved — verify DNS next")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}

func (s *Server) postAgentVerifyDomain(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	agent, err := s.Store.GetAgent(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	domain := db.AgentDomain(agent)
	if domain == "" {
		s.setFlash(w, "err", "Set a domain first")
		http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
		return
	}
	platformHost := s.Cfg.MainHost()
	if err := s.DomainVerifier.Verify(r.Context(), domain, agent.DomainVerifyToken, platformHost); err != nil {
		s.setFlash(w, "err", "DNS verification failed")
	} else {
		err := s.Store.MarkAgentDomainVerified(r.Context(), id)
		s.saveFlash(w, err, "Domain verified")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}

func (s *Server) postAgentDomainToggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	enable := r.FormValue("enable") == "1"
	if err := s.Store.SetAgentDomainEnabled(r.Context(), id, enable); err != nil {
		s.setFlash(w, "err", err.Error())
	} else if enable {
		s.setFlash(w, "ok", "Custom domain enabled")
	} else {
		s.setFlash(w, "ok", "Custom domain disabled")
	}
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
}
