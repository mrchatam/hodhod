package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
)

// OrderService manages purchase orders.
type OrderService struct {
	Store  *db.Store
	Wallet *WalletService
}

// PurchaseFromWallet debits balance and creates an approved order.
func (s *OrderService) PurchaseFromWallet(ctx context.Context, botID int64, user *db.EndUser, plan *db.Plan) (*db.Order, error) {
	if err := s.Wallet.Debit(ctx, botID, user.ID, plan.PriceToman, "plan_purchase", "plan", plan.ID); err != nil {
		return nil, err
	}
	now := time.Now()
	o := &db.Order{
		BotID:      botID,
		EndUserID:  user.ID,
		PlanID:     plan.ID,
		Status:     db.OrderApproved,
		PriceToman: plan.PriceToman,
		ApprovedAt: &now,
	}
	if err := s.Store.CreateOrder(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// CreatePendingOrder creates an order awaiting payment.
func (s *OrderService) CreatePendingOrder(ctx context.Context, botID int64, user *db.EndUser, plan *db.Plan) (*db.Order, error) {
	o := &db.Order{
		BotID:      botID,
		EndUserID:  user.ID,
		PlanID:     plan.ID,
		Status:     db.OrderPendingPayment,
		PriceToman: plan.PriceToman,
	}
	return o, s.Store.CreateOrder(ctx, o)
}

// CreateTopUpPayment records a manual receipt top-up request.
func (s *OrderService) CreateTopUpPayment(ctx context.Context, botID, endUserID, amount int64, receiptRef string) (*db.Payment, error) {
	p := &db.Payment{
		BotID:       botID,
		EndUserID:   endUserID,
		AmountToman: amount,
		Method:      "card_receipt",
		ReceiptRef:  receiptRef,
		Status:      db.PaymentPending,
	}
	return p, s.Store.CreatePayment(ctx, p)
}

// MarkOrderProvisioned updates order status after provisioning.
func (s *OrderService) MarkOrderProvisioned(ctx context.Context, botID, orderID int64) error {
	o, err := s.Store.GetOrder(ctx, botID, orderID)
	if err != nil {
		return err
	}
	o.Status = db.OrderProvisioned
	return s.Store.UpdateOrder(ctx, botID, o)
}

// ValidatePlanForAgent checks price/duration/volume within agent limits.
func ValidatePlanForAgent(plan *db.Plan, agent *db.Agent) error {
	if plan.PriceToman < agent.PriceFloorToman || plan.PriceToman > agent.PriceCeilingToman {
		return fmt.Errorf("price out of allowed range")
	}
	if plan.DurationDays < 1 || plan.VolumeGB < 1 {
		return fmt.Errorf("invalid duration or volume")
	}
	return nil
}
