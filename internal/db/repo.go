package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store wraps GORM with repository methods.
type Store struct {
	DB *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{DB: db} }

// --- Agents (master) ---

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	var out []Agent
	return out, s.DB.WithContext(ctx).Order("id").Find(&out).Error
}

func (s *Store) GetAgent(ctx context.Context, id int64) (*Agent, error) {
	var a Agent
	err := s.DB.WithContext(ctx).First(&a, id).Error
	return &a, err
}

func (s *Store) CreateAgent(ctx context.Context, a *Agent) error {
	return s.DB.WithContext(ctx).Create(a).Error
}

func (s *Store) UpdateAgent(ctx context.Context, a *Agent) error {
	return s.DB.WithContext(ctx).Save(a).Error
}

// UpdateAgentSettings updates seller profile fields without touching domain columns.
func (s *Store) UpdateAgentSettings(ctx context.Context, id int64, name string, status AgentStatus, maxBots int, floor, ceiling int64, tgAdminID *int64) error {
	return s.DB.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"name":                name,
		"status":              status,
		"max_bots":            maxBots,
		"price_floor_toman":   floor,
		"price_ceiling_toman": ceiling,
		"tg_admin_id":         tgAdminID,
	}).Error
}

// AgentDomain returns the custom domain string or empty if unset.
func AgentDomain(a *Agent) string {
	if a == nil || a.CustomDomain == nil {
		return ""
	}
	return *a.CustomDomain
}

// --- Admins ---

func (s *Store) AdminByUsername(ctx context.Context, username string) (*Admin, error) {
	var a Admin
	err := s.DB.WithContext(ctx).Where("username = ?", username).First(&a).Error
	return &a, err
}

func (s *Store) CountAdminsByRole(ctx context.Context, role AdminRole) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Admin{}).Where("role = ?", role).Count(&n).Error
	return n, err
}

func (s *Store) CreateAdmin(ctx context.Context, a *Admin) error {
	return s.DB.WithContext(ctx).Create(a).Error
}

func (s *Store) GetAdmin(ctx context.Context, id int64) (*Admin, error) {
	var a Admin
	err := s.DB.WithContext(ctx).First(&a, id).Error
	return &a, err
}

// --- Panels (master) ---

func (s *Store) ListPanels(ctx context.Context) ([]Panel, error) {
	var out []Panel
	return out, s.DB.WithContext(ctx).Order("id").Find(&out).Error
}

func (s *Store) GetPanel(ctx context.Context, id int64) (*Panel, error) {
	var p Panel
	err := s.DB.WithContext(ctx).First(&p, id).Error
	return &p, err
}

func (s *Store) CreatePanel(ctx context.Context, p *Panel) error {
	return s.DB.WithContext(ctx).Create(p).Error
}

func (s *Store) UpdatePanel(ctx context.Context, p *Panel) error {
	return s.DB.WithContext(ctx).Save(p).Error
}

func (s *Store) CountAgentPanelsByPanel(ctx context.Context, panelID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&AgentPanel{}).Where("panel_id = ?", panelID).Count(&n).Error
	return n, err
}

// PanelAgentRow is an agent assignment for a panel (master detail view).
type PanelAgentRow struct {
	AgentID   int64
	AgentName string
	MaxUsers  int
	ExpiryCap int
}

func (s *Store) ListPanelAgentRows(ctx context.Context, panelID int64) ([]PanelAgentRow, error) {
	var out []PanelAgentRow
	err := s.DB.WithContext(ctx).
		Table("agent_panels").
		Select("agent_panels.agent_id, agents.name AS agent_name, agent_panels.max_users, agent_panels.expiry_cap_days AS expiry_cap").
		Joins("JOIN agents ON agents.id = agent_panels.agent_id").
		Where("agent_panels.panel_id = ?", panelID).
		Scan(&out).Error
	return out, err
}

// PanelBotRow is a bot assignment for a panel (master detail view).
type PanelBotRow struct {
	BotID       int64
	BotUsername string
	AgentID     int64
}

