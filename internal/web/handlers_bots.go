package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/botconfig"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/telegram"
)

func (s *Server) botScope(r *http.Request) botconfig.Scope {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if strings.HasPrefix(r.URL.Path, "/master/") {
		return botconfig.Scope{Base: "/master", IsMaster: true}
	}
	agentID := int64(0)
	if admin.AgentID != nil {
		agentID = *admin.AgentID
	}
	return botconfig.Scope{Base: "/agent", IsMaster: false, AgentID: agentID}
}

func (s *Server) requireBotManage(r *http.Request, botID int64) (*db.Admin, botconfig.Scope, bool) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	scope := s.botScope(r)
	if !s.canAccessBot(r, admin, botID) {
		return admin, scope, false
	}
	if admin.Role != db.RoleMaster && !s.canPerm(r, admin, db.PermManageBot) {
		return admin, scope, false
	}
	return admin, scope, true
}

func (s *Server) pageBots(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	scope := s.botScope(r)
	perms, _ := s.permsFor(r, admin)
	var agents []db.Agent
	if scope.IsMaster {
		agents, _ = s.Store.ListAgents(r.Context())
	} else if admin.AgentID != nil {
		a, _ := s.Store.GetAgent(r.Context(), *admin.AgentID)
		agents = []db.Agent{*a}
	}
	rows, err := s.BotSvc.ListForScope(r.Context(), scope, agents)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	for i := range rows {
		whURL, whStatus := s.Telegram.WebhookInfo(r.Context(), rows[i].Bot.ID)
		rows[i].WebhookStatus = whStatus
		if whStatus == "" {
			rows[i].WebhookStatus = whURL
		}
	}
	s.renderPage(w, "bots", r, map[string]any{
		"BotRows": rows, "Agents": agents, "Scope": scope,
		"Caps": botconfig.CapabilitiesFor(admin.Role, perms),
	})
}

func (s *Server) postBot(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	scope := s.botScope(r)
	if !scope.IsMaster && !s.canPerm(r, admin, db.PermManageBot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	agentID := scope.AgentID
	if scope.IsMaster {
		agentID, _ = strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	}
	agent, err := s.Store.GetAgent(r.Context(), agentID)
	if err != nil {
		http.Error(w, "invalid agent", http.StatusBadRequest)
		return
	}
	count, _ := s.Store.CountBotsByAgent(r.Context(), agentID)
	if int(count) >= agent.MaxBots {
		http.Error(w, "max bots reached", http.StatusBadRequest)
		return
	}
	if err := s.postBotWithToken(r, agentID); err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, botconfig.RedirectBots(scope), http.StatusSeeOther)
		return
	}
	bots, _ := s.Store.ListBotsByAgent(r.Context(), agentID)
	if len(bots) > 0 {
		s.audit(r, &admin.ID, "create_bot", "bot", bots[0].ID, map[string]any{"agent_id": agentID})
	}
	s.setFlash(w, "ok", "Bot added")
	http.Redirect(w, r, botconfig.RedirectBots(scope), http.StatusSeeOther)
}

func (s *Server) pageBotSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, id)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bot, err := s.Store.GetBot(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	perms, _ := s.permsFor(r, admin)
	caps := botconfig.CapabilitiesFor(admin.Role, perms)
	whURL, whStatus := s.Telegram.WebhookInfo(r.Context(), id)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "general"
	}
	data := map[string]any{
		"Bot": bot, "Settings": s.BotReader.SettingsMap(r.Context(), id),
		"WebhookURL": whURL, "WebhookStatus": whStatus,
		"Scope": scope, "Caps": caps, "Tab": tab,
		"Cards": mustCards(r, s, id),
		"Channels": mustChannels(r, s, id),
		"MenuButtons": mustMenu(r, s, id),
	}
	if caps.ShowPanelsTab {
		data["BotPanelRows"] = s.botPanelRows(r, id)
		agentPanels, _ := s.Store.ListPanelsForAgent(r.Context(), bot.AgentID)
		data["AgentPanels"] = agentPanels
		type assignPanelOption struct {
			Panel    db.Panel
			Inbounds []panels.InboundInfo
		}
		var assignOpts []assignPanelOption
		for _, p := range agentPanels {
			opt := assignPanelOption{Panel: p}
			if client, err := s.Panels.Get(r.Context(), p.ID); err == nil {
				opt.Inbounds, _ = client.ListInbounds(r.Context())
			}
			assignOpts = append(assignOpts, opt)
		}
		data["AssignPanelOptions"] = assignOpts
		agent, _ := s.Store.GetAgent(r.Context(), bot.AgentID)
		data["AgentPublicURL"] = s.AgentPublicURL(r.Context(), agent)
	}
	if scope.IsMaster {
		data["Plans"], _ = s.Store.ListPlansByBot(r.Context(), id, false)
	}
	s.renderPage(w, "bot_settings", r, data)
}

