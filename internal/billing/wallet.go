package billing

import (
	"context"

	"github.com/mrchatam/hodhod/internal/db"
)

// WalletService handles balance operations.
type WalletService struct {
	Store *db.Store
}

// Credit adds funds after approved top-up.
func (s *WalletService) Credit(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error {
	return s.Store.CreditWallet(ctx, botID, endUserID, amount, reason, refType, refID)
}

// Debit removes funds for a purchase.
func (s *WalletService) Debit(ctx context.Context, botID, endUserID, amount int64, reason, refType string, refID int64) error {
	return s.Store.DebitWallet(ctx, botID, endUserID, amount, reason, refType, refID)
}

// Adjust credits or debits an end-user wallet with reason (admin).
func (s *WalletService) Adjust(ctx context.Context, botID, endUserID, delta int64, reason string) error {
	if delta > 0 {
		return s.Credit(ctx, botID, endUserID, delta, reason, "admin_adjust", 0)
	}
	if delta < 0 {
		return s.Debit(ctx, botID, endUserID, -delta, reason, "admin_adjust", 0)
	}
	return nil
}

// ApprovePayment credits wallet from a pending payment.
func (s *WalletService) ApprovePayment(ctx context.Context, botID, paymentID, reviewerID int64) error {
	return s.Store.ApprovePayment(ctx, botID, paymentID, reviewerID)
}
