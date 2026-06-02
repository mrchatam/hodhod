package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/config"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
	"github.com/mrchatam/hodhod/internal/panels"
	"github.com/mrchatam/hodhod/internal/telegram"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the admin web GUI.
type Server struct {
	Cfg      *config.Config
	Store    *db.Store
	Box      *crypto.Box
	Panels   *panels.Registry
	Telegram *telegram.Manager
	tmpl     *template.Template
}

// NewServer creates a web server.
func NewServer(cfg *config.Config, store *db.Store, box *crypto.Box, reg *panels.Registry, tg *telegram.Manager) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{Cfg: cfg, Store: store, Box: box, Panels: reg, Telegram: tg, tmpl: tmpl}, nil
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.securityHeaders, RateLimitLogin)

	r.Get("/login", s.pageLogin)
	r.Post("/login", s.postLogin)
	r.Post("/logout", s.postLogout)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/", s.pageDashboard)
		r.Get("/payments/pending", s.pagePendingPayments)
		r.Post("/payments/{botID}/{id}/approve", s.approvePayment)
		r.Post("/payments/{botID}/{id}/reject", s.rejectPayment)

		r.Route("/master", func(r chi.Router) {
			r.Use(s.requireMaster)
			r.Get("/agents", s.pageAgents)
			r.Post("/agents", s.postAgent)
			r.Get("/panels", s.pagePanels)
			r.Post("/panels", s.postPanel)
			r.Post("/panels/{id}/test", s.testPanel)
			r.Get("/bots", s.pageBots)
			r.Post("/bots", s.postBot)
			r.Post("/bots/{id}/bot-panels", s.postBotPanel)
		})

		r.Route("/agent", func(r chi.Router) {
			r.Get("/plans", s.pageAgentPlans)
			r.Post("/plans", s.postAgentPlan)
			r.Get("/bots", s.pageAgentBots)
		})
	})
	return r
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self' https://cdn.tailwindcss.com; script-src 'self' https://cdn.tailwindcss.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) pageLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", nil)
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	admin, err := s.Store.AdminByUsername(r.Context(), r.FormValue("username"))
	if err != nil || !CheckPassword(admin.PasswordHash, r.FormValue("password")) {
		http.Redirect(w, r, "/login?e=1", http.StatusSeeOther)
		return
	}
	sid, err := s.createSession(r.Context(), admin)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: sid, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: !s.Cfg.IsDev(),
	})
	s.audit(r, &admin.ID, "login", "admin", admin.ID, map[string]any{"role": admin.Role})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}
	if admin, _, ok := s.sessionAdmin(r); ok {
		s.audit(r, &admin.ID, "logout", "admin", admin.ID, nil)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1, Secure: !s.Cfg.IsDev()})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) pageDashboard(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	pending, _ := s.Store.CountPendingPaymentsAll(r.Context())
	active, _ := s.Store.CountActiveServices(r.Context())
	s.render(w, "dashboard.html", map[string]any{
		"Admin": admin, "Pending": pending, "ActiveServices": active,
		"CSRF": r.Context().Value(ctxCSRF),
	})
}

func (s *Server) pagePendingPayments(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	var payments []db.Payment
	if admin.Role == db.RoleMaster {
		payments, _ = s.Store.ListAllPendingPayments(r.Context())
	}
	if admin.Role == db.RoleAgent && admin.AgentID != nil {
		payments, _ = s.Store.ListPendingPaymentsByAgent(r.Context(), *admin.AgentID)
	}
	s.render(w, "payments.html", map[string]any{"Payments": payments, "CSRF": r.Context().Value(ctxCSRF)})
}

