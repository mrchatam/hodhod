package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
	"github.com/mrchatam/hodhod/internal/telegram"
)

func (s *Server) pageLogin(w http.ResponseWriter, r *http.Request) {
	lang := "fa"
	if c, err := r.Cookie("hodhod_lang"); err == nil && (c.Value == "en" || c.Value == "fa") {
		lang = c.Value
	}
	data := map[string]any{"Error": r.URL.Query().Get("e") != "", "Lang": lang, "IsRTL": lang == "fa"}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.loginT.ExecuteTemplate(w, "login.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	admin, err := s.Store.AdminByUsername(r.Context(), r.FormValue("username"))
	if err != nil || !CheckPassword(admin.PasswordHash, r.FormValue("password")) {
		http.Redirect(w, r, "/login?e=1", http.StatusSeeOther)
		return
	}
	if admin.Role == db.RoleAgent && admin.AgentID != nil {
		agent, err := s.Store.GetAgent(r.Context(), *admin.AgentID)
		if err != nil || agent.Status == db.AgentDisabled {
			http.Redirect(w, r, "/login?e=disabled", http.StatusSeeOther)
			return
		}
	}
	if hk, _ := r.Context().Value(ctxHostKind).(hostKind); hk == hostAgent {
		hostAgentID, _ := r.Context().Value(ctxHostAgentID).(int64)
		if admin.Role != db.RoleAgent || admin.AgentID == nil || *admin.AgentID != hostAgentID {
			http.Redirect(w, r, s.Cfg.PublicBaseURL+"/login?e=wrong_host", http.StatusSeeOther)
			return
		}
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
	extra := map[string]any{"Pending": pending, "ActiveServices": active}
	if admin.Role == db.RoleMaster {
		extra["AgentCount"], _ = s.Store.CountAgents(r.Context())
		extra["BotCount"], _ = s.Store.CountBots(r.Context())
		type stat struct {
			Name, Status string
			Services     int64
			Revenue      int64
		}
		var stats []stat
		agents, _ := s.Store.ListAgents(r.Context())
		for _, a := range agents {
			svc, _ := s.Store.CountServicesByAgent(r.Context(), a.ID)
			rev, _ := s.Store.SumApprovedPaymentsByAgent(r.Context(), a.ID)
			stats = append(stats, stat{Name: a.Name, Services: svc, Revenue: rev})
		}
		extra["AgentStats"] = stats
		extra["PanelHealth"] = s.panelHealthRows(r.Context())
	} else if admin.AgentID != nil {
		n, _ := s.Store.CountServicesByAgent(r.Context(), *admin.AgentID)
		extra["MyServices"] = n
	}
	s.renderPage(w, "dashboard", r, extra)
}

func (s *Server) pageOnboarding(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	page := "onboarding_agent"
	if admin.Role == db.RoleMaster {
		page = "onboarding_master"
	}
	s.renderPage(w, page, r, nil)
}

func (s *Server) approvePayment(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	botID, _ := strconv.ParseInt(chi.URLParam(r, "botID"), 10, 64)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.canAccessBot(r, admin, botID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_, err := s.Review.ApprovePaymentAndProvision(r.Context(), botID, id, admin.ID)
	if err != nil {
		s.setFlash(w, "err", err.Error())
		http.Redirect(w, r, "/payments/pending", http.StatusSeeOther)
		return
	}
	s.audit(r, &admin.ID, "approve_payment", "payment", id, map[string]any{"bot_id": botID})
	s.notifyPaymentReviewed(r, botID, id)
	s.setFlash(w, "ok", "Payment approved")
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
	_ = s.Review.RejectPayment(r.Context(), botID, id, admin.ID)
	s.audit(r, &admin.ID, "reject_payment", "payment", id, map[string]any{"bot_id": botID})
	s.notifyPaymentReviewed(r, botID, id)
	s.setFlash(w, "ok", "Payment rejected")
	http.Redirect(w, r, "/payments/pending", http.StatusSeeOther)
}

func (s *Server) pagePendingPayments(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	histPage, _ := strconv.Atoi(r.URL.Query().Get("hist_page"))
	if histPage < 1 {
		histPage = 1
	}
	histPerPage := 25
	if v, _ := strconv.Atoi(r.URL.Query().Get("per_page")); validPerPage(v) {
		histPerPage = v
	}
	filter := paymentFilterFromRequest(r)
	isMaster := admin.Role == db.RoleMaster
	agentID := int64(0)
	if admin.AgentID != nil {
		agentID = *admin.AgentID
	}
	payments, _, _ := s.Store.ListPendingPaymentsFiltered(r.Context(), agentID, isMaster, filter, 0, 0)
	history, histTotal, _ := s.Store.ListPaymentsHistoryFiltered(r.Context(), agentID, isMaster, filter, histPerPage, (histPage-1)*histPerPage)
	histPag := Pagination{
		Page: histPage, PerPage: histPerPage, Total: int(histTotal),
		Base: r.URL.Path, PageParam: "hist_page",
	}
	var bots []db.Bot
	if isMaster {
		agents, _ := s.Store.ListAgents(r.Context())
		for _, a := range agents {
			list, _ := s.Store.ListBotsByAgent(r.Context(), a.ID)
			bots = append(bots, list...)
		}
	} else if agentID > 0 {
		bots, _ = s.Store.ListBotsByAgent(r.Context(), agentID)
	}
	s.renderPage(w, "payments", r, map[string]any{
		"Payments": payments, "History": history, "Currency": "Toman", "HistPagination": histPag,
		"Filter": filter, "Bots": bots,
	})
}

func paymentFilterFromRequest(r *http.Request) db.PaymentFilter {
	f := db.PaymentFilter{}
	if v, _ := strconv.ParseInt(r.URL.Query().Get("bot_id"), 10, 64); v > 0 {
		f.BotID = v
	}
	if st := r.URL.Query().Get("status"); st != "" {
		f.Status = db.PaymentStatus(st)
	}
	if v := r.URL.Query().Get("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			f.DateFrom = &t
		}
	}
	if v := r.URL.Query().Get("date_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			f.DateTo = &end
		}
	}
	return f
}

func (s *Server) getPaymentReceipt(w http.ResponseWriter, r *http.Request) {
	admin := r.Context().Value(ctxAdmin).(*db.Admin)
	botID, _ := strconv.ParseInt(chi.URLParam(r, "botID"), 10, 64)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.canAccessBot(r, admin, botID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p, err := s.Store.GetPayment(r.Context(), botID, id)
	if err != nil || p.ReceiptRef == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	data, ctype, err := s.Telegram.FetchFile(r.Context(), botID, p.ReceiptRef)
	if err != nil {
		http.Error(w, "receipt unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Write(data)
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
	msg := paymentReviewMessage(r.Context(), s.Store, u.Lang, botID, *p)
	_ = s.Telegram.SendMessage(r.Context(), botRec.PublicID, u.TelegramID, msg)
}

func paymentReviewMessage(ctx context.Context, store *db.Store, lang string, botID int64, p db.Payment) string {
	if p.Status == db.PaymentRejected {
		return i18n.T(lang, "order_failed")
	}
	if p.Status == db.PaymentApproved {
		if p.OrderID != nil && *p.OrderID > 0 {
			if svc, err := store.GetServiceByOrderID(ctx, botID, *p.OrderID); err == nil && svc.SubLink != "" {
				return i18n.T(lang, "service_ready", svc.SubLink)
			}
		}
		return i18n.T(lang, "topup_approved")
	}
	return i18n.T(lang, "topup_pending")
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

func (s *Server) postBotWithToken(r *http.Request, agentID int64) error {
	token := strings.TrimSpace(r.FormValue("token"))
	username, err := telegram.ValidateToken(r.Context(), s.Box, token, s.Telegram.HTTPClient())
	if err != nil {
		return err
	}
	b, err := telegram.CreateBotRecord(r.Context(), s.Store, s.Box, agentID, token, username)
	if err != nil {
		return err
	}
	if v := strings.TrimSpace(r.FormValue("approver_tg_id")); v != "" {
		_ = s.Store.SetSetting(r.Context(), "bot", b.ID, "approver_tg_id", v)
	}
	return s.Telegram.Add(r.Context(), b.ID)
}
