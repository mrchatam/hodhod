package db

import (
	"time"

	"gorm.io/datatypes"
)

type AgentStatus string

const (
	AgentActive   AgentStatus = "active"
	AgentDisabled AgentStatus = "disabled"
)

func (s AgentStatus) Valid() bool {
	return s == AgentActive || s == AgentDisabled
}

type Agent struct {
	ID                int64  `gorm:"primaryKey"`
	Name              string `gorm:"not null"`
	TgAdminID         *int64
	Status            AgentStatus `gorm:"not null;default:active"`
	MaxBots           int         `gorm:"not null;default:5"`
	PriceFloorToman   int64       `gorm:"not null;default:0"`
	PriceCeilingToman int64       `gorm:"not null;default:999999999"`
	CreatedAt         time.Time
}

type AdminRole string

const (
	RoleMaster AdminRole = "master"
	RoleAgent  AdminRole = "agent"
)

func (r AdminRole) Valid() bool { return r == RoleMaster || r == RoleAgent }

type Admin struct {
	ID           int64 `gorm:"primaryKey"`
	AgentID      *int64
	Username     string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         AdminRole `gorm:"not null"`
	CreatedAt    time.Time
}

type PanelType string

const (
	PanelMarzban PanelType = "marzban"
	PanelXUI     PanelType = "xui"
)

func (t PanelType) Valid() bool { return t == PanelMarzban || t == PanelXUI }

type Panel struct {
	ID          int64          `gorm:"primaryKey"`
	Type        PanelType      `gorm:"not null"`
	Name        string         `gorm:"not null"`
	BaseURL     string         `gorm:"not null"`
	BasePath    string         `gorm:"not null;default:''"`
	Username    string         `gorm:"not null"`
	PasswordEnc string         `gorm:"not null"`
	APITokenEnc string         `gorm:"not null;default:''"`
	ExtraJSON   datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
	Status      string         `gorm:"not null;default:active"`
	CreatedAt   time.Time
}

type Bot struct {
	ID            int64          `gorm:"primaryKey"`
	AgentID       int64          `gorm:"not null;index"`
	PublicID      string         `gorm:"uniqueIndex;not null"`
	Username      string         `gorm:"not null;default:''"`
	TokenEnc      string         `gorm:"not null"`
	WebhookSecret string         `gorm:"not null"`
	Status        string         `gorm:"not null;default:active"`
	SettingsJSON  datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
	CreatedAt     time.Time
}

type BotPanel struct {
	ID        int64          `gorm:"primaryKey"`
	BotID     int64          `gorm:"not null;uniqueIndex:idx_bot_panel"`
	PanelID   int64          `gorm:"not null;uniqueIndex:idx_bot_panel"`
	ScopeJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
}

type Plan struct {
	ID           int64 `gorm:"primaryKey"`
	BotID        int64 `gorm:"not null;index"`
	AgentID      int64 `gorm:"not null"`
	PanelID      *int64
	Name         string `gorm:"not null"`
	DurationDays int    `gorm:"not null"`
	VolumeGB     int    `gorm:"not null"`
	PriceToman   int64  `gorm:"not null"`
	Status       string `gorm:"not null;default:active"`
	CreatedAt    time.Time
}

type EndUser struct {
	ID           int64  `gorm:"primaryKey"`
	BotID        int64  `gorm:"not null;uniqueIndex:idx_bot_tg"`
	TelegramID   int64  `gorm:"not null;uniqueIndex:idx_bot_tg"`
	Lang         string `gorm:"not null;default:fa"`
	BalanceToman int64  `gorm:"not null;default:0"`
	WarnPercent  int    `gorm:"not null;default:80"`
	CreatedAt    time.Time
}

type OrderStatus string

const (
	OrderPendingPayment   OrderStatus = "pending_payment"
	OrderAwaitingApproval OrderStatus = "awaiting_approval"
	OrderApproved         OrderStatus = "approved"
	OrderProvisioned      OrderStatus = "provisioned"
	OrderRejected         OrderStatus = "rejected"
	OrderExpired          OrderStatus = "expired"
)

func (s OrderStatus) Valid() bool {
	switch s {
	case OrderPendingPayment, OrderAwaitingApproval, OrderApproved, OrderProvisioned, OrderRejected, OrderExpired:
		return true
	}
	return false
}

type Order struct {
	ID         int64       `gorm:"primaryKey"`
	BotID      int64       `gorm:"not null;index"`
	EndUserID  int64       `gorm:"not null"`
	PlanID     int64       `gorm:"not null"`
	Status     OrderStatus `gorm:"not null"`
	PriceToman int64       `gorm:"not null"`
	CreatedAt  time.Time
	ApprovedBy *int64
	ApprovedAt *time.Time
}

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentApproved PaymentStatus = "approved"
	PaymentRejected PaymentStatus = "rejected"
)

func (s PaymentStatus) Valid() bool {
	return s == PaymentPending || s == PaymentApproved || s == PaymentRejected
}

type Payment struct {
	ID          int64 `gorm:"primaryKey"`
	BotID       int64 `gorm:"not null;index"`
	EndUserID   int64 `gorm:"not null"`
	OrderID     *int64
	AmountToman int64         `gorm:"not null"`
	Method      string        `gorm:"not null"`
	ReceiptRef  string        `gorm:"not null;default:''"`
	Status      PaymentStatus `gorm:"not null;default:pending"`
	ReviewedBy  *int64
	ReviewedAt  *time.Time
	CreatedAt   time.Time
}

type WalletTx struct {
	ID           int64  `gorm:"primaryKey"`
	BotID        int64  `gorm:"not null;index"`
	EndUserID    int64  `gorm:"not null"`
	DeltaToman   int64  `gorm:"not null"`
	Reason       string `gorm:"not null"`
	RefType      string `gorm:"not null;default:''"`
	RefID        int64  `gorm:"not null;default:0"`
	BalanceAfter int64  `gorm:"not null"`
	CreatedAt    time.Time
}

type Service struct {
	ID                int64 `gorm:"primaryKey"`
	BotID             int64 `gorm:"not null;index"`
	EndUserID         int64 `gorm:"not null"`
	OrderID           *int64
	PanelID           int64  `gorm:"not null"`
	PanelUsername     string `gorm:"not null"`
	SubLink           string `gorm:"not null;default:''"`
	DataLimitBytes    int64  `gorm:"not null;default:0"`
	UsedBytes         int64  `gorm:"not null;default:0"`
	ExpireAt          *time.Time
	Status            string `gorm:"not null;default:active"`
	LastWarnedPercent int    `gorm:"not null;default:0"`
	CreatedAt         time.Time
}

type Setting struct {
	ID      int64  `gorm:"primaryKey"`
	Scope   string `gorm:"not null;uniqueIndex:idx_setting_scope"`
	ScopeID int64  `gorm:"not null;uniqueIndex:idx_setting_scope;default:0"`
	Key     string `gorm:"not null;uniqueIndex:idx_setting_scope"`
	Value   string `gorm:"not null;default:''"`
}

type AuditLog struct {
	ID         int64 `gorm:"primaryKey"`
	AdminID    *int64
	Action     string         `gorm:"not null"`
	EntityType string         `gorm:"not null"`
	EntityID   int64          `gorm:"not null;default:0"`
	DetailJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
}

type Session struct {
	ID        string    `gorm:"primaryKey"`
	AdminID   int64     `gorm:"not null"`
	CSRFToken string    `gorm:"not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}
