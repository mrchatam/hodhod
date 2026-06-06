package miniapp

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/botconfig"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/provisioning"
)

// API serves Mini App JSON endpoints.
type API struct {
	Store  *db.Store
	Box    *crypto.Box
	Orders *billing.OrderService
	Wallet *billing.WalletService
	Prov   *provisioning.Service
	Reader *botconfig.Reader
}

// Routes mounts miniapp routes on r.
func (a *API) Routes(r chi.Router) {
	r.Route("/api/miniapp/{publicID}", func(r chi.Router) {
		r.Get("/plans", a.plans)
		r.Get("/wallet", a.wallet)
		r.Get("/wallet/transactions", a.walletTransactions)
		r.Get("/services", a.services)
		r.Post("/orders", a.createOrder)
		r.Post("/orders/card", a.createCardOrder)
		r.Post("/wallet/topup", a.createTopUp)
		r.Post("/trial", a.createTrial)
	})
}

func (a *API) auth(r *http.Request) (botID int64, tgID int64, user *db.EndUser, err error) {
	publicID := chi.URLParam(r, "publicID")
	initData := r.Header.Get("X-Telegram-Init-Data")
	if initData == "" {
		initData = r.URL.Query().Get("initData")
	}
	bot, err := a.Store.GetBotByPublicID(r.Context(), publicID)
	if err != nil {
		return 0, 0, nil, err
	}
	token, err := a.Box.Decrypt(bot.TokenEnc)
	if err != nil {
		return 0, 0, nil, err
	}
	tgID, err = ValidateInitData(initData, token, 24*time.Hour)
	if err != nil {
		return 0, 0, nil, err
	}
	user, err = a.Store.GetOrCreateEndUser(r.Context(), bot.ID, tgID)
	if err != nil {
		return 0, 0, nil, err
	}
	return bot.ID, tgID, user, nil
}

func (a *API) plans(w http.ResponseWriter, r *http.Request) {
	botID, _, _, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	plans, err := a.Store.ListPlansByBot(r.Context(), botID, true)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plans)
}

func (a *API) wallet(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = botID
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int64{"balance_toman": user.BalanceToman})
}

func (a *API) services(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	svcs, err := a.Store.ListServicesByUser(r.Context(), botID, user.ID)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(svcs)
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	plan, err := a.Store.GetPlan(r.Context(), botID, body.PlanID)
	if err != nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}
	if user.BalanceToman < plan.PriceToman {
		http.Error(w, "insufficient balance", http.StatusPaymentRequired)
		return
	}
	order, err := a.Orders.PurchaseFromWallet(r.Context(), botID, user, plan)
	if err != nil {
		http.Error(w, "purchase failed", http.StatusInternalServerError)
		return
	}
	svc, err := a.Prov.ProvisionOrder(r.Context(), botID, order)
	if err != nil {
		_ = a.Wallet.Credit(r.Context(), botID, user.ID, plan.PriceToman, "provision_refund", "order", order.ID)
		order.Status = db.OrderRejected
		_ = a.Store.UpdateOrder(r.Context(), botID, order)
		http.Error(w, "provisioning failed", http.StatusBadGateway)
		return
	}
	_ = a.Orders.MarkOrderProvisioned(r.Context(), botID, order.ID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"order_id": order.ID,
		"service":  svc,
	})
}

func (a *API) createTopUp(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Amount     int64  `json:"amount_toman"`
		ReceiptRef string `json:"receipt_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Amount < 1000 || body.ReceiptRef == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payment, err := a.Orders.CreateTopUpPayment(r.Context(), botID, user.ID, body.Amount, body.ReceiptRef)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payment)
}

func (a *API) createCardOrder(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		PlanID     int64  `json:"plan_id"`
		ReceiptRef string `json:"receipt_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanID == 0 || body.ReceiptRef == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	plan, err := a.Store.GetPlan(r.Context(), botID, body.PlanID)
	if err != nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}
	payment, err := a.Orders.CreatePlanOrderPayment(r.Context(), botID, user, plan, body.ReceiptRef)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payment)
}

func (a *API) walletTransactions(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	txs, err := a.Store.ListWalletTxByUser(r.Context(), botID, user.ID, 20)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(txs)
}

func (a *API) createTrial(w http.ResponseWriter, r *http.Request) {
	botID, _, user, err := a.auth(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if a.Reader != nil && a.Reader.Setting(r.Context(), botID, "trial_enabled") != "true" {
		http.Error(w, "trial disabled", http.StatusForbidden)
		return
	}
	if user.Status == "blocked" {
		http.Error(w, "user blocked", http.StatusForbidden)
		return
	}
	maxPerUser := int64(1)
	if a.Reader != nil {
		maxPerUser = botconfig.ParseInt64(a.Reader.Setting(r.Context(), botID, "trial_max_per_user"), 1)
	}
	if user.TrialCount >= int(maxPerUser) || user.TrialUsedAt != nil {
		http.Error(w, "trial already used", http.StatusConflict)
		return
	}
	plans, err := a.Store.ListPlansByBot(r.Context(), botID, true)
	if err != nil || len(plans) == 0 {
		http.Error(w, "no trial plan", http.StatusNotFound)
		return
	}
	plan := plans[0]
	for _, p := range plans {
		if p.IsTrial {
			plan = p
			break
		}
	}
	now := time.Now()
	order := &db.Order{
		BotID: botID, EndUserID: user.ID, PlanID: plan.ID,
		Status: db.OrderApproved, PriceToman: plan.PriceToman,
		IsTrial: true, PaymentMethod: "trial", ApprovedAt: &now,
	}
	if err := a.Store.CreateOrder(r.Context(), order); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	svc, err := a.Prov.ProvisionOrder(r.Context(), botID, order)
	if err != nil {
		http.Error(w, "provisioning failed", http.StatusBadGateway)
		return
	}
	now = time.Now()
	user.TrialUsedAt = &now
	user.TrialCount++
	_ = a.Store.UpdateEndUser(r.Context(), botID, user)
	_ = a.Orders.MarkOrderProvisioned(r.Context(), botID, order.ID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"service": svc})
}
