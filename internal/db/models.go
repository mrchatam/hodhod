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
	CustomDomain      *string
	DomainEnabled     bool        `gorm:"not null;default:false"`
	DomainVerifiedAt  *time.Time
	DomainVerifyToken string `gorm:"not null;default:''"`
	CreatedAt         time.Time
}

// AgentPermissions are granular seller capabilities set by master.
type AgentPermissions struct {
	AgentID       int64 `gorm:"primaryKey"`
	CreateUser    bool  `gorm:"not null;default:false"`
	ModifyUser    bool  `gorm:"not null;default:false"`
	AddTime       bool  `gorm:"not null;default:false"`
	AddVolume     bool  `gorm:"not null;default:false"`
	ResetUsage    bool  `gorm:"not null;default:false"`
	DisableEnable bool  `gorm:"not null;default:false"`
	DeleteUser    bool  `gorm:"not null;default:false"`
	ManageBot     bool  `gorm:"not null;default:false"`
	ManagePlans   bool  `gorm:"not null;default:false"`
	ViewOnly      bool  `gorm:"not null;default:true"`
}

// Perm names for permission checks.
type Perm string

const (
	PermCreateUser    Perm = "create_user"
	PermModifyUser    Perm = "modify_user"
	PermAddTime       Perm = "add_time"
	PermAddVolume     Perm = "add_volume"
	PermResetUsage    Perm = "reset_usage"
	PermDisableEnable Perm = "disable_enable"
	PermDeleteUser    Perm = "delete_user"
	PermManageBot     Perm = "manage_bot"
	PermManagePlans   Perm = "manage_plans"
)

func (p *AgentPermissions) Has(perm Perm) bool {
	if p == nil {
		return false
	}
	switch perm {
	case PermCreateUser:
		return p.CreateUser
	case PermModifyUser:
		return p.ModifyUser
	case PermAddTime:
		return p.AddTime
	case PermAddVolume:
		return p.AddVolume
	case PermResetUsage:
		return p.ResetUsage
	case PermDisableEnable:
		return p.DisableEnable
	case PermDeleteUser:
		return p.DeleteUser
	case PermManageBot:
		return p.ManageBot
	case PermManagePlans:
		return p.ManagePlans
	default:
		return false
	}
}

// AgentPanel assigns a panel + scope + quota to a seller.
type AgentPanel struct {
	ID            int64          `gorm:"primaryKey"`
	AgentID       int64          `gorm:"not null;uniqueIndex:idx_agent_panel"`
	PanelID       int64          `gorm:"not null;uniqueIndex:idx_agent_panel"`
	ScopeJSON     datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
	QuotaBytes    int64          `gorm:"not null;default:0"`
	MaxUsers      int            `gorm:"not null;default:0"`
	ExpiryCapDays int            `gorm:"not null;default:0"`
}

// AgentInboundGrant controls per-inbound create/view toggles for a seller on a panel.
type AgentInboundGrant struct {
	ID             int64 `gorm:"primaryKey"`
	AgentID        int64 `gorm:"not null;uniqueIndex:idx_agent_inbound_grant"`
	PanelID        int64 `gorm:"not null;uniqueIndex:idx_agent_inbound_grant"`
	InboundID      int   `gorm:"not null;uniqueIndex:idx_agent_inbound_grant"`
	AllowCreate    bool  `gorm:"not null;default:false"`
	AllowViewUsers bool  `gorm:"not null;default:false"`
}

// AgentUserGrant controls per-user view/modify toggles for a seller on a panel.
type AgentUserGrant struct {
	ID            int64  `gorm:"primaryKey"`
	AgentID       int64  `gorm:"not null;uniqueIndex:idx_agent_user_grant"`
	PanelID       int64  `gorm:"not null;uniqueIndex:idx_agent_user_grant"`
	PanelUsername string `gorm:"not null;uniqueIndex:idx_agent_user_grant"`
	AllowView     bool   `gorm:"not null;default:false"`
	AllowModify   bool   `gorm:"not null;default:false"`
}

// Customer groups manual panel services for a seller.
type Customer struct {
	ID        int64  `gorm:"primaryKey"`
	AgentID   int64  `gorm:"not null;index"`
	Label     string `gorm:"not null;default:''"`
	Contact   string `gorm:"not null;default:''"`
	CreatedAt time.Time
}

type ServiceSource string

const (
	ServiceSourceBot   ServiceSource = "bot"
	ServiceSourcePanel ServiceSource = "panel"
)

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

// PanelBackup stores a downloaded panel database snapshot (3x-ui x-ui.db).
type PanelBackup struct {
	ID        int64     `gorm:"primaryKey"`
	PanelID   int64     `gorm:"not null;index"`
	Filename  string    `gorm:"not null"`
	SizeBytes int64     `gorm:"not null;default:0"`
	Status    string    `gorm:"not null;default:ok"`
	Error     string    `gorm:"not null;default:''"`
	CreatedAt time.Time
}

type Bot struct {
	ID               int64          `gorm:"primaryKey"`
	AgentID          int64          `gorm:"not null;index"`
	PublicID         string         `gorm:"uniqueIndex;not null"`
	Username         string         `gorm:"not null;default:''"`
	TokenEnc         string         `gorm:"not null"`
	WebhookSecret    string         `gorm:"not null"`
	Status           string         `gorm:"not null;default:active"`
	SettingsJSON     datatypes.JSON `gorm:"type:jsonb;default:'{}'"` // deprecated: do not write; use bot_* tables and KV via botconfig
	CardDisplayMode  string         `gorm:"not null;default:random"`
	CardRRIndex      int            `gorm:"not null;default:0"`
	WebhookLastError string         `gorm:"not null;default:''"`
	DeletedAt        *time.Time
	CreatedAt        time.Time
}

