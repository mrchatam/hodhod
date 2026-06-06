package sales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	InboundIDs   []int
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

// CreatePanelAccountInput unifies all web-side panel account creation.
type CreatePanelAccountInput struct {
	CreateManualInput
	ManualUsername string // non-empty → use as panel username (master panel tab only)
	SkipAgentQuota   bool   // master-only: bypass enforceQuota when true
	SkipPermCheck    bool   // master-only: bypass checkPerm (panel tab emergency create)
	Note             string
	LimitIP          int
}

// ModifyPanelAccountInput is the unified modify DTO for panel clients.
type ModifyPanelAccountInput struct {
	ModifyInput
	PanelID  int64
	Username string
	AgentID  int64
	IsMaster bool
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
	return s.CreatePanelAccount(ctx, CreatePanelAccountInput{CreateManualInput: in})
}

// CreatePanelAccount creates a panel client and Hodhod Service row when AgentID > 0.
func (s *Service) CreatePanelAccount(ctx context.Context, in CreatePanelAccountInput) (*db.Service, error) {
	if !in.SkipPermCheck {
		if err := s.checkPerm(ctx, in.AgentID, in.IsMaster, db.PermCreateUser); err != nil {
			return nil, err
		}
	}
	if in.AgentID > 0 {
		ok, err := s.Store.AgentHasPanel(ctx, in.AgentID, in.PanelID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrPanelNotAssigned
		}
	} else if !in.IsMaster {
		return nil, ErrPanelNotAssigned
	}

	var scope panels.Scope
	var ap *db.AgentPanel
	var err error
	if in.AgentID > 0 {
		scope, ap, err = s.loadCreateScope(ctx, in.AgentID, in.PanelID, in.IsMaster)
		if err != nil {
			return nil, err
		}
	} else if len(in.InboundIDs) > 0 {
		scope = panels.Scope{InboundIDs: in.InboundIDs}
	}

	if !in.IsMaster && in.AgentID > 0 {
		if len(in.InboundIDs) > 0 {
			granted := map[int]bool{}
			for _, id := range scope.InboundIDs {
				granted[id] = true
			}
			var picked []int
			for _, id := range in.InboundIDs {
				if !granted[id] {
					return nil, ErrNoCreateInbound
				}
				picked = append(picked, id)
			}
			scope = panels.Scope{InboundIDs: picked}
		}
	} else if in.IsMaster && len(in.InboundIDs) > 0 {
		scope = panels.Scope{InboundIDs: in.InboundIDs}
	}

	vol := in.VolumeGB
	dur := in.DurationDays
	if vol <= 0 {
		vol = 30
	}
	if dur <= 0 {
		dur = 30
	}
	limitBytes := int64(vol) * 1024 * 1024 * 1024
	if !in.SkipAgentQuota && in.AgentID > 0 {
		if ap == nil {
			ap, _ = s.Store.GetAgentPanel(ctx, in.AgentID, in.PanelID)
		}
		if err := s.enforceQuota(ctx, in.AgentID, in.PanelID, ap, limitBytes, dur); err != nil {
			return nil, err
		}
	}

	panelUsername := strings.TrimSpace(in.ManualUsername)
	if panelUsername == "" {
		panelUsername = fmt.Sprintf("hodhod-%d-%s", in.AgentID, uuid.New().String()[:8])
	}

	var customerID *int64
	if in.CustomerID != nil {
		customerID = in.CustomerID
	}

	client, err := s.Panels.Get(ctx, in.PanelID)
	if err != nil {
		return nil, err
	}
	expire := time.Now().Add(time.Duration(dur) * 24 * time.Hour)
	note := in.Note
	if note == "" {
		note = in.Label
	}
	info, err := client.CreateUser(ctx, panels.CreateUserRequest{
		Username:       panelUsername,
		DataLimitBytes: limitBytes,
		ExpireAt:       expire,
		Scope:          scope,
		Note:           note,
		LimitIP:        in.LimitIP,
	})
	if err != nil {
		return nil, err
	}
	sub := info.SubscriptionURL
	if sub == "" {
		sub, _ = client.SubscriptionURL(ctx, panelUsername)
	}

	if in.AgentID <= 0 {
		return nil, nil
	}

	adminID := in.AdminID
	svc := &db.Service{
		AgentID:          in.AgentID,
		CustomerID:       customerID,
		Source:           db.ServiceSourcePanel,
		Label:            in.Label,
		Contact:          in.Contact,
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
	return s.ModifyPanelAccount(ctx, ModifyPanelAccountInput{
		ModifyInput: in,
		PanelID:     svc.PanelID,
		Username:    svc.PanelUsername,
		AgentID:     agentID,
		IsMaster:    isMaster,
	})
}

// ModifyPanelAccount updates a panel client and syncs the linked db.Service when present.
func (s *Service) ModifyPanelAccount(ctx context.Context, in ModifyPanelAccountInput) (*db.Service, error) {
	if !in.IsMaster {
		if err := s.checkPerm(ctx, in.AgentID, in.IsMaster, db.PermModifyUser); err != nil {
			return nil, err
		}
	}
	req := buildModifyUpdateRequest(in.ModifyInput)
	if req.DataLimitBytes != nil && !in.IsMaster {
		svc, err := s.Store.GetServiceByPanelUsername(ctx, in.PanelID, in.Username)
		if err == nil {
			ap, aerr := s.Store.GetAgentPanel(ctx, svc.AgentID, in.PanelID)
			if aerr == nil && ap.QuotaBytes > 0 && *req.DataLimitBytes > ap.QuotaBytes {
				return nil, ErrQuotaExceeded
			}
		}
	}
	client, err := s.Panels.Get(ctx, in.PanelID)
	if err != nil {
		return nil, err
	}
	info, err := client.UpdateUser(ctx, in.Username, req)
	if err != nil {
		return nil, err
	}
	svc, err := s.Store.GetServiceByPanelUsername(ctx, in.PanelID, in.Username)
	if err != nil {
		return nil, nil
	}
	return s.applyUserInfo(ctx, svc, info)
}

func buildModifyUpdateRequest(in ModifyInput) panels.UpdateUserRequest {
	req := panels.UpdateUserRequest{}
	if in.VolumeGB != nil {
		limit := int64(*in.VolumeGB) * 1024 * 1024 * 1024
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
	return req
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
