package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"github.com/mrchatam/hodhod/internal/billing"
	"github.com/mrchatam/hodhod/internal/config"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
	"github.com/mrchatam/hodhod/internal/provisioning"
)

// Manager runs multiple Telegram bots via webhooks.
type Manager struct {
	cfg         *config.Config
	box         *crypto.Box
	store       *db.Store
	http        *http.Client
	orders      *billing.OrderService
	wallet      *billing.WalletService
	prov        *provisioning.Service
	mu          sync.RWMutex
	bots        map[string]*managedBot
	handlers    *Handlers
	buyMu       sync.Map
	topUpMu     sync.Map
	seenMu      sync.Mutex
	seenUpdates map[string]struct{}
	states      stateStore
}

type managedBot struct {
	record *db.Bot
	api    *bot.Bot
}

// NewManager creates a bot manager.
func NewManager(
	cfg *config.Config,
	box *crypto.Box,
	store *db.Store,
	httpClient *http.Client,
	orders *billing.OrderService,
	wallet *billing.WalletService,
	prov *provisioning.Service,
) *Manager {
	m := &Manager{
		cfg: cfg, box: box, store: store, http: httpClient,
		orders: orders, wallet: wallet, prov: prov,
		bots:        make(map[string]*managedBot),
		seenUpdates: make(map[string]struct{}),
	}
	m.handlers = &Handlers{mgr: m}
	return m
}

// LoadActive loads all active bots from DB.
func (m *Manager) LoadActive(ctx context.Context) error {
	list, err := m.store.ListActiveBots(ctx)
	if err != nil {
		return err
	}
	for i := range list {
		if err := m.Add(ctx, list[i].ID); err != nil {
			slog.Warn("bot load failed", "bot_id", list[i].ID, "err", err)
		}
	}
	return nil
}

// Add registers a bot and sets its webhook.
func (m *Manager) Add(ctx context.Context, botID int64) error {
	rec, err := m.store.GetBot(ctx, botID)
	if err != nil {
		return err
	}
	token, err := m.box.Decrypt(rec.TokenEnc)
	if err != nil {
		return err
	}
	opts := []bot.Option{
		bot.WithHTTPClient(15, m.http),
	}
	api, err := bot.New(token, opts...)
	if err != nil {
		return err
	}
	mb := &managedBot{record: rec, api: api}
	m.mu.Lock()
	m.bots[rec.PublicID] = mb
	m.mu.Unlock()
	whURL := fmt.Sprintf("%s/wh/tg/%s", m.cfg.PublicBaseURL, rec.PublicID)
	_, err = api.SetWebhook(ctx, &bot.SetWebhookParams{
		URL:                whURL,
		SecretToken:        rec.WebhookSecret,
		DropPendingUpdates: false,
	})
	if err != nil {
		slog.Warn("setWebhook failed, will retry", "public_id", rec.PublicID, "err", err)
		go m.retrySetWebhook(rec.PublicID, api, whURL, rec.WebhookSecret)
	} else {
		slog.Info("bot registered", "public_id", rec.PublicID, "username", rec.Username)
	}
	return nil
}