func mustCards(r *http.Request, s *Server, botID int64) []db.BotPaymentCard {
	c, _ := s.Store.ListPaymentCards(r.Context(), botID)
	return c
}

func mustChannels(r *http.Request, s *Server, botID int64) []db.BotChannel {
	c, _ := s.Store.ListBotChannels(r.Context(), botID)
	return c
}

func mustMenu(r *http.Request, s *Server, botID int64) []db.BotMenuButton {
	m, _ := s.BotReader.MenuButtons(r.Context(), botID)
	return m
}

func (s *Server) postBotSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, id)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "general"
	}
	bot, _ := s.Store.GetBot(r.Context(), id)
	switch tab {
	case "general":
		if st := r.FormValue("status"); st != "" && st != bot.Status {
			bot.Status = st
			_ = s.Store.UpdateBot(r.Context(), bot)
			if st == "active" {
				_ = s.Telegram.Add(r.Context(), id)
			} else {
				_ = s.Telegram.DeleteWebhook(r.Context(), id)
				s.Telegram.Remove(bot.PublicID)
			}
			s.audit(r, &admin.ID, "bot_status", "bot", id, map[string]any{"status": st})
		}
		if token := r.FormValue("token"); token != "" {
			username, err := telegram.ValidateToken(r.Context(), s.Box, token, s.Telegram.HTTPClient())
			if err != nil {
				s.setFlash(w, "err", err.Error())
				http.Redirect(w, r, botconfig.RedirectSettings(scope, id), http.StatusSeeOther)
				return
			}
			enc, _ := s.Box.Encrypt(strings.TrimSpace(token))
			bot.TokenEnc = enc
			bot.Username = username
			_ = s.Store.UpdateBot(r.Context(), bot)
			_ = s.Telegram.Reload(r.Context(), id)
			s.audit(r, &admin.ID, "bot_token_rotate", "bot", id, nil)
		}
		keys := []string{"warn_percent", "currency", "topup_min_toman", "topup_max_toman",
			"trial_enabled", "trial_duration_hours", "trial_volume_gb", "trial_max_per_user"}
		botconfig.SaveSettingsKeys(s.Store, r.Context(), id, keys, formMap(r))
		trialVal := "false"
		if r.FormValue("trial_enabled") == "true" {
			trialVal = "true"
		}
		_ = s.Store.SetSetting(r.Context(), "bot", id, "trial_enabled", trialVal)
		if mode := r.FormValue("card_display_mode"); mode != "" {
			bot.CardDisplayMode = mode
			_ = s.Store.UpdateBot(r.Context(), bot)
		}
	case "messages":
		keys := []string{"welcome_text_fa", "welcome_text_en", "help_text_fa", "help_text_en", "support_contact"}
		botconfig.SaveSettingsKeys(s.Store, r.Context(), id, keys, formMap(r))
	case "notify":
		keys := []string{"approver_tg_id", "expiry_warn_days", "notify_receipt_pending", "notify_purchase", "notify_new_user", "notify_webhook_error"}
		botconfig.SaveSettingsKeys(s.Store, r.Context(), id, keys, formMap(r))
		_ = s.Store.ReplaceNotificationTargets(r.Context(), id, botconfig.ParseApproverField(r.FormValue("approver_tg_id")))
		for _, k := range []string{"notify_receipt_pending", "notify_purchase", "notify_new_user", "notify_webhook_error"} {
			v := "false"
			if r.FormValue(k) == "true" {
				v = "true"
			}
			_ = s.Store.SetSetting(r.Context(), "bot", id, k, v)
		}
	case "menu":
		for _, btn := range mustMenu(r, s, id) {
			enabled := r.FormValue("menu_"+btn.ButtonKey) == "on"
			btn.Enabled = enabled
			_ = s.Store.UpsertMenuButton(r.Context(), &btn)
		}
	}
	s.audit(r, &admin.ID, "bot_settings_save", "bot", id, map[string]any{"tab": tab})
	s.setFlash(w, "ok", "Settings saved")
	http.Redirect(w, r, botconfig.RedirectSettings(scope, id)+"?tab="+tab, http.StatusSeeOther)
}

func formMap(r *http.Request) map[string]string {
	m := make(map[string]string)
	for k := range r.Form {
		m[k] = r.FormValue(k)
	}
	return m
}