func (s *Store) ListPanelBotRows(ctx context.Context, panelID int64) ([]PanelBotRow, error) {
	var out []PanelBotRow
	err := s.DB.WithContext(ctx).
		Table("bot_panels").
		Select("bot_panels.bot_id, bots.username AS bot_username, bots.agent_id").
		Joins("JOIN bots ON bots.id = bot_panels.bot_id").
		Where("bot_panels.panel_id = ?", panelID).
		Scan(&out).Error
	return out, err
}

// --- Bots ---

func (s *Store) ListActiveBots(ctx context.Context) ([]Bot, error) {
	var out []Bot
	return out, s.DB.WithContext(ctx).Where("status = ?", "active").Find(&out).Error
}

func (s *Store) ListBotsByAgent(ctx context.Context, agentID int64) ([]Bot, error) {
	var out []Bot
	return out, s.DB.WithContext(ctx).Where("agent_id = ?", agentID).Find(&out).Error
}

func (s *Store) CountBotsByAgent(ctx context.Context, agentID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Bot{}).Where("agent_id = ?", agentID).Count(&n).Error
	return n, err
}

func (s *Store) GetBot(ctx context.Context, id int64) (*Bot, error) {
	var b Bot
	err := s.DB.WithContext(ctx).First(&b, id).Error
	return &b, err
}

func (s *Store) GetBotForAgent(ctx context.Context, agentID, botID int64) (*Bot, error) {
	var b Bot
	err := s.DB.WithContext(ctx).Where("id = ? AND agent_id = ?", botID, agentID).First(&b).Error
	return &b, err
}

func (s *Store) GetBotByPublicID(ctx context.Context, publicID string) (*Bot, error) {
	var b Bot
	err := s.DB.WithContext(ctx).Where("public_id = ?", publicID).First(&b).Error
	return &b, err
}

func (s *Store) CreateBot(ctx context.Context, b *Bot) error {
	return s.DB.WithContext(ctx).Create(b).Error
}

func (s *Store) UpdateBot(ctx context.Context, b *Bot) error {
	return s.DB.WithContext(ctx).Save(b).Error
}

// --- Bot panels ---

func (s *Store) ListBotPanels(ctx context.Context, botID int64) ([]BotPanel, error) {
	var out []BotPanel
	return out, s.DB.WithContext(ctx).Where("bot_id = ?", botID).Find(&out).Error
}

func (s *Store) ListPanelsForBot(ctx context.Context, botID int64) ([]Panel, error) {
	var out []Panel
	err := s.DB.WithContext(ctx).
		Model(&Panel{}).
		Joins("JOIN bot_panels ON bot_panels.panel_id = panels.id").
		Where("bot_panels.bot_id = ?", botID).
		Order("panels.id").
		Find(&out).Error
	return out, err
}

func (s *Store) BotHasPanel(ctx context.Context, botID, panelID int64) (bool, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&BotPanel{}).Where("bot_id = ? AND panel_id = ?", botID, panelID).Count(&n).Error
	return n > 0, err
}

func (s *Store) UpsertBotPanel(ctx context.Context, bp *BotPanel) error {
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bot_id"}, {Name: "panel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope_json"}),
	}).Create(bp).Error
}

func (s *Store) DeleteBotPanel(ctx context.Context, botID, panelID int64) error {
	return s.DB.WithContext(ctx).Where("bot_id = ? AND panel_id = ?", botID, panelID).Delete(&BotPanel{}).Error
}

// --- Plans (tenant) ---

func (s *Store) ListPlansByBot(ctx context.Context, botID int64, activeOnly bool) ([]Plan, error) {
	var out []Plan
	q := s.DB.WithContext(ctx).Where("bot_id = ?", botID)
	if activeOnly {
		q = q.Where("status = ?", "active")
	}
	return out, q.Order("id").Find(&out).Error
}

func (s *Store) GetPlan(ctx context.Context, botID, id int64) (*Plan, error) {
	var p Plan
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, id).First(&p).Error
	return &p, err
}

func (s *Store) CreatePlan(ctx context.Context, p *Plan) error {
	return s.DB.WithContext(ctx).Create(p).Error
}

func (s *Store) UpdatePlan(ctx context.Context, botID int64, p *Plan) error {
	p.BotID = botID
	return s.DB.WithContext(ctx).Where("bot_id = ?", botID).Save(p).Error
}

// --- End users (tenant) ---