func (m *Manager) retrySetWebhook(publicID string, api *bot.Bot, whURL, secret string) {
	for attempt := 1; attempt <= 5; attempt++ {
		time.Sleep(time.Duration(attempt*attempt) * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, err := api.SetWebhook(ctx, &bot.SetWebhookParams{
			URL: whURL, SecretToken: secret,
		})
		cancel()
		if err == nil {
			slog.Info("setWebhook succeeded on retry", "public_id", publicID, "attempt", attempt)
			return
		}
		slog.Warn("setWebhook retry", "public_id", publicID, "attempt", attempt, "err", err)
	}
}

// Remove drops a bot from memory.
func (m *Manager) Remove(publicID string) {
	m.mu.Lock()
	delete(m.bots, publicID)
	m.mu.Unlock()
}

// Reload refreshes a bot from DB.
func (m *Manager) Reload(ctx context.Context, botID int64) error {
	rec, err := m.store.GetBot(ctx, botID)
	if err != nil {
		return err
	}
	m.Remove(rec.PublicID)
	return m.Add(ctx, botID)
}

// SecretFor returns webhook secret for public ID, falling back to DB.
func (m *Manager) SecretFor(ctx context.Context, publicID string) (string, bool) {
	m.mu.RLock()
	mb, ok := m.bots[publicID]
	m.mu.RUnlock()
	if ok {
		return mb.record.WebhookSecret, true
	}
	rec, err := m.store.GetBotByPublicID(ctx, publicID)
	if err != nil {
		return "", false
	}
	return rec.WebhookSecret, true
}

// Dispatch handles a webhook update JSON body.
func (m *Manager) Dispatch(ctx context.Context, publicID string, body []byte) error {
	m.mu.RLock()
	mb, ok := m.bots[publicID]
	m.mu.RUnlock()
	if !ok {
		rec, err := m.store.GetBotByPublicID(ctx, publicID)
		if err != nil {
			return err
		}
		if err := m.Add(ctx, rec.ID); err != nil {
			return err
		}
		m.mu.RLock()
		mb = m.bots[publicID]
		m.mu.RUnlock()
	}
	var upd models.Update
	if err := json.Unmarshal(body, &upd); err != nil {
		return err
	}
	if upd.ID != 0 {
		key := fmt.Sprintf("%s:%d", publicID, upd.ID)
		if !m.markUpdateSeen(key) {
			return nil
		}
	}
	return m.handlers.handle(ctx, mb, &upd)
}

func (m *Manager) markUpdateSeen(key string) bool {
	m.seenMu.Lock()
	defer m.seenMu.Unlock()
	if _, ok := m.seenUpdates[key]; ok {
		return false
	}
	m.seenUpdates[key] = struct{}{}
	if len(m.seenUpdates) > 5000 {
		// Simple in-memory cap; old entries are dropped in bulk.
		m.seenUpdates = make(map[string]struct{}, 2500)
	}
	return true
}

func (m *Manager) userBuyLock(userID int64) *sync.Mutex {
	lock, _ := m.buyMu.LoadOrStore(userID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// CreateBotRecord creates DB record for a new bot (master use).
func CreateBotRecord(ctx context.Context, store *db.Store, box *crypto.Box, agentID int64, token, username string) (*db.Bot, error) {
	enc, err := box.Encrypt(token)
	if err != nil {
		return nil, err
	}
	b := &db.Bot{
		AgentID:       agentID,
		PublicID:      uuid.New().String(),
		Username:      username,
		TokenEnc:      enc,
		WebhookSecret: uuid.New().String(),
		Status:        "active",
	}
	return b, store.CreateBot(ctx, b)
}

// SendMessage sends a text message to a telegram user.
func (m *Manager) SendMessage(ctx context.Context, publicID string, chatID int64, text string) error {
	m.mu.RLock()
	mb, ok := m.bots[publicID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("bot not loaded")
	}
	_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

// Handlers processes updates.
type Handlers struct{ mgr *Manager }

func (h *Handlers) handle(ctx context.Context, mb *managedBot, upd *models.Update) error {
	if upd.Message == nil {
		return h.handleCallback(ctx, mb, upd)
	}
	msg := upd.Message
	if msg.From == nil {
		return nil
	}
	user, err := h.mgr.store.GetOrCreateEndUser(ctx, mb.record.ID, msg.From.ID)
	if err != nil {
		return err
	}
	key := stateKey(mb.record.ID, user.ID)
	st := h.mgr.states.get(key)

	if len(msg.Photo) > 0 && st.Step == "topup_receipt" {
		return h.finishTopUp(ctx, mb, user, msg.Chat.ID, st, msg.Photo[len(msg.Photo)-1].FileID)
	}

	text := strings.TrimSpace(msg.Text)
	if text == "/start" {
		h.mgr.states.clear(key)
		return h.sendMainMenu(ctx, mb, user, msg.Chat.ID)
	}

	if st.Step == "topup_amount" {
		var amount int64
		if _, err := fmt.Sscanf(text, "%d", &amount); err != nil || amount < 1000 {
			_, _ = mb.api.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID, Text: i18n.T(user.Lang, "topup_invalid_amount"),
			})
			return nil
		}
		h.mgr.states.set(key, userState{Step: "topup_receipt", Amount: amount})
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID, Text: i18n.T(user.Lang, "topup_send_receipt"),
		})
		return err
	}

	if text != "" {
		return h.sendMainMenu(ctx, mb, user, msg.Chat.ID)
	}
	return nil
}

