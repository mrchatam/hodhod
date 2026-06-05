package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/telegram"
)

func (s *Server) pageAgentPlans(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !s.canPerm(r, admin, db.PermManagePlans) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bots, _ := s.Store.ListBotsByAgent(r.Context(), *admin.AgentID)
	var plans []db.Plan
	type panelOption struct {
		BotID, PanelID int64
		PanelName      string
	}
	var panelOptions []panelOption
	for _, b := range bots {
		ps, _ := s.Store.ListPlansByBot(r.Context(), b.ID, false)
		plans = append(plans, ps...)
		panels, _ := s.Store.ListPanelsForBot(r.Context(), b.ID)
		for _, p := range panels {
			panelOptions = append(panelOptions, panelOption{BotID: b.ID, PanelID: p.ID, PanelName: p.Name})
		}
	}
	s.renderPage(w, "agent_plans", r, map[string]any{
		"Plans": plans, "Bots": bots, "PanelOptions": panelOptions,
	})
}

func (s *Server) postAgentPlan(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermManagePlans) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	agent, _ := s.Store.GetAgent(r.Context(), *admin.AgentID)
	botID, _ := strconv.ParseInt(r.FormValue("bot_id"), 10, 64)
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	price, _ := strconv.ParseInt(r.FormValue("price_toman"), 10, 64)
	dur, _ := strconv.Atoi(r.FormValue("duration_days"))
	vol, _ := strconv.Atoi(r.FormValue("volume_gb"))
	if _, err := s.Store.GetBotForAgent(r.Context(), *admin.AgentID, botID); err != nil {
		http.Error(w, "invalid bot", http.StatusBadRequest)
		return
	}
	ok, err := s.Store.BotHasPanel(r.Context(), botID, panelID)
	if err != nil || !ok {
		http.Error(w, "panel is not assigned to bot", http.StatusBadRequest)
		return
	}
	plan := &db.Plan{
		BotID: botID, AgentID: *admin.AgentID,
		Name: r.FormValue("name"), DurationDays: dur, VolumeGB: vol, PriceToman: price,
		PanelID: &panelID, Status: "active",
	}
	if err := billing.ValidatePlanForAgent(plan, agent); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = s.Store.CreatePlan(r.Context(), plan)
	s.setFlash(w, "ok", "Plan created")
	http.Redirect(w, r, "/agent/plans", http.StatusSeeOther)
}

func (s *Server) postAgentPlanUpdate(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermManagePlans) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	planID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	botID, _ := strconv.ParseInt(r.FormValue("bot_id"), 10, 64)
	plan, err := s.Store.GetPlan(r.Context(), botID, planID)
	if err != nil || plan.AgentID != *admin.AgentID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	agent, _ := s.Store.GetAgent(r.Context(), *admin.AgentID)
	plan.Name = r.FormValue("name")
	plan.DurationDays, _ = strconv.Atoi(r.FormValue("duration_days"))
	plan.VolumeGB, _ = strconv.Atoi(r.FormValue("volume_gb"))
	plan.PriceToman, _ = strconv.ParseInt(r.FormValue("price_toman"), 10, 64)
	if err := billing.ValidatePlanForAgent(plan, agent); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = s.Store.UpdatePlan(r.Context(), botID, plan)
	s.setFlash(w, "ok", "Plan updated")
	http.Redirect(w, r, "/agent/plans", http.StatusSeeOther)
}

func (s *Server) postAgentPlanDisable(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermManagePlans) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	planID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	botID, _ := strconv.ParseInt(r.FormValue("bot_id"), 10, 64)
	plan, err := s.Store.GetPlan(r.Context(), botID, planID)
	if err != nil || plan.AgentID != *admin.AgentID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	plan.Status = "disabled"
	_ = s.Store.UpdatePlan(r.Context(), botID, plan)
	s.setFlash(w, "ok", "Plan disabled")
	http.Redirect(w, r, "/agent/plans", http.StatusSeeOther)
}

func (s *Server) pageAgentBots(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bots, _ := s.Store.ListBotsByAgent(r.Context(), *admin.AgentID)
	s.renderPage(w, "agent_bots", r, map[string]any{"Bots": bots})
}

func (s *Server) postAgentBot(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil || !s.canPerm(r, admin, db.PermManageBot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agent, _ := s.Store.GetAgent(r.Context(), *admin.AgentID)
	count, _ := s.Store.CountBotsByAgent(r.Context(), *admin.AgentID)
	if int(count) >= agent.MaxBots {
		http.Error(w, "max bots reached", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	if err := s.postBotWithToken(r, *admin.AgentID); err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, "/agent/bots", http.StatusSeeOther)
		return
	}
	s.setFlash(w, "ok", "Bot added")
	http.Redirect(w, r, "/agent/bots", http.StatusSeeOther)
}

func (s *Server) pageAgentBotSettings(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.canAccessBot(r, admin, id) || !s.canPerm(r, admin, db.PermManageBot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bot, err := s.Store.GetBot(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	whURL, whStatus := s.Telegram.WebhookInfo(r.Context(), id)
	s.renderPage(w, "agent_bot_settings", r, map[string]any{
		"Bot": bot, "Settings": s.botSettingsMap(r, id),
		"WebhookURL": whURL, "WebhookStatus": whStatus,
	})
}

func (s *Server) postAgentBotSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if !s.canAccessBot(r, admin, id) || !s.canPerm(r, admin, db.PermManageBot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	if token := r.FormValue("token"); token != "" {
		username, err := telegram.ValidateToken(r.Context(), s.Box, token, s.Telegram.HTTPClient())
		if err != nil {
			s.setFlash(w, "err", err.Error())
			http.Redirect(w, r, fmt.Sprintf("/agent/bots/%d/settings", id), http.StatusSeeOther)
			return
		}
		bot, _ := s.Store.GetBot(r.Context(), id)
		enc, _ := s.Box.Encrypt(strings.TrimSpace(token))
		bot.TokenEnc = enc
		bot.Username = username
		_ = s.Store.UpdateBot(r.Context(), bot)
		_ = s.Telegram.Reload(r.Context(), id)
	}
	s.saveBotSettings(r, id)
	s.setFlash(w, "ok", "Settings saved")
	http.Redirect(w, r, fmt.Sprintf("/agent/bots/%d/settings", id), http.StatusSeeOther)
}

func (s *Server) pageAgentPanels(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentPanels, _ := s.Store.ListAgentPanels(r.Context(), *admin.AgentID)
	type row struct {
		AgentPanel db.AgentPanel
		Panel      db.Panel
		Usage      int64
	}
	var rows []row
	for _, ap := range agentPanels {
		p, err := s.Store.GetPanel(r.Context(), ap.PanelID)
		if err != nil {
			continue
		}
		n, _ := s.Store.CountServicesByAgentPanel(r.Context(), *admin.AgentID, ap.PanelID)
		rows = append(rows, row{AgentPanel: ap, Panel: *p, Usage: n})
	}
	s.renderPage(w, "agent_panels", r, map[string]any{"Rows": rows})
}