func (s *Store) GetOrCreateEndUser(ctx context.Context, botID, telegramID int64) (*EndUser, error) {
	var u EndUser
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND telegram_id = ?", botID, telegramID).First(&u).Error
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	u = EndUser{BotID: botID, TelegramID: telegramID, Lang: "fa", WarnPercent: 80}
	if err := s.DB.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetEndUser(ctx context.Context, botID, id int64) (*EndUser, error) {
	var u EndUser
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, id).First(&u).Error
	return &u, err
}

func (s *Store) UpdateEndUser(ctx context.Context, botID int64, u *EndUser) error {
	return s.DB.WithContext(ctx).Where("bot_id = ?", botID).Save(u).Error
}

// --- Orders (tenant) ---

func (s *Store) CreateOrder(ctx context.Context, o *Order) error {
	return s.DB.WithContext(ctx).Create(o).Error
}

func (s *Store) GetOrder(ctx context.Context, botID, id int64) (*Order, error) {
	var o Order
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, id).First(&o).Error
	return &o, err
}

func (s *Store) UpdateOrder(ctx context.Context, botID int64, o *Order) error {
	return s.DB.WithContext(ctx).Where("bot_id = ?", botID).Save(o).Error
}

// --- Payments (tenant) ---

func (s *Store) CreatePayment(ctx context.Context, p *Payment) error {
	return s.DB.WithContext(ctx).Create(p).Error
}

func (s *Store) GetPayment(ctx context.Context, botID, id int64) (*Payment, error) {
	var p Payment
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, id).First(&p).Error
	return &p, err
}

func (s *Store) ListPendingPayments(ctx context.Context, botID int64) ([]Payment, error) {
	var out []Payment
	return out, s.DB.WithContext(ctx).Where("bot_id = ? AND status = ?", botID, PaymentPending).Find(&out).Error
}

func (s *Store) ListPendingPaymentsByAgent(ctx context.Context, agentID int64) ([]Payment, error) {
	var out []Payment
	return out, s.DB.WithContext(ctx).
		Joins("JOIN bots ON bots.id = payments.bot_id").
		Where("bots.agent_id = ? AND payments.status = ?", agentID, PaymentPending).
		Find(&out).Error
}

func (s *Store) ListAllPendingPayments(ctx context.Context) ([]Payment, error) {
	var out []Payment
	err := s.DB.WithContext(ctx).
		Where("status = ?", PaymentPending).
		Order("id DESC").
		Find(&out).Error
	return out, err
}

// --- Wallet (tenant, transactional) ---

func (s *Store) CreditWallet(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u EndUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, endUserID).First(&u).Error; err != nil {
			return err
		}
		u.BalanceToman += amount
		if err := tx.Save(&u).Error; err != nil {
			return err
		}
		return tx.Create(&WalletTx{
			BotID: botID, EndUserID: endUserID, DeltaToman: amount,
			Reason: reason, RefType: refType, RefID: refID, BalanceAfter: u.BalanceToman,
		}).Error
	})
}

var ErrInsufficientBalance = errors.New("insufficient balance")

func (s *Store) DebitWallet(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u EndUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, endUserID).First(&u).Error; err != nil {
			return err
		}
		if u.BalanceToman < amount {
			return ErrInsufficientBalance
		}
		u.BalanceToman -= amount
		if err := tx.Save(&u).Error; err != nil {
			return err
		}
		return tx.Create(&WalletTx{
			BotID: botID, EndUserID: endUserID, DeltaToman: -amount,
			Reason: reason, RefType: refType, RefID: refID, BalanceAfter: u.BalanceToman,
		}).Error
	})
}

func (s *Store) RejectPayment(ctx context.Context, botID, paymentID, reviewerID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, paymentID).First(&p).Error; err != nil {
			return err
		}
		if p.Status != PaymentPending {
			return errors.New("payment not pending")
		}
		now := time.Now()
		p.Status = PaymentRejected
		p.ReviewedBy = &reviewerID
		p.ReviewedAt = &now
		return tx.Save(&p).Error
	})
}

