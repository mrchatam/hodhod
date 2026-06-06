package sales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

var (
	ErrForbidden        = errors.New("sales: forbidden")
	ErrViewOnly         = errors.New("sales: view_only")
	ErrPermDenied       = errors.New("sales: perm_denied")
	ErrPanelNotAssigned = errors.New("sales: panel_not_assigned")
	ErrNoCreateInbound  = errors.New("sales: no_create_inbound")
	ErrQuotaExceeded    = errors.New("sales: quota exceeded")
)

// Service performs permission-checked panel operations for sellers and bots.
type Service struct {
	Store  *db.Store
	Panels *panels.Registry
}

// CreateManualInput is input for panel-created services (no Telegram user).
type CreateManualInput struct {
	AgentID      int64
	PanelID      int64
	Label        string
	Contact      string
	CustomerID   *int64
	VolumeGB     int
	DurationDays int
	AdminID      int64
	IsMaster     bool
}

// ProvisionOrderInput provisions from a bot order.
type ProvisionOrderInput struct {
	BotID   int64
	Order   *db.Order
	AdminID *int64
}

// ModifyInput carries optional fields for modifying a service limit/expiry/state.
type ModifyInput struct {
	VolumeGB     *int
	ExpireAt     *time.Time
	Enabled      *bool
	DurationDays *int // sets expiry from now when ExpireAt nil
}

func (s *Service) checkPerm(ctx context.Context, agentID int64, isMaster bool, perm db.Perm) error {
	if isMaster {
		return nil
	}
	p, err := s.Store.GetAgentPermissions(ctx, agentID)
	if err != nil {
		return err
	}
	if p.ViewOnly && perm != "" {
		return ErrViewOnly
	}
	if perm != "" && !p.Has(perm) {
		return ErrPermDenied
	}
	return nil
}

func (s *Service) loadCreateScope(ctx context.Context, agentID, panelID int64, isMaster bool) (panels.Scope, *db.AgentPanel, error) {
	ap, err := s.Store.GetAgentPanel(ctx, agentID, panelID)
	if err != nil {
		return panels.Scope{}, nil, err
	}
	if !isMaster {
		ids, err := s.Store.ListAgentInboundCreateIDs(ctx, agentID, panelID)
		if err != nil {
			return panels.Scope{}, ap, err
		}
		if len(ids) == 0 {
			var scope panels.Scope
			_ = json.Unmarshal(ap.ScopeJSON, &scope)
			if len(scope.InboundIDs) == 0 && scope.InboundID > 0 {
				scope.InboundIDs = []int{scope.InboundID}
			}
			if len(scope.InboundIDs) > 0 {
				ids = scope.InboundIDs
			}
		}
		if len(ids) == 0 {
			return panels.Scope{}, ap, ErrNoCreateInbound
		}
		return panels.Scope{InboundIDs: ids}, ap, nil
	}
	var scope panels.Scope
	_ = json.Unmarshal(ap.ScopeJSON, &scope)
	if len(scope.InboundIDs) == 0 && scope.InboundID > 0 {
		scope.InboundIDs = []int{scope.InboundID}
	}
	return scope, ap, nil
}

func (s *Service) enforceQuota(ctx context.Context, agentID, panelID int64, ap *db.AgentPanel, addBytes int64, durationDays int) error {
	if ap == nil {
		return nil
	}
	if ap.MaxUsers > 0 {
		n, err := s.Store.CountServicesByAgentPanel(ctx, agentID, panelID)
		if err != nil {
			return err
		}
		if int(n) >= ap.MaxUsers {
			return ErrQuotaExceeded
		}
	}
	if ap.ExpiryCapDays > 0 && durationDays > ap.ExpiryCapDays {
		return fmt.Errorf("sales: duration exceeds cap (%d days)", ap.ExpiryCapDays)
	}
	// QuotaBytes is the per-service data limit cap for this agent on this panel.
	if ap.QuotaBytes > 0 && addBytes > ap.QuotaBytes {
		return ErrQuotaExceeded
	}
	return nil
}

