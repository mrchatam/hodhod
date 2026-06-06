package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/provisioning"
)

// PaymentReviewService handles payment approval (wallet top-up or plan provision).
type PaymentReviewService struct {
	Store  *db.Store
	Wallet *WalletService
	Orders *OrderService
	Prov   *provisioning.Service
}

// ApproveResult describes what happened on approve.
type ApproveResult struct {
	Provisioned bool
	Service     *db.Service
}

var ErrPaymentNotPending = errors.New("payment not pending")

// ApprovePaymentAndProvision approves a pending payment — credits wallet or provisions plan order.
func (s *PaymentReviewService) ApprovePaymentAndProvision(ctx context.Context, botID, paymentID, reviewerID int64) (*ApproveResult, error) {
	p, err := s.Store.GetPayment(ctx, botID, paymentID)
	if err != nil {
		return nil, err
	}
	if p.OrderID != nil && *p.OrderID > 0 {
		return s.approvePlanPayment(ctx, botID, paymentID, reviewerID)
	}
	if err := s.Wallet.ApprovePayment(ctx, botID, paymentID, reviewerID); err != nil {
		return nil, err
	}
	return &ApproveResult{}, nil
}

func (s *PaymentReviewService) approvePlanPayment(ctx context.Context, botID, paymentID, reviewerID int64) (*ApproveResult, error) {
	var orderID int64
	err := s.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p db.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, paymentID).First(&p).Error; err != nil {
			return err
		}
		if p.Status != db.PaymentPending {
			return ErrPaymentNotPending
		}
		if p.OrderID == nil {
			return errors.New("payment missing order")
		}
		orderID = *p.OrderID
		now := time.Now()
		p.Status = db.PaymentApproved
		p.ReviewedBy = &reviewerID
		p.ReviewedAt = &now
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		var o db.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bot_id = ? AND id = ?", botID, orderID).First(&o).Error; err != nil {
			return err
		}
		o.Status = db.OrderApproved
		o.ApprovedBy = &reviewerID
		o.ApprovedAt = &now
		return tx.Save(&o).Error
	})
	if err != nil {
		return nil, err
	}
	order, err := s.Store.GetOrder(ctx, botID, orderID)
	if err != nil {
		return nil, err
	}
	svc, err := s.Prov.ProvisionOrder(ctx, botID, order)
	if err != nil {
		return nil, err
	}
	_ = s.Orders.MarkOrderProvisioned(ctx, botID, order.ID)
	return &ApproveResult{Provisioned: true, Service: svc}, nil
}

// RejectPayment rejects a pending payment and linked order if any.
func (s *PaymentReviewService) RejectPayment(ctx context.Context, botID, paymentID, reviewerID int64) error {
	p, err := s.Store.GetPayment(ctx, botID, paymentID)
	if err != nil {
		return err
	}
	if err := s.Store.RejectPayment(ctx, botID, paymentID, reviewerID); err != nil {
		return err
	}
	if p.OrderID != nil && *p.OrderID > 0 {
		order, err := s.Store.GetOrder(ctx, botID, *p.OrderID)
		if err == nil && order.Status == db.OrderAwaitingApproval {
			order.Status = db.OrderRejected
			_ = s.Store.UpdateOrder(ctx, botID, order)
		}
	}
	return nil
}