func (s *Server) postBotDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.Role != db.RoleMaster || !s.canAccessBot(r, admin, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bot, err := s.Store.GetBot(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.BotSvc.Delete(r.Context(), id); err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
		return
	}
	_ = s.Telegram.DeleteWebhook(r.Context(), id)
	s.Telegram.Remove(bot.PublicID)
	s.audit(r, &admin.ID, "delete_bot", "bot", id, nil)
	s.setFlash(w, "ok", "Bot deleted")
	http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
}

type botPanelRow struct {
	PanelID        int64
	PanelName      string
	InboundIDs     []int
	InboundLabels  []string
}

func (s *Server) botPanelRows(r *http.Request, botID int64) []botPanelRow {
	bps, _ := s.Store.ListBotPanels(r.Context(), botID)
	out := make([]botPanelRow, 0, len(bps))
	for _, bp := range bps {
		row := botPanelRow{PanelID: bp.PanelID}
		if p, err := s.Store.GetPanel(r.Context(), bp.PanelID); err == nil {
			row.PanelName = p.Name
		} else {
			row.PanelName = fmt.Sprintf("Panel %d", bp.PanelID)
		}
		var scope panels.Scope
		_ = json.Unmarshal(bp.ScopeJSON, &scope)
		if len(scope.InboundIDs) == 0 && scope.InboundID > 0 {
			scope.InboundIDs = []int{scope.InboundID}
		}
		row.InboundIDs = scope.InboundIDs
		if client, err := s.Panels.Get(r.Context(), bp.PanelID); err == nil {
			if inb, err := client.ListInbounds(r.Context()); err == nil {
				tagByID := map[int]string{}
				for _, i := range inb {
					tagByID[i.ID] = i.Tag
				}
				if len(row.InboundIDs) == 0 {
					row.InboundLabels = []string{"all inbounds"}
				} else {
					for _, id := range row.InboundIDs {
						tag := tagByID[id]
						if tag == "" {
							tag = fmt.Sprintf("%d", id)
						}
						row.InboundLabels = append(row.InboundLabels, fmt.Sprintf("%d: %s", id, tag))
					}
				}
			}
		}
		if len(row.InboundLabels) == 0 && len(row.InboundIDs) > 0 {
			for _, id := range row.InboundIDs {
				row.InboundLabels = append(row.InboundLabels, strconv.Itoa(id))
			}
		}
		out = append(out, row)
	}
	return out
}