// CreateManualService creates a VPN account from the sales panel.
func (s *Service) CreateManualService(ctx context.Context, in CreateManualInput) (*db.Service, error) {
	if err := s.checkPerm(ctx, in.AgentID, in.IsMaster, db.PermCreateUser); err != nil {
		return nil, err
	}
	ok, err := s.Store.AgentHasPanel(ctx, in.AgentID, in.PanelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPanelNotAssigned
	}
	scope, ap, err := s.loadCreateScope(ctx, in.AgentID, in.PanelID, in.IsMaster)
	if err != nil {
		return nil, err
	}
	limitBytes := int64(in.VolumeGB) * 1024 * 1024 * 1024
	if err := s.enforceQuota(ctx, in.AgentID, in.PanelID, ap, limitBytes, in.DurationDays); err != nil {
		return nil, err
	}
	var customerID *int64
	if in.CustomerID != nil {
		customerID = in.CustomerID
	} else if in.Label != "" || in.Contact != "" {
		c := &db.Customer{AgentID: in.AgentID, Label: in.Label, Contact: in.Contact}
		if err := s.Store.CreateCustomer(ctx, c); err != nil {
			return nil, err
		}
		customerID = &c.ID
	}
	client, err := s.Panels.Get(ctx, in.PanelID)
	if err != nil {
		return nil, err
	}
	panelUsername := fmt.Sprintf("hodhod-%d-%s", in.AgentID, uuid.New().String()[:8])
	expire := time.Now().Add(time.Duration(in.DurationDays) * 24 * time.Hour)
	info, err := client.CreateUser(ctx, panels.CreateUserRequest{
		Username:       panelUsername,
		DataLimitBytes: limitBytes,
		ExpireAt:       expire,
		Scope:          scope,
		Note:           in.Label,
	})
	if err != nil {
		return nil, err
	}
	sub := info.SubscriptionURL
	if sub == "" {
		sub, _ = client.SubscriptionURL(ctx, panelUsername)
	}
	adminID := in.AdminID
	svc := &db.Service{
		AgentID:          in.AgentID,
		CustomerID:       customerID,
		Source:           db.ServiceSourcePanel,
		Label:            in.Label,
		CreatedByAdminID: &adminID,
		PanelID:          in.PanelID,
		PanelUsername:    panelUsername,
		SubLink:          sub,
		DataLimitBytes:   limitBytes,
		ExpireAt:         &expire,
		Status:           "active",
	}
	if err := s.Store.CreateService(ctx, svc); err != nil {
		_ = client.DeleteUser(ctx, panelUsername)
		return nil, err
	}
	return svc, nil
}

// ProvisionFromOrder creates a service for a bot purchase (shared path).
func (s *Service) ProvisionFromOrder(ctx context.Context, botID int64, order *db.Order) (*db.Service, error) {
	plan, err := s.Store.GetPlan(ctx, botID, order.PlanID)
	if err != nil {
		return nil, err
	}
	if plan.PanelID == nil {
		return nil, fmt.Errorf("plan has no panel")
	}
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}
	ok, err := s.Store.BotHasPanel(ctx, botID, *plan.PanelID)
	if err != nil || !ok {
		return nil, fmt.Errorf("bot not assigned to panel")
	}
	var scope panels.Scope
	bps, _ := s.Store.ListBotPanels(ctx, botID)
	for _, bp := range bps {
		if bp.PanelID == *plan.PanelID {
			_ = json.Unmarshal(bp.ScopeJSON, &scope)
			break
		}
	}
	client, err := s.Panels.Get(ctx, *plan.PanelID)
	if err != nil {
		return nil, err
	}
	panelUsername := fmt.Sprintf("%s-%s", bot.PublicID, uuid.New().String()[:8])
	expire := time.Now().Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	limitBytes := int64(plan.VolumeGB) * 1024 * 1024 * 1024
	info, err := client.CreateUser(ctx, panels.CreateUserRequest{
		Username:       panelUsername,
		DataLimitBytes: limitBytes,
		ExpireAt:       expire,
		Scope:          scope,
		Note:           fmt.Sprintf("hodhod bot=%d order=%d", botID, order.ID),
	})
	if err != nil {
		return nil, err
	}
	sub := info.SubscriptionURL
	if sub == "" {
		sub, _ = client.SubscriptionURL(ctx, panelUsername)
	}
	endUserID := order.EndUserID
	botIDPtr := botID
	svc := &db.Service{
		AgentID:        bot.AgentID,
		BotID:          &botIDPtr,
		EndUserID:      &endUserID,
		OrderID:        &order.ID,
		Source:         db.ServiceSourceBot,
		PanelID:        *plan.PanelID,
		PanelUsername:  panelUsername,
		SubLink:        sub,
		DataLimitBytes: limitBytes,
		ExpireAt:       &expire,
		Status:         "active",
	}
	if err := s.Store.CreateService(ctx, svc); err != nil {
		_ = client.DeleteUser(ctx, panelUsername)
		return nil, err
	}
	return svc, nil
}

func (s *Service) getService(ctx context.Context, agentID, serviceID int64, isMaster bool) (*db.Service, error) {
	if isMaster {
		return s.Store.GetService(ctx, serviceID)
	}
	return s.Store.GetServiceForAgent(ctx, agentID, serviceID)
}