func (h *Handlers) handleCallback(ctx context.Context, mb *managedBot, upd *models.Update) error {
	if upd.CallbackQuery == nil {
		return nil
	}
	cq := upd.CallbackQuery
	_, _ = mb.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cq.ID})
	if cq.From.ID == 0 {
		return nil
	}
	user, err := h.mgr.store.GetOrCreateEndUser(ctx, mb.record.ID, cq.From.ID)
	if err != nil {
		return err
	}
	chatID := cq.From.ID
	if cq.Message.Message != nil {
		chatID = cq.Message.Message.Chat.ID
	}
	switch cq.Data {
	case "menu":
		h.mgr.states.clear(stateKey(mb.record.ID, user.ID))
		return h.sendMainMenu(ctx, mb, user, chatID)
	case "wallet":
		text := i18n.T(user.Lang, "wallet_balance", user.BalanceToman)
		kb := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: i18n.T(user.Lang, "btn_topup"), CallbackData: "topup"}},
			{{Text: "«", CallbackData: "menu"}},
		}}
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: kb})
		return err
	case "topup":
		h.mgr.states.set(stateKey(mb.record.ID, user.ID), userState{Step: "topup_amount"})
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "topup_enter_amount"),
		})
		return err
	case "support":
		support, _ := h.mgr.store.GetSetting(ctx, "bot", mb.record.ID, "support_contact")
		if support == "" {
			support, _ = h.mgr.store.GetSetting(ctx, "master", 0, "support_contact")
		}
		if support == "" {
			support = "-"
		}
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "support_text", support),
		})
		return err
	case "lang":
		kb := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "فارسی", CallbackData: "lang:fa"}, {Text: "English", CallbackData: "lang:en"}},
		}}
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "btn_language"), ReplyMarkup: kb,
		})
		return err
	case "plans":
		return h.sendPlans(ctx, mb, user, chatID)
	case "services":
		return h.sendServices(ctx, mb, user, chatID)
	}
	if strings.HasPrefix(cq.Data, "lang:") {
		lang := strings.TrimPrefix(cq.Data, "lang:")
		if lang == "fa" || lang == "en" {
			user.Lang = lang
			_ = h.mgr.store.UpdateEndUser(ctx, mb.record.ID, user)
		}
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "lang_switched"),
		})
		return err
	}
	if len(cq.Data) > 5 && cq.Data[:5] == "plan:" {
		var planID int64
		fmt.Sscanf(cq.Data, "plan:%d", &planID)
		return h.buyPlan(ctx, mb, user, chatID, planID)
	}
	return nil
}

func (h *Handlers) sendMainMenu(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64) error {
	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: i18n.T(user.Lang, "btn_buy"), CallbackData: "plans"}},
			{{Text: i18n.T(user.Lang, "btn_services"), CallbackData: "services"}},
			{{Text: i18n.T(user.Lang, "btn_wallet"), CallbackData: "wallet"}},
			{{Text: i18n.T(user.Lang, "btn_support"), CallbackData: "support"}, {Text: i18n.T(user.Lang, "btn_language"), CallbackData: "lang"}},
		},
	}
	_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        i18n.T(user.Lang, "start_welcome") + "\n" + i18n.T(user.Lang, "main_menu"),
		ReplyMarkup: kb,
	})
	return err
}

func (h *Handlers) sendPlans(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64) error {
	plans, err := h.mgr.store.ListPlansByBot(ctx, mb.record.ID, true)
	if err != nil {
		return err
	}
	var rows [][]models.InlineKeyboardButton
	text := i18n.T(user.Lang, "plan_list") + "\n"
	for _, p := range plans {
		text += i18n.T(user.Lang, "plan_item", p.Name, p.VolumeGB, p.DurationDays, p.PriceToman) + "\n"
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: p.Name, CallbackData: fmt.Sprintf("plan:%d", p.ID)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "«", CallbackData: "menu"}})
	_, err = mb.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID, Text: text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func (h *Handlers) sendServices(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64) error {
	svcs, err := h.mgr.store.ListServicesByUser(ctx, mb.record.ID, user.ID)
	if err != nil {
		return err
	}
	text := ""
	for _, s := range svcs {
		text += fmt.Sprintf("%s\n%s\n\n", s.PanelUsername, s.SubLink)
	}
	if text == "" {
		text = "-"
	}
	_, err = mb.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	return err
}

