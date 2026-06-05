package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/telegram"
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
	s.renderPage(w, "agent_edit", r, map[string]any{
		"Agent": agent, "Perms": perms, "AgentPanels": agentPanels, "PanelRows": panelRows,
		"AgentAdmin": agentAdmin, "PermFields": permFields(perms),
		"PlatformHost": s.Cfg.MainHost(), "AgentPublicURL": s.AgentPublicURL(r.Context(), agent),
	})
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
	maxUsers, _ := strconv.Atoi(r.FormValue("max_users"))
	capDays, _ := strconv.Atoi(r.FormValue("expiry_cap_days"))
	ap := &db.AgentPanel{
		AgentID: id, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(parseInboundIDs(r.FormValue("inbound_ids"))),
		MaxUsers: maxUsers, ExpiryCapDays: capDays,
	}
	err := s.Store.UpsertAgentPanel(r.Context(), ap)
	s.saveFlash(w, err, "Panel assigned")
	http.Redirect(w, r, fmt.Sprintf("/master/agents/%d", id), http.StatusSeeOther)
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
	err := s.Panels.TestConnection(r.Context(), id)
	ok, msg := panelTestMessage(err)
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

func (s *Server) pageBots(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.Store.ListAgents(r.Context())
	var all []db.Bot
	for _, a := range agents {
		bots, _ := s.Store.ListBotsByAgent(r.Context(), a.ID)
		all = append(all, bots...)
	}
	s.renderPage(w, "bots", r, map[string]any{"Bots": all, "Agents": agents})
}

func (s *Server) postBot(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	agentID, _ := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	agent, err := s.Store.GetAgent(r.Context(), agentID)
	if err != nil {
		http.Error(w, "invalid agent", http.StatusBadRequest)
		return
	}
	count, _ := s.Store.CountBotsByAgent(r.Context(), agentID)
	if int(count) >= agent.MaxBots {
		http.Error(w, "agent max bots reached", http.StatusBadRequest)
		return
	}
	if err := s.postBotWithToken(r, agentID); err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
		return
	}
	s.setFlash(w, "ok", "Bot added")
	http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
}

func (s *Server) pageBotSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bot, err := s.Store.GetBot(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	whURL, whStatus := s.Telegram.WebhookInfo(r.Context(), id)
	botPanels, _ := s.Store.ListBotPanels(r.Context(), id)
	agentPanels, _ := s.Store.ListPanelsForAgent(r.Context(), bot.AgentID)
	agent, _ := s.Store.GetAgent(r.Context(), bot.AgentID)
	s.renderPage(w, "bot_settings", r, map[string]any{
		"Bot": bot, "Settings": s.botSettingsMap(r, id),
		"WebhookURL": whURL, "WebhookStatus": whStatus,
		"BotPanels": botPanels, "AgentPanels": agentPanels,
		"AgentPublicURL": s.AgentPublicURL(r.Context(), agent),
	})
}

func (s *Server) postBotSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if !s.canAccessBot(r, admin, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	bot, _ := s.Store.GetBot(r.Context(), id)
	if st := r.FormValue("status"); st != "" {
		bot.Status = st
		_ = s.Store.UpdateBot(r.Context(), bot)
		if st == "active" {
			_ = s.Telegram.Add(r.Context(), id)
		} else {
			s.Telegram.Remove(bot.PublicID)
		}
	}
	if token := r.FormValue("token"); token != "" {
		username, err := telegram.ValidateToken(r.Context(), s.Box, token, s.Telegram.HTTPClient())
		if err != nil {
			s.setFlash(w, "err", err.Error())
			redirect := fmt.Sprintf("/master/bots/%d/settings", id)
			if admin.Role == db.RoleAgent {
				redirect = fmt.Sprintf("/agent/bots/%d/settings", id)
			}
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		enc, _ := s.Box.Encrypt(strings.TrimSpace(token))
		bot.TokenEnc = enc
		bot.Username = username
		_ = s.Store.UpdateBot(r.Context(), bot)
		_ = s.Telegram.Reload(r.Context(), id)
	}
	s.saveBotSettings(r, id)
	redirect := fmt.Sprintf("/master/bots/%d/settings", id)
	if admin.Role == db.RoleAgent {
		redirect = fmt.Sprintf("/agent/bots/%d/settings", id)
	}
	s.setFlash(w, "ok", "Settings saved")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) postBotPanel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	bp := &db.BotPanel{BotID: botID, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(parseInboundIDs(r.FormValue("inbound_ids")))}
	err := s.Store.UpsertBotPanel(r.Context(), bp)
	s.saveFlash(w, err, "Panel linked to bot")
	http.Redirect(w, r, fmt.Sprintf("/master/bots/%d/settings", botID), http.StatusSeeOther)
}

func (s *Server) pagePanelEdit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := s.Store.GetPanel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	agentRows, _ := s.Store.ListPanelAgentRows(r.Context(), id)
	botRows, _ := s.Store.ListPanelBotRows(r.Context(), id)
	s.renderPage(w, "panel_edit", r, map[string]any{
		"Panel": p, "AgentRows": agentRows, "BotRows": botRows,
	})
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
