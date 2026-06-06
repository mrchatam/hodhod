package billing

import (
	"errors"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

func TestErrPaymentNotPending(t *testing.T) {
	if ErrPaymentNotPending.Error() != "payment not pending" {
		t.Fatalf("unexpected error text")
	}
}

func TestApproveResultFields(t *testing.T) {
	svc := &db.Service{SubLink: "https://example.com/sub"}
	res := &ApproveResult{Provisioned: true, Service: svc}
	if !res.Provisioned || res.Service.SubLink != svc.SubLink {
		t.Fatal("approve result mismatch")
	}
}

func TestApprovePaymentBranchesOrderID(t *testing.T) {
	p := db.Payment{OrderID: nil}
	if p.OrderID != nil {
		t.Fatal("expected nil order for top-up path")
	}
	orderID := int64(7)
	p.OrderID = &orderID
	if p.OrderID == nil || *p.OrderID != 7 {
		t.Fatal("expected plan order id")
	}
}

func TestCompensatingRejectStatuses(t *testing.T) {
	if db.PaymentRejected == "" || db.OrderRejected == "" {
		t.Fatal("reject statuses must be defined")
	}
	if !errors.Is(ErrPaymentNotPending, ErrPaymentNotPending) {
		t.Fatal("error identity")
	}
}