func (s *Server) approvePayment(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	botID, _ := strconv.ParseInt(chi.URLParam(r, "botID"), 10, 64)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.canAccessBot(r, admin, botID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.ApprovePayment(r.Context(), botID, id, admin.ID)
	s.audit(r, &admin.ID, "approve_payment", "payment", id, map[string]any{"bot_id": botID})
	s.notifyPaymentReviewed(r, botID, id)
	http.Redirect(w, r, "/payments/pending", http.StatusSeeOther)
}

func (s *Server) rejectPayment(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	botID, _ := strconv.ParseInt(chi.URLParam(r, "botID"), 10, 64)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.canAccessBot(r, admin, botID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_ = s.Store.RejectPayment(r.Context(), botID, id, admin.ID)
	s.audit(r, &admin.ID, "reject_payment", "payment", id, map[string]any{"bot_id": botID})
	s.notifyPaymentReviewed(r, botID, id)
	http.Redirect(w, r, "/payments/pending", http.StatusSeeOther)
}

func (s *Server) pageAgents(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.Store.ListAgents(r.Context())
	s.render(w, "agents.html", map[string]any{"Agents": agents, "CSRF": r.Context().Value(ctxCSRF)})
}

func (s *Server) postAgent(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	a := &db.Agent{Name: r.FormValue("name"), Status: db.AgentActive}
	_ = s.Store.CreateAgent(r.Context(), a)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	s.audit(r, &admin.ID, "create_agent", "agent", a.ID, map[string]any{"name": a.Name})
	http.Redirect(w, r, "/master/agents", http.StatusSeeOther)
}

func (s *Server) pagePanels(w http.ResponseWriter, r *http.Request) {
	panels, _ := s.Store.ListPanels(r.Context())
	s.render(w, "panels.html", map[string]any{"Panels": panels, "CSRF": r.Context().Value(ctxCSRF)})
}

func (s *Server) postPanel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	enc, _ := s.Box.Encrypt(r.FormValue("password"))
	apiTokenEnc := ""
	if token := r.FormValue("api_token"); token != "" {
		apiTokenEnc, _ = s.Box.Encrypt(token)
	}
	p := &db.Panel{
		Type:        db.PanelType(r.FormValue("type")),
		Name:        r.FormValue("name"),
		BaseURL:     r.FormValue("base_url"),
		BasePath:    r.FormValue("base_path"),
		Username:    r.FormValue("username"),
		PasswordEnc: enc,
		APITokenEnc: apiTokenEnc,
		Status:      "active",
	}
	_ = s.Store.CreatePanel(r.Context(), p)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	s.audit(r, &admin.ID, "create_panel", "panel", p.ID, map[string]any{"name": p.Name, "type": p.Type})
	s.Panels.Invalidate(p.ID)
	http.Redirect(w, r, "/master/panels", http.StatusSeeOther)
}

func (s *Server) testPanel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := s.Panels.TestConnection(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Write([]byte("ok"))
}

func (s *Server) pageBots(w http.ResponseWriter, r *http.Request) {
	agents, _ := s.Store.ListAgents(r.Context())
	var all []db.Bot
	for _, a := range agents {
		bots, _ := s.Store.ListBotsByAgent(r.Context(), a.ID)
		all = append(all, bots...)
	}
	s.render(w, "bots.html", map[string]any{"Bots": all, "Agents": agents, "CSRF": r.Context().Value(ctxCSRF)})
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
	token := r.FormValue("token")
	username := r.FormValue("username")
	b, err := telegram.CreateBotRecord(r.Context(), s.Store, s.Box, agentID, token, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = s.Telegram.Add(r.Context(), b.ID)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	s.audit(r, &admin.ID, "create_bot", "bot", b.ID, map[string]any{"agent_id": agentID})
	http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
}

func (s *Server) postBotPanel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	botID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	panelID, _ := strconv.ParseInt(r.FormValue("panel_id"), 10, 64)
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if !s.canAccessBot(r, admin, botID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bp := &db.BotPanel{BotID: botID, PanelID: panelID, ScopeJSON: []byte(`{}`)}
	_ = s.Store.UpsertBotPanel(r.Context(), bp)
	s.audit(r, &admin.ID, "assign_bot_panel", "bot", botID, map[string]any{"panel_id": panelID})
	http.Redirect(w, r, "/master/bots", http.StatusSeeOther)
}

func (s *Server) pageAgentPlans(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bots, _ := s.Store.ListBotsByAgent(r.Context(), *admin.AgentID)
	var plans []db.Plan
	type panelOption struct {
		BotID     int64
		PanelID   int64
		PanelName string
	}
	var panelOptions []panelOption
	for _, b := range bots {
		ps, _ := s.Store.ListPlansByBot(r.Context(), b.ID, false)
		plans = append(plans, ps...)
		panels, _ := s.Store.ListPanelsForBot(r.Context(), b.ID)
		for _, p := range panels {
			panelOptions = append(panelOptions, panelOption{
				BotID: b.ID, PanelID: p.ID, PanelName: p.Name,
			})
		}
	}
	s.render(w, "agent_plans.html", map[string]any{
		"Plans": plans, "Bots": bots, "PanelOptions": panelOptions, "CSRF": r.Context().Value(ctxCSRF),
	})
}

func (s *Server) postAgentPlan(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
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
	s.audit(r, &admin.ID, "create_plan", "plan", plan.ID, map[string]any{"bot_id": botID, "panel_id": panelID})
	http.Redirect(w, r, "/agent/plans", http.StatusSeeOther)
}

func (s *Server) pageAgentBots(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	if admin.AgentID == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bots, _ := s.Store.ListBotsByAgent(r.Context(), *admin.AgentID)
	s.render(w, "agent_bots.html", map[string]any{"Bots": bots})
}

func (s *Server) canAccessBot(r *http.Request, admin *db.Admin, botID int64) bool {
	if admin.Role == db.RoleMaster {
		return true
	}
	if admin.AgentID == nil {
		return false
	}
	_, err := s.Store.GetBotForAgent(r.Context(), *admin.AgentID, botID)
	return err == nil
}

func (s *Server) notifyPaymentReviewed(r *http.Request, botID, paymentID int64) {
	p, err := s.Store.GetPayment(r.Context(), botID, paymentID)
	if err != nil {
		return
	}
	u, err := s.Store.GetEndUser(r.Context(), botID, p.EndUserID)
	if err != nil {
		return
	}
	botRec, err := s.Store.GetBot(r.Context(), botID)
	if err != nil {
		return
	}
	msg := i18n.T(u.Lang, "topup_pending")
	if p.Status == db.PaymentApproved {
		msg = i18n.T(u.Lang, "topup_approved")
	}
	if p.Status == db.PaymentRejected {
		msg = i18n.T(u.Lang, "order_failed")
	}
	_ = s.Telegram.SendMessage(r.Context(), botRec.PublicID, u.TelegramID, msg)
}

func (s *Server) audit(r *http.Request, adminID *int64, action, entityType string, entityID int64, detail map[string]any) {
	payload := []byte("{}")
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			payload = b
		}
	}
	_ = s.Store.Audit(r.Context(), adminID, action, entityType, entityID, payload)
}