func (h *Handlers) buyPlan(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64, planID int64) error {
	lock := h.mgr.userBuyLock(user.ID)
	lock.Lock()
	defer lock.Unlock()

	latestUser, err := h.mgr.store.GetEndUser(ctx, mb.record.ID, user.ID)
	if err != nil {
		return err
	}
	user = latestUser

	plan, err := h.mgr.store.GetPlan(ctx, mb.record.ID, planID)
	if err != nil {
		return err
	}
	if user.BalanceToman < plan.PriceToman {
		kb := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: i18n.T(user.Lang, "btn_topup"), CallbackData: "topup"}},
		}}
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "insufficient"), ReplyMarkup: kb,
		})
		return err
	}
	order, err := h.mgr.orders.PurchaseFromWallet(ctx, mb.record.ID, user, plan)
	if err != nil {
		return err
	}
	svc, err := h.mgr.prov.ProvisionOrder(ctx, mb.record.ID, order)
	if err != nil {
		_ = h.mgr.wallet.Credit(ctx, mb.record.ID, user.ID, plan.PriceToman, "provision_refund", "order", order.ID)
		order.Status = db.OrderRejected
		_ = h.mgr.store.UpdateOrder(ctx, mb.record.ID, order)
		_, msgErr := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   i18n.T(user.Lang, "order_failed"),
		})
		if msgErr != nil {
			return msgErr
		}
		return err
	}
	_ = h.mgr.orders.MarkOrderProvisioned(ctx, mb.record.ID, order.ID)
	user, _ = h.mgr.store.GetEndUser(ctx, mb.record.ID, user.ID)
	text := i18n.T(user.Lang, "service_ready", svc.SubLink)
	_, err = mb.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	return err
}

func (h *Handlers) finishTopUp(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64, st userState, fileID string) error {
	if !h.mgr.allowTopUp(user.ID) {
		_, err := mb.api.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: i18n.T(user.Lang, "topup_invalid_amount"),
		})
		return err
	}
	key := stateKey(mb.record.ID, user.ID)
	h.mgr.states.clear(key)
	payment, err := h.mgr.orders.CreateTopUpPayment(ctx, mb.record.ID, user.ID, st.Amount, fileID)
	if err != nil {
		return err
	}
	_, err = mb.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID, Text: i18n.T(user.Lang, "topup_pending"),
	})
	if err != nil {
		return err
	}
	h.notifyApprover(ctx, mb, payment)
	return nil
}

func (m *Manager) allowTopUp(userID int64) bool {
	v, _ := m.topUpMu.LoadOrStore(userID, newTopUpLimiter())
	return v.(*topUpLimiter).allow()
}

type topUpLimiter struct {
	mu    sync.Mutex
	count int
	reset time.Time
}

func newTopUpLimiter() *topUpLimiter {
	return &topUpLimiter{count: 0, reset: time.Now().Add(time.Minute)}
}

func (l *topUpLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.reset) {
		l.count = 0
		l.reset = time.Now().Add(time.Minute)
	}
	if l.count >= 3 {
		return false
	}
	l.count++
	return true
}

func (h *Handlers) notifyApprover(ctx context.Context, mb *managedBot, payment *db.Payment) {
	botRec, err := h.mgr.store.GetBot(ctx, mb.record.ID)
	if err != nil {
		return
	}
	agent, err := h.mgr.store.GetAgent(ctx, botRec.AgentID)
	if err != nil || agent.TgAdminID == nil {
		return
	}
	text := fmt.Sprintf("New top-up pending: bot=%d payment=%d amount=%d", payment.BotID, payment.ID, payment.AmountToman)
	_ = h.mgr.SendMessage(ctx, mb.record.PublicID, *agent.TgAdminID, text)
}
