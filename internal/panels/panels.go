package panels

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// PanelKind identifies the panel adapter type.
type PanelKind string

const (
	KindMarzban PanelKind = "marzban"
	KindXUI     PanelKind = "xui"
)

// Scope limits provisioning (inbounds/tags/flow) for a bot on a panel.
type Scope struct {
	InboundID  int      `json:"inbound_id,omitempty"`
	InboundIDs []int    `json:"inbound_ids,omitempty"`
	Inbounds   []string `json:"inbounds,omitempty"`
	Flow       string   `json:"flow,omitempty"`
	SubBaseURL string   `json:"sub_base_url,omitempty"`
}

// CreateUserRequest is input for provisioning.
type CreateUserRequest struct {
	Username       string
	DataLimitBytes int64
	ExpireAt       time.Time
	Scope          Scope
	Note           string
	LimitIP        int
}

// UserInfo is normalized panel user state.
type UserInfo struct {
	Username        string
	UsedBytes       int64
	DataLimitBytes  int64
	ExpireAt        time.Time
	Enabled         bool
	SubscriptionURL string
	InboundID       int
	InboundIDs      []int
	InboundTag      string
	InboundTags     []string
	LimitIP         int
	Note            string
	Raw             map[string]any
}

// UpdateUserRequest carries optional fields for modifying a panel user.
type UpdateUserRequest struct {
	DataLimitBytes *int64
	ExpireAt       *time.Time
	AddBytes       int64
	AddDays        int
	Enabled        *bool
}

// InboundInfo describes a panel inbound for scope selection.
type InboundInfo struct {
	ID   int
	Tag  string
	Port int
}

// UserListOptions filters and pages panel user listing.
type UserListOptions struct {
	Page     int
	PageSize int
	Search   string
	Status   string // active, disabled, expired, all
}

// UserListPage is a paginated panel user result.
type UserListPage struct {
	Users    []UserInfo
	Total    int
	Filtered int
	Page     int
	PageSize int
}

// Client is the panel adapter contract.
type Client interface {
	CreateUser(ctx context.Context, req CreateUserRequest) (*UserInfo, error)
	GetUser(ctx context.Context, username string) (*UserInfo, error)
	UpdateUser(ctx context.Context, username string, req UpdateUserRequest) (*UserInfo, error)
	ResetUsage(ctx context.Context, username string) error
	Disable(ctx context.Context, username string) error
	Enable(ctx context.Context, username string) error
	DeleteUser(ctx context.Context, username string) error
	SubscriptionURL(ctx context.Context, username string) (string, error)
	ListInbounds(ctx context.Context) ([]InboundInfo, error)
	ListUsers(ctx context.Context) ([]UserInfo, error)
	ListUsersPaged(ctx context.Context, opts UserListOptions) (*UserListPage, error)
	Kind() PanelKind
	TestConnection(ctx context.Context) error
}

// Backuper can download a native panel database snapshot (3x-ui x-ui.db).
type Backuper interface {
	Backup(ctx context.Context) (filename string, data []byte, err error)
}

// Config for building a panel client.
type Config struct {
	Type     PanelKind
	BaseURL  string
	BasePath string
	Username string
	Password string
	APIToken string
	Extra    map[string]any
}

var (
	ErrUnsupported  = errors.New("panels: unsupported operation")
	ErrUserNotFound = errors.New("panels: user not found")
)

// PanelAuthError indicates authentication failure.
type PanelAuthError struct {
	Msg string
}

func (e *PanelAuthError) Error() string { return "panels: auth: " + e.Msg }

// New builds a panel client for the given config.
func New(cfg Config, httpClient *http.Client) (Client, error) {
	switch cfg.Type {
	case KindMarzban:
		return newMarzban(cfg, httpClient), nil
	case KindXUI:
		return newXUI(cfg, httpClient), nil
	default:
		return nil, fmt.Errorf("panels: unknown type %q", cfg.Type)
	}
}