type BotPanel struct {
	ID        int64          `gorm:"primaryKey"`
	BotID     int64          `gorm:"not null;uniqueIndex:idx_bot_panel"`
	PanelID   int64          `gorm:"not null;uniqueIndex:idx_bot_panel"`
	ScopeJSON datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
}

type Plan struct {
	ID           int64  `gorm:"primaryKey"`
	BotID        int64  `gorm:"not null;index"`
	AgentID      int64  `gorm:"not null"`
	PanelID      *int64
	Name         string `gorm:"not null"`
	Description  string `gorm:"not null;default:''"`
	DurationDays int    `gorm:"not null"`
	VolumeGB     int    `gorm:"not null"`
	PriceToman   int64  `gorm:"not null"`
	SortOrder    int    `gorm:"not null;default:0"`
	IsTrial      bool   `gorm:"not null;default:false"`
	Status       string `gorm:"not null;default:active"`
	CreatedAt    time.Time
}

type EndUser struct {
	ID           int64      `gorm:"primaryKey"`
	BotID        int64      `gorm:"not null;uniqueIndex:idx_bot_tg"`
	TelegramID   int64      `gorm:"not null;uniqueIndex:idx_bot_tg"`
	Username     string     `gorm:"not null;default:''"`
	Lang         string     `gorm:"not null;default:fa"`
	Status       string     `gorm:"not null;default:active"`
	BalanceToman int64      `gorm:"not null;default:0"`
	WarnPercent  int        `gorm:"not null;default:80"`
	TrialUsedAt  *time.Time
	TrialCount   int        `gorm:"not null;default:0"`
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
	ID            int64       `gorm:"primaryKey"`
	BotID         int64       `gorm:"not null;index"`
	EndUserID     int64       `gorm:"not null"`
	PlanID        int64       `gorm:"not null"`
	Status        OrderStatus `gorm:"not null"`
	PriceToman    int64       `gorm:"not null"`
	PaymentMethod string      `gorm:"not null;default:wallet"`
	IsTrial       bool        `gorm:"not null;default:false"`
	CreatedAt     time.Time
	ApprovedBy    *int64
	ApprovedAt    *time.Time
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
	ID                int64  `gorm:"primaryKey"`
	AgentID           int64  `gorm:"not null;index"`
	BotID             *int64 `gorm:"index"`
	EndUserID         *int64
	CustomerID        *int64
	OrderID           *int64
	Source            ServiceSource `gorm:"not null;default:bot"`
	Label             string        `gorm:"not null;default:''"`
	Contact           string        `gorm:"not null;default:''"`
	CreatedByAdminID  *int64
	PanelID           int64  `gorm:"not null"`
	PanelUsername     string `gorm:"not null"`
	SubLink           string `gorm:"not null;default:''"`
	DataLimitBytes    int64  `gorm:"not null;default:0"`
	UsedBytes         int64  `gorm:"not null;default:0"`
	ExpireAt          *time.Time
	Status            string `gorm:"not null;default:active"`
	LastWarnedPercent   int        `gorm:"not null;default:0"`
	LastExpiryWarnedAt  *time.Time
	CreatedAt           time.Time
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

type BotPaymentCard struct {
	ID         int64     `gorm:"primaryKey"`
	BotID      int64     `gorm:"not null;index"`
	Label      string    `gorm:"not null;default:''"`
	CardNumber string    `gorm:"not null"`
	HolderName string    `gorm:"not null;default:''"`
	Weight     int       `gorm:"not null;default:1"`
	SortOrder  int       `gorm:"not null;default:0"`
	Active     bool      `gorm:"not null;default:true"`
	CreatedAt  time.Time
}

type BotChannel struct {
	ID        int64  `gorm:"primaryKey"`
	BotID     int64  `gorm:"not null;index"`
	Username  string `gorm:"not null"`
	Label     string `gorm:"not null;default:''"`
	JoinURL   string `gorm:"not null;default:''"`
	Mandatory bool   `gorm:"not null;default:true"`
	SortOrder int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:true"`
}

func (c BotChannel) LabelOrUsername() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Username
}

// PaymentListRow enriches payment queue rows for the admin panel.
type PaymentListRow struct {
	Payment
	BotUsername string
	EndUserTGID int64
	OrderType   string
}

type BotMenuButton struct {
	ID        int64  `gorm:"primaryKey"`
	BotID     int64  `gorm:"not null;index"`
	ButtonKey string `gorm:"not null"`
	LabelFa   string `gorm:"not null;default:''"`
	LabelEn   string `gorm:"not null;default:''"`
	Enabled   bool   `gorm:"not null;default:true"`
	SortOrder int    `gorm:"not null;default:0"`
	URL       string `gorm:"not null;default:''"`
}

type BotNotificationTarget struct {
	ID         int64          `gorm:"primaryKey"`
	BotID      int64          `gorm:"not null;index"`
	TelegramID int64          `gorm:"not null"`
	Events     datatypes.JSON `gorm:"type:jsonb;default:'[]'"`
	CreatedAt  time.Time
}

// BotListRow aggregates dashboard stats for bot list UI.
type BotListRow struct {
	Bot            Bot
	PendingCount   int64
	ServiceCount   int64
	EndUserCount   int64
	AgentName      string
	WebhookStatus  string
}
