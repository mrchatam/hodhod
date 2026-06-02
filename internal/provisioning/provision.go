package provisioning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panels"
)

// Service provisions VPN accounts on panels.
type Service struct {
	Store  *db.Store
	Panels *panels.Registry
}

// ProvisionOrder creates a panel user for an approved order.
func (s *Service) ProvisionOrder(ctx context.Context, botID int64, order *db.Order) (*db.Service, error) {
	plan, err := s.Store.GetPlan(ctx, botID, order.PlanID)
	if err != nil {
		return nil, err
	}
	if plan.PanelID == nil {
		return nil, fmt.Errorf("plan has no panel")
	}
	ok, err := s.Store.BotHasPanel(ctx, botID, *plan.PanelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bot not assigned to panel")
	}
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}
	panelUsername := fmt.Sprintf("%s-%s", bot.PublicID, uuid.New().String()[:8])
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
	svc := &db.Service{
		BotID:          botID,
		EndUserID:      order.EndUserID,
		OrderID:        &order.ID,
		PanelID:        *plan.PanelID,
		PanelUsername:  panelUsername,
		SubLink:        sub,
		DataLimitBytes: limitBytes,
		ExpireAt:       &expire,
		Status:         "active",
	}
	if err := s.Store.CreateService(ctx, svc); err != nil {
		if delErr := client.DeleteUser(ctx, panelUsername); delErr != nil {
			slog.Error("provisioning reconciliation failed", "bot_id", botID, "order_id", order.ID, "panel_user", panelUsername, "err", delErr)
		}
		return nil, err
	}
	return svc, nil
}