// RefreshUsage syncs usage from the panel.
func (s *Service) RefreshUsage(ctx context.Context, agentID, serviceID int64, isMaster bool) (*db.Service, error) {
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	info, err := client.GetUser(ctx, svc.PanelUsername)
	if err != nil {
		return nil, err
	}
	svc.UsedBytes = info.UsedBytes
	svc.DataLimitBytes = info.DataLimitBytes
	if !info.ExpireAt.IsZero() {
		t := info.ExpireAt
		svc.ExpireAt = &t
	}
	if !info.Enabled {
		svc.Status = "disabled"
	}
	if err := s.Store.UpdateServiceByID(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// ModifyService updates limit, expiry, or enabled state on the panel.
func (s *Service) ModifyService(ctx context.Context, agentID, serviceID int64, in ModifyInput, isMaster bool) (*db.Service, error) {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermModifyUser); err != nil {
		return nil, err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	req := panels.UpdateUserRequest{}
	if in.VolumeGB != nil {
		limit := int64(*in.VolumeGB) * 1024 * 1024 * 1024
		if !isMaster {
			ap, err := s.Store.GetAgentPanel(ctx, svc.AgentID, svc.PanelID)
			if err == nil && ap.QuotaBytes > 0 && limit > ap.QuotaBytes {
				return nil, ErrQuotaExceeded
			}
		}
		req.DataLimitBytes = &limit
	}
	if in.ExpireAt != nil {
		req.ExpireAt = in.ExpireAt
	} else if in.DurationDays != nil && *in.DurationDays > 0 {
		t := time.Now().Add(time.Duration(*in.DurationDays) * 24 * time.Hour)
		req.ExpireAt = &t
	}
	if in.Enabled != nil {
		req.Enabled = in.Enabled
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	info, err := client.UpdateUser(ctx, svc.PanelUsername, req)
	if err != nil {
		return nil, err
	}
	return s.applyUserInfo(ctx, svc, info)
}

func (s *Service) AddTime(ctx context.Context, agentID, serviceID int64, days int, isMaster bool) (*db.Service, error) {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermAddTime); err != nil {
		return nil, err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	info, err := client.UpdateUser(ctx, svc.PanelUsername, panels.UpdateUserRequest{AddDays: days})
	if err != nil {
		return nil, err
	}
	return s.applyUserInfo(ctx, svc, info)
}

func (s *Service) AddVolume(ctx context.Context, agentID, serviceID int64, gb int, isMaster bool) (*db.Service, error) {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermAddVolume); err != nil {
		return nil, err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	addBytes := int64(gb) * 1024 * 1024 * 1024
	info, err := client.UpdateUser(ctx, svc.PanelUsername, panels.UpdateUserRequest{AddBytes: addBytes})
	if err != nil {
		return nil, err
	}
	return s.applyUserInfo(ctx, svc, info)
}

func (s *Service) SetEnabled(ctx context.Context, agentID, serviceID int64, enabled bool, isMaster bool) (*db.Service, error) {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermDisableEnable); err != nil {
		return nil, err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	var info *panels.UserInfo
	if enabled {
		err = client.Enable(ctx, svc.PanelUsername)
	} else {
		err = client.Disable(ctx, svc.PanelUsername)
	}
	if err != nil {
		return nil, err
	}
	info, err = client.GetUser(ctx, svc.PanelUsername)
	if err != nil {
		return nil, err
	}
	return s.applyUserInfo(ctx, svc, info)
}

func (s *Service) ResetUsage(ctx context.Context, agentID, serviceID int64, isMaster bool) (*db.Service, error) {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermResetUsage); err != nil {
		return nil, err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return nil, err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return nil, err
	}
	if err := client.ResetUsage(ctx, svc.PanelUsername); err != nil {
		return nil, err
	}
	svc.UsedBytes = 0
	if err := s.Store.UpdateServiceByID(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) DeleteService(ctx context.Context, agentID, serviceID int64, isMaster bool) error {
	if err := s.checkPerm(ctx, agentID, isMaster, db.PermDeleteUser); err != nil {
		return err
	}
	svc, err := s.getService(ctx, agentID, serviceID, isMaster)
	if err != nil {
		return err
	}
	client, err := s.Panels.Get(ctx, svc.PanelID)
	if err != nil {
		return err
	}
	if err := client.DeleteUser(ctx, svc.PanelUsername); err != nil && !errors.Is(err, panels.ErrUserNotFound) {
		return err
	}
	return s.Store.DeleteService(ctx, svc.ID)
}

func (s *Service) applyUserInfo(ctx context.Context, svc *db.Service, info *panels.UserInfo) (*db.Service, error) {
	svc.UsedBytes = info.UsedBytes
	svc.DataLimitBytes = info.DataLimitBytes
	if !info.ExpireAt.IsZero() {
		t := info.ExpireAt
		svc.ExpireAt = &t
	}
	if info.Enabled {
		svc.Status = "active"
	} else {
		svc.Status = "disabled"
	}
	if info.SubscriptionURL != "" {
		svc.SubLink = info.SubscriptionURL
	}
	if err := s.Store.UpdateServiceByID(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}
