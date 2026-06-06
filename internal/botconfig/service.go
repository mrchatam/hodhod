package botconfig

import (
	"context"
	"fmt"

	"github.com/mrchatam/hodhod/internal/db"
)

// BotService handles bot lifecycle operations.
type BotService struct {
	Store *db.Store
}

// ListForScope returns bots visible to the current scope with dashboard stats.
func (s *BotService) ListForScope(ctx context.Context, scope Scope, agents []db.Agent) ([]db.BotListRow, error) {
	var bots []db.Bot
	if scope.IsMaster {
		for _, a := range agents {
			list, err := s.Store.ListBotsByAgent(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			bots = append(bots, list...)
		}
	} else {
		list, err := s.Store.ListBotsByAgent(ctx, scope.AgentID)
		if err != nil {
			return nil, err
		}
		bots = bots[:0]
		bots = list
	}
	agentNames := make(map[int64]string, len(agents))
	for _, a := range agents {
		agentNames[a.ID] = a.Name
	}
	out := make([]db.BotListRow, 0, len(bots))
	for _, b := range bots {
		pending, _ := s.Store.CountPendingPaymentsByBot(ctx, b.ID)
		services, _ := s.Store.CountActiveServicesByBot(ctx, b.ID)
		users, _ := s.Store.CountEndUsersByBot(ctx, b.ID)
		out = append(out, db.BotListRow{
			Bot:          b,
			PendingCount: pending,
			ServiceCount: services,
			EndUserCount: users,
			AgentName:    agentNames[b.AgentID],
		})
	}
	return out, nil
}

// Delete soft-deletes a bot after checks; caller must teardown webhook.
func (s *BotService) Delete(ctx context.Context, botID int64) error {
	n, err := s.Store.CountActiveServicesByBot(ctx, botID)
	if err != nil {
		return err
	}
	if n > 0 {
		return db.ErrBotHasActiveServices
	}
	return s.Store.SoftDeleteBot(ctx, botID)
}

// AdjustWallet credits or debits an end-user with audit reason.
func (s *BotService) AdjustWallet(ctx context.Context, wallet WalletAdjuster, botID, userID, delta int64, reason string) error {
	if delta > 0 {
		return wallet.Credit(ctx, botID, userID, delta, reason, "admin_adjust", 0)
	}
	if delta < 0 {
		return wallet.Debit(ctx, botID, userID, -delta, reason, "admin_adjust", 0)
	}
	return nil
}

// WalletAdjuster abstracts billing wallet ops.
type WalletAdjuster interface {
	Credit(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error
	Debit(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error
}

func RedirectSettings(scope Scope, botID int64) string {
	return fmt.Sprintf("%s/bots/%d/settings", scope.Base, botID)
}

func RedirectBots(scope Scope) string {
	return scope.Base + "/bots"
}