func (s *Store) ApprovePayment(ctx context.Context, botID, paymentID, reviewerID int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, paymentID).First(&p).Error; err != nil {
			return err
		}
		if p.Status != PaymentPending {
			return errors.New("payment not pending")
		}
		now := time.Now()
		p.Status = PaymentApproved
		p.ReviewedBy = &reviewerID
		p.ReviewedAt = &now
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		var u EndUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, p.EndUserID).First(&u).Error; err != nil {
			return err
		}
		u.BalanceToman += p.AmountToman
		if err := tx.Save(&u).Error; err != nil {
			return err
		}
		return tx.Create(&WalletTx{
			BotID: botID, EndUserID: p.EndUserID, DeltaToman: p.AmountToman,
			Reason: "topup_approved", RefType: "payment", RefID: p.ID, BalanceAfter: u.BalanceToman,
		}).Error
	})
}

// --- Services (tenant) ---

func (s *Store) CreateService(ctx context.Context, svc *Service) error {
	return s.DB.WithContext(ctx).Create(svc).Error
}

func (s *Store) ListActiveServices(ctx context.Context, botID int64) ([]Service, error) {
	var out []Service
	return out, s.DB.WithContext(ctx).Where("bot_id = ? AND status = ?", botID, "active").Find(&out).Error
}

func (s *Store) ListServicesByUser(ctx context.Context, botID, endUserID int64) ([]Service, error) {
	var out []Service
	return out, s.DB.WithContext(ctx).Where("bot_id = ? AND end_user_id = ?", botID, endUserID).Find(&out).Error
}

func (s *Store) ListActiveServicesForPanel(ctx context.Context, panelID int64) ([]Service, error) {
	var out []Service
	return out, s.DB.WithContext(ctx).Where("panel_id = ? AND status = ?", panelID, "active").Find(&out).Error
}

func (s *Store) ListServicesForPanel(ctx context.Context, panelID int64) ([]Service, error) {
	var out []Service
	return out, s.DB.WithContext(ctx).Where("panel_id = ?", panelID).Order("id DESC").Find(&out).Error
}

func (s *Store) ListServicesForPanelUsernames(ctx context.Context, panelID int64, usernames []string) ([]Service, error) {
	if len(usernames) == 0 {
		return nil, nil
	}
	var out []Service
	return out, s.DB.WithContext(ctx).Where("panel_id = ? AND panel_username IN ?", panelID, usernames).Find(&out).Error
}

func (s *Store) GetServiceByPanelUsername(ctx context.Context, panelID int64, username string) (*Service, error) {
	var svc Service
	err := s.DB.WithContext(ctx).Where("panel_id = ? AND panel_username = ?", panelID, username).Order("id DESC").First(&svc).Error
	return &svc, err
}

func (s *Store) UpdateService(ctx context.Context, botID int64, svc *Service) error {
	return s.DB.WithContext(ctx).Where("bot_id = ?", botID).Save(svc).Error
}

func (s *Store) GetSetting(ctx context.Context, scope string, scopeID int64, key string) (string, error) {
	var st Setting
	err := s.DB.WithContext(ctx).Where("scope = ? AND scope_id = ? AND key = ?", scope, scopeID, key).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return st.Value, err
}

func (s *Store) SetSetting(ctx context.Context, scope string, scopeID int64, key, value string) error {
	st := Setting{Scope: scope, ScopeID: scopeID, Key: key, Value: value}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scope"}, {Name: "scope_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&st).Error
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	return s.DB.WithContext(ctx).Create(sess).Error
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	err := s.DB.WithContext(ctx).Where("id = ? AND expires_at > ?", id, time.Now()).First(&sess).Error
	return &sess, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Delete(&Session{}, "id = ?", id).Error
}

// --- Audit ---

func (s *Store) Audit(ctx context.Context, adminID *int64, action, entityType string, entityID int64, detail []byte) error {
	return s.DB.WithContext(ctx).Create(&AuditLog{
		AdminID: adminID, Action: action, EntityType: entityType, EntityID: entityID,
		DetailJSON: detail,
	}).Error
}

// --- Stats ---

func (s *Store) CountPendingPaymentsAll(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Payment{}).Where("status = ?", PaymentPending).Count(&n).Error
	return n, err
}

func (s *Store) CountActiveServices(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Service{}).Where("status = ?", "active").Count(&n).Error
	return n, err
}