func (s *Server) postBotPanel(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !scope.IsMaster {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	redirect := fmt.Sprintf("/master/bots/%d/settings?tab=panels", botID)
	if panelID <= 0 {
		s.setFlash(w, "err", "Select a panel first")
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
		s.setFlash(w, "err", "Panel is not assigned to this bot's agent")
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
				s.setFlash(w, "err", "Some inbounds are not granted to the agent for create")
				http.Redirect(w, r, redirect, http.StatusSeeOther)
				return
			}
		}
	}
	bp := &db.BotPanel{BotID: botID, PanelID: panelID, ScopeJSON: scopeJSONFromInboundIDs(inboundIDs)}
	err = s.Store.UpsertBotPanel(r.Context(), bp)
	if err == nil {
		s.audit(r, &admin.ID, "bot_panel_link", "bot", botID, map[string]any{"panel_id": panelID, "inbounds": inboundIDs})
	}
	s.saveFlash(w, err, "Panel linked to bot")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) postBotPanelDelete(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	panelID, _ := strconv.ParseInt(chi.URLParam(r, "panel_id"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok || !scope.IsMaster {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	err := s.Store.DeleteBotPanel(r.Context(), botID, panelID)
	if err == nil {
		s.audit(r, &admin.ID, "bot_panel_unlink", "bot", botID, map[string]any{"panel_id": panelID})
	}
	s.saveFlash(w, err, "Panel unlinked from bot")
	http.Redirect(w, r, fmt.Sprintf("/master/bots/%d/settings?tab=panels", botID), http.StatusSeeOther)
}

func (s *Server) pageBotUsers(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = admin
	tgQ, _ := strconv.ParseInt(r.URL.Query().Get("tg_id"), 10, 64)
	pag := paginationFromRequest(r, 0, "tg_id")
	total, _ := s.Store.CountEndUsersFiltered(r.Context(), botID, tgQ)
	pag.Total = int(total)
	users, _ := s.Store.ListEndUsersByBot(r.Context(), botID, pag.PerPage, pag.Offset())
	bot, _ := s.Store.GetBot(r.Context(), botID)
	s.renderPage(w, "bot_users", r, map[string]any{
		"Bot": bot, "Users": users, "Scope": scope, "Pagination": pag, "TgFilter": tgQ,
	})
}

func (s *Server) pageBotUserDetail(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	uid, _ := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	user, err := s.Store.GetEndUser(r.Context(), botID, uid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	txs, _ := s.Store.ListWalletTxByUser(r.Context(), botID, uid, 20)
	svcs, _ := s.Store.ListServicesByUser(r.Context(), botID, uid)
	bot, _ := s.Store.GetBot(r.Context(), botID)
	s.renderPage(w, "bot_user_detail", r, map[string]any{
		"Bot": bot, "User": user, "Txs": txs, "Services": svcs, "Scope": scope,
	})
}

func (s *Server) postBotUserBlock(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	uid, _ := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.SetEndUserStatus(r.Context(), botID, uid, "blocked")
	s.audit(r, &admin.ID, "block_end_user", "end_user", uid, map[string]any{"bot_id": botID})
	http.Redirect(w, r, fmt.Sprintf("%s/bots/%d/users/%d", scope.Base, botID, uid), http.StatusSeeOther)
}

func (s *Server) postBotUserWalletAdjust(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	uid, _ := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	delta, _ := strconv.ParseInt(r.FormValue("delta"), 10, 64)
	reason := r.FormValue("reason")
	if reason == "" {
		reason = "admin_adjust"
	}
	if err := s.BotSvc.AdjustWallet(r.Context(), s.Wallet, botID, uid, delta, reason); err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, fmt.Sprintf("%s/bots/%d/users/%d", scope.Base, botID, uid), http.StatusSeeOther)
		return
	}
	s.audit(r, &admin.ID, "wallet_adjust", "end_user", uid, map[string]any{"bot_id": botID, "delta": delta})
	http.Redirect(w, r, fmt.Sprintf("%s/bots/%d/users/%d", scope.Base, botID, uid), http.StatusSeeOther)
}

func (s *Server) postBotCard(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	c := &db.BotPaymentCard{
		BotID: botID, Label: r.FormValue("label"), CardNumber: r.FormValue("card_number"),
		HolderName: r.FormValue("holder_name"), Active: r.FormValue("active") == "on",
	}
	_ = s.Store.CreatePaymentCard(r.Context(), c)
	http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=cards", http.StatusSeeOther)
}

func (s *Server) postBotCardDelete(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cid, _ := strconv.ParseInt(chi.URLParam(r, "cid"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.DeletePaymentCard(r.Context(), botID, cid)
	http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=cards", http.StatusSeeOther)
}

func (s *Server) postBotCardUpdate(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cid, _ := strconv.ParseInt(chi.URLParam(r, "cid"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	c, err := s.Store.GetPaymentCard(r.Context(), botID, cid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if v := r.FormValue("label"); v != "" {
		c.Label = v
	}
	if v := r.FormValue("card_number"); v != "" {
		c.CardNumber = v
	}
	if v := r.FormValue("holder_name"); v != "" {
		c.HolderName = v
	}
	if v := r.FormValue("weight"); v != "" {
		c.Weight, _ = strconv.Atoi(v)
	}
	c.Active = r.FormValue("active") == "on"
	_ = s.Store.UpdatePaymentCard(r.Context(), c)
	http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=cards", http.StatusSeeOther)
}

func (s *Server) postBotChannel(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	count, _ := s.Store.ListBotChannels(r.Context(), botID)
	if len(count) >= 5 {
		s.setFlash(w, "err", "Maximum 5 channels allowed")
		http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=channels", http.StatusSeeOther)
		return
	}
	ch := &db.BotChannel{
		BotID: botID, Username: strings.TrimSpace(r.FormValue("username")),
		Label: r.FormValue("label"), JoinURL: r.FormValue("join_url"),
		Mandatory: r.FormValue("mandatory") == "on", Active: r.FormValue("active") != "off",
	}
	_ = s.Store.UpsertBotChannel(r.Context(), ch)
	http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=channels", http.StatusSeeOther)
}

func (s *Server) postBotChannelDelete(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cid, _ := strconv.ParseInt(chi.URLParam(r, "cid"), 10, 64)
	_, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.DeleteBotChannel(r.Context(), botID, cid)
	http.Redirect(w, r, botconfig.RedirectSettings(scope, botID)+"?tab=channels", http.StatusSeeOther)
}

func (s *Server) postBotUserUnblock(w http.ResponseWriter, r *http.Request) {
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	uid, _ := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	admin, scope, ok := s.requireBotManage(r, botID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.SetEndUserStatus(r.Context(), botID, uid, "active")
	s.audit(r, &admin.ID, "unblock_end_user", "end_user", uid, map[string]any{"bot_id": botID})
	http.Redirect(w, r, fmt.Sprintf("%s/bots/%d/users/%d", scope.Base, botID, uid), http.StatusSeeOther)
}
